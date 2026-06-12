package postgres

import (
	"context"
	"strconv"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/hashtag"
)

// ReplacePostHashtags stores the hashtags of the content published for one
// post/account pair, replacing previous rows.
func (s *Store) ReplacePostHashtags(ctx context.Context, postID, accountID string, tags []hashtag.Tag) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `delete from post_hashtags where post_id = $1 and account_id = $2`, postID, accountID); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := tx.Exec(ctx, `
			insert into post_hashtags (post_id, account_id, tag_norm, tag_display)
			values ($1, $2, $3, $4)
			on conflict (post_id, account_id, tag_norm) do nothing`,
			postID, accountID, tag.Norm, tag.Display,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// BackfillPostHashtags extracts hashtags from already-posted content (per-account
// version override or post content). Idempotent; intended for startup.
func (s *Store) BackfillPostHashtags(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		select p.id::text, t.account_id::text, p.content, coalesce(v.content, '')
		from scheduled_posts p
		inner join scheduled_post_targets t on t.post_id = p.id and t.status = 'posted'
		left join post_versions v on v.post_id = p.id and v.account_id = t.account_id
		where p.status = 'posted'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type pair struct {
		postID, accountID, content string
	}
	var pairs []pair
	for rows.Next() {
		var p pair
		var versionContent string
		if err := rows.Scan(&p.postID, &p.accountID, &p.content, &versionContent); err != nil {
			return err
		}
		if strings.TrimSpace(versionContent) != "" {
			p.content = versionContent
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, p := range pairs {
		for _, tag := range hashtag.Extract(p.content) {
			if _, err := tx.Exec(ctx, `
				insert into post_hashtags (post_id, account_id, tag_norm, tag_display)
				values ($1, $2, $3, $4)
				on conflict (post_id, account_id, tag_norm) do nothing`,
				p.postID, p.accountID, tag.Norm, tag.Display,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// ListTeamHashtagPerformance aggregates engagement per normalized hashtag for
// posted content, optionally filtered by provider and time window.
func (s *Store) ListTeamHashtagPerformance(ctx context.Context, teamID string, days int, provider string, limit int) ([]domain.HashtagPerformance, error) {
	if days <= 0 {
		days = 90
	}
	if days > 366 {
		days = 366
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	providerFilter := ""
	args := []any{teamID, cutoff}
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider != "" && provider != "all" {
		args = append(args, provider)
		providerFilter = " and a.provider = $3"
	}
	smoothing := strconv.Itoa(domain.HashtagScoreSmoothing)
	args = append(args, limit)
	limitPlaceholder := "$" + strconv.Itoa(len(args))
	query := `
		select ph.tag_norm,
		       (select ph2.tag_display
		        from post_hashtags ph2
		        inner join scheduled_posts p2 on p2.id = ph2.post_id
		        where ph2.tag_norm = ph.tag_norm and p2.team_id = $1
		        order by p2.scheduled_at desc limit 1) as display,
		       count(distinct ph.post_id)::bigint as uses,
		       coalesce(sum(pm.value), 0)::bigint as total
		from post_hashtags ph
		inner join scheduled_posts p on p.id = ph.post_id
		inner join social_accounts a on a.id = ph.account_id
		left join post_metrics pm on pm.post_id = ph.post_id and pm.account_id = ph.account_id
		where p.team_id = $1 and p.status = 'posted' and p.scheduled_at >= $2` + providerFilter + `
		group by ph.tag_norm
		order by (coalesce(sum(pm.value), 0)::float / (count(distinct ph.post_id) + ` + smoothing + `)) desc, uses desc, ph.tag_norm asc
		limit ` + limitPlaceholder
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.HashtagPerformance, 0, limit)
	for rows.Next() {
		var row domain.HashtagPerformance
		if err := rows.Scan(&row.Tag, &row.Display, &row.Uses, &row.TotalEngagement); err != nil {
			return nil, err
		}
		row.FinalizeScores()
		out = append(out, row)
	}
	return out, rows.Err()
}
