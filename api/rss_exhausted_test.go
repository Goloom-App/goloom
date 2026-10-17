package api_test

import (
	"context"
	"net/http"
	"testing"
)

func TestRSSFeedAPIExposesExhaustedItems(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	base := "/v1/teams/" + f.team.ID + "/rss-feeds"
	rec := f.do(t, http.MethodPost, base, map[string]any{"feed_url": "https://example.test/exhausted", "name": "exhausted"})
	requireStatus(t, rec, http.StatusCreated)
	data := decodeJSON[map[string]any](t, rec)
	feedID := data["id"].(string)
	if data["exhausted_items_count"] != float64(0) {
		t.Fatalf("new feed count=%v", data["exhausted_items_count"])
	}
	for range 3 {
		if won, err := f.store.ClaimRSSItemAttempt(ctx, f.team.ID, feedID, "exhausted"); err != nil || !won {
			t.Fatal(won, err)
		}
		if _, err := f.store.ReleaseRSSItemAttempt(ctx, f.team.ID, feedID, "exhausted"); err != nil {
			t.Fatal(err)
		}
	}
	rec = f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)
	listed := decodeJSON[struct {
		Items []map[string]any `json:"items"`
	}](t, rec)
	if len(listed.Items) != 1 || listed.Items[0]["exhausted_items_count"] != float64(1) {
		t.Fatalf("list=%v", listed.Items)
	}
	// Patch returns the refreshed feed read model, including its exhaustion count.
	rec = f.do(t, http.MethodPatch, base+"/"+feedID, map[string]any{"name": "renamed"})
	requireStatus(t, rec, http.StatusOK)
	data = decodeJSON[map[string]any](t, rec)
	if data["exhausted_items_count"] != float64(1) {
		t.Fatalf("detail count=%v", data["exhausted_items_count"])
	}
}
