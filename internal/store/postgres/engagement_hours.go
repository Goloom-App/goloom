package postgres

import (
	"context"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// GetTeamEngagementHourHistogram aggregates post_metrics sums by UTC hour of scheduled_at for posted rows.
func (s *Store) GetTeamEngagementHourHistogram(ctx context.Context, teamID string, days int) ([]domain.EngagementHourBucket, error) {
	if days <= 0 {
		days = 90
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	rows, err := s.pool.Query(ctx, `
		select extract(hour from scheduled_at at time zone 'utc')::integer as hr,
		       coalesce(sum(pm.value), 0)::bigint
		from scheduled_posts sp
		join post_metrics pm on pm.post_id = sp.id
		where sp.team_id = $1
		  and sp.status = 'posted'
		  and sp.scheduled_at >= $2
		group by hr
		order by hr`,
		teamID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.EngagementHourBucket, 0, 24)
	for rows.Next() {
		var b domain.EngagementHourBucket
		if err := rows.Scan(&b.HourUTC, &b.Score); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetTeamEngagementHeatmap aggregates post_metrics sums by UTC weekday (0=Sunday)
// and hour, optionally restricted to one account.
func (s *Store) GetTeamEngagementHeatmap(ctx context.Context, teamID string, days int, accountID string) ([]domain.EngagementHeatmapBucket, error) {
	if days <= 0 {
		days = 90
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	accountFilter := ""
	args := []any{teamID, since}
	if id := strings.TrimSpace(accountID); id != "" && id != "all" {
		args = append(args, id)
		accountFilter = " and pm.account_id = $3"
	}
	rows, err := s.pool.Query(ctx, `
		select extract(dow from scheduled_at at time zone 'utc')::integer as wd,
		       extract(hour from scheduled_at at time zone 'utc')::integer as hr,
		       coalesce(sum(pm.value), 0)::bigint
		from scheduled_posts sp
		join post_metrics pm on pm.post_id = sp.id
		where sp.team_id = $1
		  and sp.status = 'posted'
		  and sp.scheduled_at >= $2`+accountFilter+`
		group by wd, hr
		order by wd, hr`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.EngagementHeatmapBucket, 0, 7*24)
	for rows.Next() {
		var b domain.EngagementHeatmapBucket
		if err := rows.Scan(&b.WeekdayUTC, &b.HourUTC, &b.Score); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListTeamPostEngagement returns each posted post with its total engagement (sum
// of post_metrics) within the time window, optionally restricted to one provider.
// The raw rows feed timezone-aware timeslot analysis in the domain layer.
func (s *Store) ListTeamPostEngagement(ctx context.Context, teamID string, days int, provider string) ([]domain.PostEngagement, error) {
	if days <= 0 {
		days = 90
	}
	if days > 366 {
		days = 366
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	provider = strings.TrimSpace(strings.ToLower(provider))

	var query string
	var args []any
	if provider != "" && provider != "all" {
		// Restrict engagement to the provider's accounts; a post with no metrics
		// on that provider drops out of the analysis entirely.
		query = `
			select sp.id, sp.scheduled_at, coalesce(sum(pm.value), 0)::bigint
			from scheduled_posts sp
			inner join post_metrics pm on pm.post_id = sp.id
			inner join social_accounts a on a.id = pm.account_id and a.provider = $1
			where sp.team_id = $2 and sp.status = 'posted' and sp.scheduled_at >= $3
			group by sp.id, sp.scheduled_at
			order by sp.scheduled_at asc`
		args = []any{provider, teamID, since}
	} else {
		query = `
			select sp.id, sp.scheduled_at, coalesce(sum(pm.value), 0)::bigint
			from scheduled_posts sp
			left join post_metrics pm on pm.post_id = sp.id
			where sp.team_id = $1 and sp.status = 'posted' and sp.scheduled_at >= $2
			group by sp.id, sp.scheduled_at
			order by sp.scheduled_at asc`
		args = []any{teamID, since}
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PostEngagement
	for rows.Next() {
		var row domain.PostEngagement
		if err := rows.Scan(&row.PostID, &row.ScheduledAt, &row.Engagement); err != nil {
			return nil, err
		}
		row.ScheduledAt = row.ScheduledAt.UTC()
		out = append(out, row)
	}
	return out, rows.Err()
}
