package domain

import (
	"testing"
	"time"
)

func TestAggregateTimeslotsRanksBySmoothedAverage(t *testing.T) {
	// Monday 10:00 UTC: two posts, total 30 → avg 15, score 30/3 = 10.
	// Tuesday 14:00 UTC: one post, 18 → avg 18, score 18/2 = 9.
	mon := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC) // Monday
	tue := time.Date(2026, 6, 9, 14, 0, 0, 0, time.UTC) // Tuesday
	posts := []PostEngagement{
		{PostID: "a", ScheduledAt: mon, Engagement: 20},
		{PostID: "b", ScheduledAt: mon.Add(2 * time.Minute), Engagement: 10},
		{PostID: "c", ScheduledAt: tue, Engagement: 18},
	}

	got := AggregateTimeslots(posts, time.UTC, 0)
	if len(got) != 2 {
		t.Fatalf("got %d buckets, want 2: %#v", len(got), got)
	}

	// Despite the lower average, Monday wins on the smoothed score because the
	// sample size is larger.
	top := got[0]
	if top.Weekday != time.Monday || top.Hour != 10 {
		t.Fatalf("top bucket = %v %dh, want Monday 10h", top.Weekday, top.Hour)
	}
	if top.Posts != 2 || top.TotalEngagement != 30 || top.AvgEngagement != 15 {
		t.Fatalf("top bucket aggregates wrong: %#v", top)
	}
	if top.Score != 10 {
		t.Fatalf("top score = %v, want 10", top.Score)
	}
	if got[1].Weekday != time.Tuesday || got[1].Hour != 14 {
		t.Fatalf("second bucket = %v %dh, want Tuesday 14h", got[1].Weekday, got[1].Hour)
	}
}

func TestAggregateTimeslotsHonorsTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata unavailable:", err)
	}
	// 2026-06-08 02:00 UTC is still Sunday 22:00 in New York (EDT, -4) — the
	// weekday and hour must follow the requested zone, not UTC.
	utc := time.Date(2026, 6, 8, 2, 0, 0, 0, time.UTC)
	posts := []PostEngagement{{PostID: "x", ScheduledAt: utc, Engagement: 5}}

	got := AggregateTimeslots(posts, loc, 0)
	if len(got) != 1 {
		t.Fatalf("got %d buckets, want 1", len(got))
	}
	if got[0].Weekday != time.Sunday || got[0].Hour != 22 {
		t.Fatalf("bucket = %v %dh, want Sunday 22h", got[0].Weekday, got[0].Hour)
	}
}

func TestAggregateTimeslotsLimitAndNilLocation(t *testing.T) {
	base := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	var posts []PostEngagement
	for i := 0; i < 4; i++ {
		posts = append(posts, PostEngagement{
			PostID:      "p",
			ScheduledAt: base.AddDate(0, 0, i).Add(time.Duration(i) * time.Hour),
			Engagement:  int64(i + 1),
		})
	}
	// nil location must not panic and must behave like UTC.
	got := AggregateTimeslots(posts, nil, 2)
	if len(got) != 2 {
		t.Fatalf("limit not applied: got %d buckets", len(got))
	}
}

func TestAggregateTimeslotsEmpty(t *testing.T) {
	if got := AggregateTimeslots(nil, time.UTC, 5); len(got) != 0 {
		t.Fatalf("expected no buckets, got %#v", got)
	}
}
