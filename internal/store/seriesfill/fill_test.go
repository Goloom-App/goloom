package seriesfill

import (
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestFillMetricHistory_forwardFillsToToday(t *testing.T) {
	now := time.Date(2026, 5, 19, 15, 0, 0, 0, time.UTC)
	sparse := []domain.MetricHistoryPoint{
		{Date: "2026-05-14", Value: 10},
		{Date: "2026-05-16", Value: 16},
	}
	out := FillMetricHistory(sparse, 7, now)
	if len(out) != 7 {
		t.Fatalf("len: got %d want 7: %#v", len(out), out)
	}
	last := out[len(out)-1]
	if last.Date != "2026-05-19" {
		t.Fatalf("last date: got %q", last.Date)
	}
	if last.Value != 16 {
		t.Fatalf("last value: got %d want 16", last.Value)
	}
	if out[0].Date != "2026-05-13" || out[0].Value != 0 {
		t.Fatalf("first point: %#v", out[0])
	}
	if out[3].Date != "2026-05-16" || out[3].Value != 16 {
		t.Fatalf("May 16: %#v", out[3])
	}
	if out[4].Value != 16 || out[5].Value != 16 {
		t.Fatalf("forward fill after May 16: %#v %#v", out[4], out[5])
	}
}

func TestFillMetricHistory_seedsFromDayBeforeRange(t *testing.T) {
	now := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	sparse := []domain.MetricHistoryPoint{
		{Date: "2026-05-12", Value: 90},
		{Date: "2026-05-16", Value: 100},
	}
	out := FillMetricHistory(sparse, 7, now)
	if out[0].Date != "2026-05-13" || out[0].Value != 90 {
		t.Fatalf("seeded first day: %#v", out[0])
	}
}
