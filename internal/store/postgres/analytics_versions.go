package postgres

import (
	"context"
	"errors"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) GetTeamAnalytics(ctx context.Context, teamID string, topPostsLimit int) (domain.TeamAnalyticsSummary, error) {
	if topPostsLimit <= 0 {
		topPostsLimit = 10
	}
	out := domain.TeamAnalyticsSummary{
		MetricsTotal: make(map[string]int64),
		TopPosts:     []domain.PostEngagementSummary{},
	}

	rows, err := s.pool.Query(ctx, `
		select m.metric, sum(m.value)::bigint
		from post_metrics m
		inner join scheduled_posts p on p.id = m.post_id
		where p.team_id = $1 and p.status = 'posted'
		group by m.metric`,
		teamID,
	)
	if err != nil {
		return domain.TeamAnalyticsSummary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var sum int64
		if err := rows.Scan(&name, &sum); err != nil {
			return domain.TeamAnalyticsSummary{}, err
		}
		name = strings.TrimSpace(name)
		if name != "" {
			out.MetricsTotal[name] = sum
		}
	}
	if err := rows.Err(); err != nil {
		return domain.TeamAnalyticsSummary{}, err
	}

	topRows, err := s.pool.Query(ctx, `
		select p.id::text, coalesce(p.title, ''), coalesce(sum(m.value), 0)::bigint as score
		from scheduled_posts p
		inner join post_metrics m on m.post_id = p.id
		where p.team_id = $1 and p.status = 'posted'
		group by p.id, p.title
		order by score desc
		limit $2`,
		teamID, topPostsLimit,
	)
	if err != nil {
		return domain.TeamAnalyticsSummary{}, err
	}
	defer topRows.Close()
	for topRows.Next() {
		var row domain.PostEngagementSummary
		if err := topRows.Scan(&row.PostID, &row.Title, &row.Score); err != nil {
			return domain.TeamAnalyticsSummary{}, err
		}
		out.TopPosts = append(out.TopPosts, row)
	}
	return out, topRows.Err()
}

func (s *Store) ListPostMetricsForTeamPost(ctx context.Context, teamID, postID string) ([]domain.PostMetric, error) {
	rows, err := s.pool.Query(ctx, `
		select m.post_id::text, m.account_id::text, m.metric, m.value, m.updated_at
		from post_metrics m
		inner join scheduled_posts p on p.id = m.post_id
		where m.post_id = $1 and p.team_id = $2
		order by m.account_id, m.metric`,
		postID, teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PostMetric
	for rows.Next() {
		var m domain.PostMetric
		if err := rows.Scan(&m.PostID, &m.AccountID, &m.Metric, &m.Value, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) ListPostVersionsForTeamPost(ctx context.Context, teamID, postID string) ([]domain.PostVersion, error) {
	rows, err := s.pool.Query(ctx, `
		select v.post_id::text, v.account_id::text, v.content
		from post_versions v
		inner join scheduled_posts p on p.id = v.post_id
		where v.post_id = $1 and p.team_id = $2
		order by v.account_id`,
		postID, teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.PostVersion
	for rows.Next() {
		var v domain.PostVersion
		if err := rows.Scan(&v.PostID, &v.AccountID, &v.Content); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) ApplyPostVersionsPatch(ctx context.Context, teamID, postID string, versions []domain.PostVersion) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var gotTeam string
	err = tx.QueryRow(ctx, `select team_id::text from scheduled_posts where id = $1`, postID).Scan(&gotTeam)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("post not found")
	}
	if err != nil {
		return err
	}
	if gotTeam != teamID {
		return errors.New("post not found")
	}

	targetRows, err := tx.Query(ctx, `select account_id::text from scheduled_post_targets where post_id = $1`, postID)
	if err != nil {
		return err
	}
	defer targetRows.Close()
	valid := make(map[string]struct{})
	for targetRows.Next() {
		var id string
		if err := targetRows.Scan(&id); err != nil {
			return err
		}
		valid[id] = struct{}{}
	}
	if err := targetRows.Err(); err != nil {
		return err
	}

	for _, v := range versions {
		aid := strings.TrimSpace(v.AccountID)
		if aid == "" {
			continue
		}
		if _, ok := valid[aid]; !ok {
			return errors.New("account not targeted by post")
		}
		if strings.TrimSpace(v.Content) == "" {
			if _, err := tx.Exec(ctx, `delete from post_versions where post_id = $1 and account_id = $2`, postID, aid); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(ctx, `
			insert into post_versions (post_id, account_id, content)
			values ($1, $2, $3)
			on conflict (post_id, account_id) do update set content = excluded.content`,
			postID, aid, v.Content,
		); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
