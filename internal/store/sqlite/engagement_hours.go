package sqlite

import (
	"context"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// GetTeamEngagementHourHistogram aggregates post_metrics by UTC hour using substring on ISO8601 scheduled_at.
func (s *Store) GetTeamEngagementHourHistogram(ctx context.Context, teamID string, days int) ([]domain.EngagementHourBucket, error) {
	if days <= 0 {
		days = 90
	}
	since := time.Now().UTC().AddDate(0, 0, -days)
	sinceStr := formatTime(since)
	rows, err := s.db.QueryContext(ctx, `
		select cast(substr(sp.scheduled_at, 12, 2) as integer) as hr,
		       coalesce(sum(pm.value), 0)
		from scheduled_posts sp
		join post_metrics pm on pm.post_id = sp.id
		where sp.team_id = ?
		  and sp.status = 'posted'
		  and sp.scheduled_at >= ?
		group by hr
		order by hr`,
		teamID, sinceStr,
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
