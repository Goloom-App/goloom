package api

import (
	"encoding/json"
	"testing"
)

func TestRSSAutomationCarriesArticleFetchFailure(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"params": map[string]any{"rss_automation": map[string]any{
		"feed_id": "feed", "item_key": "item", "article_fetch_failed": true,
	}}})
	meta := parseRSSAutomationMeta(payload)
	if meta == nil || !meta.ArticleFetchFailed {
		t.Fatalf("article failure notice lost: %+v", meta)
	}
}
