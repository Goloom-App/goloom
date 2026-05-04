package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMastodonCompatibleStatusPayload_spoilerAndSensitive(t *testing.T) {
	t.Parallel()
	st := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	p := mastodonCompatibleStatusPayload(PublishRequest{
		Content:     "hello",
		MediaIDs:    []string{" 1 ", ""},
		Visibility:  "unlisted",
		ScheduledAt: &st,
		SpoilerText: "CW text",
		Sensitive:   true,
	})
	if p["status"] != "hello" {
		t.Fatalf("status: %v", p["status"])
	}
	ids, _ := p["media_ids"].([]string)
	if len(ids) != 1 || ids[0] != "1" {
		t.Fatalf("media_ids: %#v", p["media_ids"])
	}
	if p["visibility"] != "unlisted" {
		t.Fatalf("visibility: %v", p["visibility"])
	}
	if p["spoiler_text"] != "CW text" {
		t.Fatalf("spoiler_text: %v", p["spoiler_text"])
	}
	if p["sensitive"] != true {
		t.Fatalf("sensitive: %v", p["sensitive"])
	}
	if _, ok := p["scheduled_at"]; !ok {
		t.Fatal("expected scheduled_at")
	}
}

func TestUploadMastodonV2Media_immediateWhenURLInCreateResponse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/media" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "9", "url": "http://cdn/x"})
	}))
	defer srv.Close()

	ctx := WithOutboundInstancePolicy(context.Background(), OutboundPolicy{AllowPrivateLAN: true})
	id, err := uploadMastodonV2Media(ctx, srv.URL, "tok", strings.NewReader("x"), "a.png", "image/png", "alt")
	if err != nil {
		t.Fatal(err)
	}
	if id != "9" {
		t.Fatalf("id %q", id)
	}
}

func TestUploadMastodonV2Media_pollsUntilURL(t *testing.T) {
	t.Parallel()
	var getCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/media", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "42"})
	})
	mux.HandleFunc("/api/v1/media/42", func(w http.ResponseWriter, r *http.Request) {
		getCalls++
		w.Header().Set("Content-Type", "application/json")
		if getCalls < 2 {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "42"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "42", "url": "http://cdn/ok"})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := WithOutboundInstancePolicy(context.Background(), OutboundPolicy{AllowPrivateLAN: true})
	id, err := uploadMastodonV2Media(ctx, srv.URL, "tok", strings.NewReader("x"), "a.png", "image/png", "alt")
	if err != nil {
		t.Fatal(err)
	}
	if id != "42" {
		t.Fatalf("id %q", id)
	}
	if getCalls < 2 {
		t.Fatalf("expected at least 2 GET polls, got %d", getCalls)
	}
}
