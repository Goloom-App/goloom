package sqlite

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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `delete from post_hashtags where post_id = ? and account_id = ?`, postID, accountID); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, `
			insert into post_hashtags (post_id, account_id, tag_norm, tag_display)
			values (?, ?, ?, ?)
			on conflict (post_id, account_id, tag_norm) do nothing`,
			postID, accountID, tag.Norm, tag.Display,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BackfillPostHashtags extracts hashtags from already-posted content (per-account
// version override or post content). Idempotent; intended for startup.
func (s *Store) BackfillPostHashtags(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		select p.id, t.account_id, p.content, coalesce(v.content, '')
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range pairs {
		for _, tag := range hashtag.Extract(p.content) {
			if _, err := tx.ExecContext(ctx, `
				insert into post_hashtags (post_id, account_id, tag_norm, tag_display)
				values (?, ?, ?, ?)
				on conflict (post_id, account_id, tag_norm) do nothing`,
				p.postID, p.accountID, tag.Norm, tag.Display,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// GetTeamHashtagInsights summarizes hashtag usage for posted content within a
// time window, optionally filtered by provider.
func (s *Store) GetTeamHashtagInsights(ctx context.Context, teamID string, days int, provider string) (domain.HashtagInsights, error) {
	if days <= 0 {
		days = 90
	}
	if days > 366 {
		days = 366
	}
	cutoff := formatTime(time.Now().UTC().AddDate(0, 0, -days))
	provider = strings.TrimSpace(strings.ToLower(provider))
	hasProvider := provider != "" && provider != "all"

	metricsFilter, tagsFilter, postsFilter := "", "", ""
	args := []any{}
	if hasProvider {
		metricsFilter = " and ma.provider = ?"
		tagsFilter = " and ta.provider = ?"
		postsFilter = ` and exists (
			select 1 from scheduled_post_targets t
			inner join social_accounts pa on pa.id = t.account_id
			where t.post_id = p.id and pa.provider = ?)`
	}
	// Placeholder order follows query text: metrics provider, tags provider,
	// team, cutoff, posts provider.
	if hasProvider {
		args = append(args, provider, provider)
	}
	args = append(args, teamID, cutoff)
	if hasProvider {
		args = append(args, provider)
	}
	query := `
		with posts as (
			select p.id,
			       coalesce((select sum(pm.value)
			                 from post_metrics pm
			                 inner join social_accounts ma on ma.id = pm.account_id
			                 where pm.post_id = p.id` + metricsFilter + `), 0) as engagement,
			       (select count(distinct ph.tag_norm)
			        from post_hashtags ph
			        inner join social_accounts ta on ta.id = ph.account_id
			        where ph.post_id = p.id` + tagsFilter + `) as tag_count
			from scheduled_posts p
			where p.team_id = ? and p.status = 'posted' and p.scheduled_at >= ?` + postsFilter + `
		)
		select count(*),
		       coalesce(sum(case when tag_count > 0 then 1 else 0 end), 0),
		       coalesce(sum(tag_count), 0),
		       coalesce(sum(case when tag_count > 0 then engagement else 0 end), 0),
		       coalesce(sum(case when tag_count = 0 then engagement else 0 end), 0)
		from posts`
	var postsTotal, postsWithTags, totalTagUses, engagementWith, engagementWithout int64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&postsTotal, &postsWithTags, &totalTagUses, &engagementWith, &engagementWithout); err != nil {
		return domain.HashtagInsights{}, err
	}

	distinctArgs := []any{teamID, cutoff}
	if hasProvider {
		distinctArgs = append(distinctArgs, provider)
	}
	distinctQuery := `
		select count(distinct ph.tag_norm)
		from post_hashtags ph
		inner join scheduled_posts p on p.id = ph.post_id
		inner join social_accounts ta on ta.id = ph.account_id
		where p.team_id = ? and p.status = 'posted' and p.scheduled_at >= ?` + tagsFilter
	var distinctTags int64
	if err := s.db.QueryRowContext(ctx, distinctQuery, distinctArgs...).Scan(&distinctTags); err != nil {
		return domain.HashtagInsights{}, err
	}

	return domain.BuildHashtagInsights(postsTotal, postsWithTags, distinctTags, totalTagUses, engagementWith, engagementWithout), nil
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
	cutoff := formatTime(time.Now().UTC().AddDate(0, 0, -days))
	providerFilter := ""
	args := []any{teamID, teamID, cutoff}
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider != "" && provider != "all" {
		args = append(args, provider)
		providerFilter = " and a.provider = ?"
	}
	args = append(args, limit)
	query := `
		select ph.tag_norm,
		       (select ph2.tag_display
		        from post_hashtags ph2
		        inner join scheduled_posts p2 on p2.id = ph2.post_id
		        where ph2.tag_norm = ph.tag_norm and p2.team_id = ?
		        order by p2.scheduled_at desc limit 1) as display,
		       count(distinct ph.post_id) as uses,
		       coalesce(sum(pm.value), 0) as total
		from post_hashtags ph
		inner join scheduled_posts p on p.id = ph.post_id
		inner join social_accounts a on a.id = ph.account_id
		left join post_metrics pm on pm.post_id = ph.post_id and pm.account_id = ph.account_id
		where p.team_id = ? and p.status = 'posted' and p.scheduled_at >= ?` + providerFilter + `
		group by ph.tag_norm
		order by (cast(coalesce(sum(pm.value), 0) as real) / (count(distinct ph.post_id) + ` + strconv.Itoa(domain.HashtagScoreSmoothing) + `)) desc, uses desc, ph.tag_norm asc
		limit ?`
	rows, err := s.db.QueryContext(ctx, query, args...)
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
