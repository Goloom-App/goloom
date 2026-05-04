package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
)

func (s *Store) GetTeamAnalytics(ctx context.Context, teamID string, topPostsLimit int) (domain.TeamAnalyticsSummary, error) {
	if topPostsLimit <= 0 {
		topPostsLimit = 10
	}
	out := domain.TeamAnalyticsSummary{
		MetricsTotal: make(map[string]int64),
		TopPosts:     []domain.PostEngagementSummary{},
	}

	rows, err := s.db.QueryContext(ctx, `
		select m.metric, sum(m.value)
		from post_metrics m
		inner join scheduled_posts p on p.id = m.post_id
		where p.team_id = ? and p.status = 'posted'
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

	topRows, err := s.db.QueryContext(ctx, `
		select p.id, coalesce(p.title, ''), coalesce(sum(m.value), 0) as score
		from scheduled_posts p
		inner join post_metrics m on m.post_id = p.id
		where p.team_id = ? and p.status = 'posted'
		group by p.id, p.title
		order by score desc
		limit ?`,
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
	rows, err := s.db.QueryContext(ctx, `
		select m.post_id, m.account_id, m.metric, m.value, m.updated_at
		from post_metrics m
		inner join scheduled_posts p on p.id = m.post_id
		where m.post_id = ? and p.team_id = ?
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
		var updatedAt string
		if err := rows.Scan(&m.PostID, &m.AccountID, &m.Metric, &m.Value, &updatedAt); err != nil {
			return nil, err
		}
		ts, err := parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		m.UpdatedAt = ts
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) ListPostVersionsForTeamPost(ctx context.Context, teamID, postID string) ([]domain.PostVersion, error) {
	rows, err := s.db.QueryContext(ctx, `
		select v.post_id, v.account_id, v.content
		from post_versions v
		inner join scheduled_posts p on p.id = v.post_id
		where v.post_id = ? and p.team_id = ?
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var gotTeam string
	err = tx.QueryRowContext(ctx, `select team_id from scheduled_posts where id = ?`, postID).Scan(&gotTeam)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("post not found")
	}
	if err != nil {
		return err
	}
	if gotTeam != teamID {
		return errors.New("post not found")
	}

	targetRows, err := tx.QueryContext(ctx, `select account_id from scheduled_post_targets where post_id = ?`, postID)
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
			if _, err := tx.ExecContext(ctx, `delete from post_versions where post_id = ? and account_id = ?`, postID, aid); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			insert into post_versions (post_id, account_id, content)
			values (?, ?, ?)
			on conflict(post_id, account_id) do update set content = excluded.content`,
			postID, aid, v.Content,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
