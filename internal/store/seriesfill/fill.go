package seriesfill

import (
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// MetricHistoryRange returns inclusive UTC calendar bounds for the last days ending on nowUTC.
func MetricHistoryRange(days int, nowUTC time.Time) (start, end time.Time) {
	if days <= 0 {
		days = 30
	}
	if days > 366 {
		days = 366
	}
	nowUTC = nowUTC.UTC()
	end = time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
	start = end.AddDate(0, 0, -(days - 1))
	return start, end
}

// FillMetricHistory extends sparse daily totals to every calendar day in the range,
// forward-filling cumulative values so charts include today even without a fresh sync row.
func FillMetricHistory(sparse []domain.MetricHistoryPoint, days int, nowUTC time.Time) []domain.MetricHistoryPoint {
	start, end := MetricHistoryRange(days, nowUTC)
	byDate := make(map[string]int64, len(sparse))
	for _, p := range sparse {
		d := strings.TrimSpace(p.Date)
		if d == "" {
			continue
		}
		byDate[d] = p.Value
	}

	seedDay := start.AddDate(0, 0, -1)
	var lastKnown int64
	out := make([]domain.MetricHistoryPoint, 0, days+1)
	for d := seedDay; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		if v, ok := byDate[key]; ok {
			lastKnown = v
		}
		if d.Before(start) {
			continue
		}
		out = append(out, domain.MetricHistoryPoint{Date: key, Value: lastKnown})
	}
	return out
}

// FillAccountGrowth extends sparse account growth points across the same daily range.
func FillAccountGrowth(sparse []domain.AccountMetricHistoryPoint, days int, nowUTC time.Time) []domain.AccountMetricHistoryPoint {
	start, end := MetricHistoryRange(days, nowUTC)
	type snap struct {
		followers, following, posts int64
	}
	byDate := make(map[string]snap, len(sparse))
	for _, p := range sparse {
		d := strings.TrimSpace(p.Date)
		if d == "" {
			continue
		}
		byDate[d] = snap{followers: p.Followers, following: p.Following, posts: p.Posts}
	}

	seedDay := start.AddDate(0, 0, -1)
	var last snap
	out := make([]domain.AccountMetricHistoryPoint, 0, days+1)
	for d := seedDay; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		if v, ok := byDate[key]; ok {
			last = v
		}
		if d.Before(start) {
			continue
		}
		out = append(out, domain.AccountMetricHistoryPoint{
			Date:      key,
			Followers: last.followers,
			Following: last.following,
			Posts:     last.posts,
		})
	}
	return out
}
