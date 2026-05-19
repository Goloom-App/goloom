package postgres

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/store/seriesfill"
)

func prevISODate(yyyyMMdd string) (string, bool) {
	t, err := time.Parse("2006-01-02", yyyyMMdd)
	if err != nil {
		return "", false
	}
	return t.UTC().AddDate(0, 0, -1).Format("2006-01-02"), true
}

func (s *Store) GetTeamAnalyticsReport(ctx context.Context, teamID string, topPostsLimit int) (domain.TeamAnalyticsReport, error) {
	summary, err := s.GetTeamAnalytics(ctx, teamID, topPostsLimit)
	if err != nil {
		return domain.TeamAnalyticsReport{}, err
	}
	rows, err := s.pool.Query(ctx, `
		select h.metric, h.recorded_at::text, sum(h.value)::bigint
		from post_metrics_history h
		inner join scheduled_posts p on p.id = h.post_id
		where p.team_id = $1 and p.status = 'posted'
		group by h.metric, h.recorded_at`,
		teamID,
	)
	if err != nil {
		return domain.TeamAnalyticsReport{}, err
	}
	defer rows.Close()

	byMetricDay := make(map[string]map[string]int64)
	maxD := ""
	for rows.Next() {
		var metric, day string
		var sum int64
		if err := rows.Scan(&metric, &day, &sum); err != nil {
			return domain.TeamAnalyticsReport{}, err
		}
		metric = strings.TrimSpace(metric)
		day = strings.TrimSpace(day)
		if metric == "" || day == "" {
			continue
		}
		if byMetricDay[metric] == nil {
			byMetricDay[metric] = make(map[string]int64)
		}
		byMetricDay[metric][day] = sum
		if maxD == "" || day > maxD {
			maxD = day
		}
	}
	if err := rows.Err(); err != nil {
		return domain.TeamAnalyticsReport{}, err
	}

	prevD := ""
	if maxD != "" {
		if p, ok := prevISODate(maxD); ok {
			prevD = p
		}
	}

	metricNames := make(map[string]struct{})
	for k := range summary.MetricsTotal {
		metricNames[k] = struct{}{}
	}
	for k := range byMetricDay {
		metricNames[k] = struct{}{}
	}
	names := make([]string, 0, len(metricNames))
	for k := range metricNames {
		names = append(names, k)
	}
	sort.Strings(names)

	deltas := make([]domain.TeamMetricDelta, 0, len(names))
	for _, name := range names {
		total := summary.MetricsTotal[name]
		var d0 int64
		if maxD != "" && byMetricDay[name] != nil {
			d0 = byMetricDay[name][maxD]
		}
		var d1 int64
		hasPrev := prevD != "" && byMetricDay[name] != nil
		if hasPrev {
			var ok bool
			d1, ok = byMetricDay[name][prevD]
			hasPrev = ok
		}
		var delta int64
		if hasPrev {
			delta = d0 - d1
		}
		var pct *float64
		if hasPrev && d1 != 0 {
			v := float64(delta) / float64(d1) * 100
			if !math.IsNaN(v) && !math.IsInf(v, 0) {
				pct = &v
			}
		}
		deltas = append(deltas, domain.TeamMetricDelta{
			Metric:         name,
			Total:          total,
			DeltaVsPrevDay: delta,
			DeltaPercent:   pct,
		})
	}

	return domain.TeamAnalyticsReport{
		Metrics:  deltas,
		TopPosts: summary.TopPosts,
	}, nil
}

func (s *Store) ListTeamPostAnalyticsRanking(ctx context.Context, teamID string, sort string, limit, offset int) ([]domain.PostAnalyticsListRow, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	orderClause := "score desc, p.scheduled_at desc"
	switch strings.TrimSpace(strings.ToLower(sort)) {
	case "scheduled_at":
		orderClause = "p.scheduled_at desc, score desc"
	case "", "score":
		// default
	default:
		orderClause = "score desc, p.scheduled_at desc"
	}
	query := `
		select p.id::text, coalesce(p.title, ''), p.scheduled_at, coalesce(sum(m.value), 0)::bigint as score
		from scheduled_posts p
		left join post_metrics m on m.post_id = p.id
		where p.team_id = $1 and p.status = 'posted'
		group by p.id, p.title, p.scheduled_at
		order by ` + orderClause + `
		limit $2 offset $3`
	rows, err := s.pool.Query(ctx, query, teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PostAnalyticsListRow
	for rows.Next() {
		var row domain.PostAnalyticsListRow
		if err := rows.Scan(&row.PostID, &row.Title, &row.ScheduledAt, &row.Score); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) GetTeamMetricHistorySeries(ctx context.Context, teamID, metric string, days int) ([]domain.MetricHistoryPoint, error) {
	metric = strings.TrimSpace(metric)
	if metric == "" {
		return []domain.MetricHistoryPoint{}, nil
	}
	if days <= 0 {
		days = 30
	}
	if days > 366 {
		days = 366
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.pool.Query(ctx, `
		select h.recorded_at::text, sum(h.value)::bigint
		from post_metrics_history h
		inner join scheduled_posts p on p.id = h.post_id
		where p.team_id = $1 and p.status = 'posted' and h.metric = $2
		  and h.recorded_at >= $3::date
		group by h.recorded_at
		order by h.recorded_at asc`,
		teamID, metric, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.MetricHistoryPoint
	for rows.Next() {
		var pt domain.MetricHistoryPoint
		if err := rows.Scan(&pt.Date, &pt.Value); err != nil {
			return nil, err
		}
		pt.Date = strings.TrimSpace(pt.Date)
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return seriesfill.FillMetricHistory(out, days, time.Now().UTC()), nil
}
