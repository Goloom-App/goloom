package postgres

import (
	"context"
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
