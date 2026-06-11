package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnrichAIJobParams_fetchesLatestRSSItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>Test</title>
<item>
  <title>Talk #381: Local Opus vs Cloud Opus</title>
  <link>https://example.com/talk-381</link>
  <description><![CDATA[<p>WireGuard, Headscale und Mesh-VPNs.</p>]]></description>
</item>
</channel></rss>`))
	}))
	defer server.Close()

	raw, err := json.Marshal(map[string]any{
		"occasion":      "Promote the latest episode.",
		"rss_feed_url":  server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}

	enriched, err := enrichAIJobParams(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}

	var params map[string]any
	if err := json.Unmarshal(enriched, &params); err != nil {
		t.Fatal(err)
	}
	if params["rss_article_title"] != "Talk #381: Local Opus vs Cloud Opus" {
		t.Fatalf("title = %v", params["rss_article_title"])
	}
	if params["rss_article_link"] != "https://example.com/talk-381" {
		t.Fatalf("link = %v", params["rss_article_link"])
	}
	content := stringParam(params["rss_article_content"])
	if !strings.Contains(content, "WireGuard") {
		t.Fatalf("content = %q", content)
	}
}
