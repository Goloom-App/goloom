package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUpdateAvailable(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"newer patch", "v0.2.4", "v0.2.5", true},
		{"newer minor", "v0.2.4", "v0.3.0", true},
		{"equal", "v0.2.4", "v0.2.4", false},
		{"older latest", "v0.2.5", "v0.2.4", false},
		{"missing v prefix", "0.2.4", "0.2.5", true},
		{"mixed prefix", "v0.2.4", "0.2.5", true},
		{"empty latest", "v0.2.4", "", false},
		{"dev current never nags", "dev", "v0.2.5", false},
		{"dev revision current never nags", "dev-abc123def456", "v0.2.5", false},
		{"garbage latest", "v0.2.4", "not-a-version", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := updateAvailable(tc.current, tc.latest); got != tc.want {
				t.Fatalf("updateAvailable(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestReleaseCheckerRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v0.9.0","html_url":"https://example/releases/v0.9.0"}`))
	}))
	defer srv.Close()

	c := newReleaseChecker("v0.2.4", srv.URL, time.Hour, http.DefaultClient, nil)

	// Before any refresh the latest is unknown; no false-positive update hint.
	if got := c.Status(); got.Latest != "" || got.UpdateAvailable {
		t.Fatalf("pre-refresh status = %+v, want empty latest and no update", got)
	}

	c.refresh(context.Background())

	got := c.Status()
	if got.Current != "v0.2.4" {
		t.Fatalf("current = %q, want v0.2.4", got.Current)
	}
	if got.Latest != "v0.9.0" {
		t.Fatalf("latest = %q, want v0.9.0", got.Latest)
	}
	if !got.UpdateAvailable {
		t.Fatalf("update_available = false, want true for %+v", got)
	}
}

func TestReleaseCheckerRefreshBadResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := newReleaseChecker("v0.2.4", srv.URL, time.Hour, http.DefaultClient, nil)
	c.refresh(context.Background()) // must not panic

	if got := c.Status(); got.Latest != "" || got.UpdateAvailable {
		t.Fatalf("status after error = %+v, want empty latest and no update", got)
	}
}

func TestReleaseCheckerRunStopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.9.0"}`))
	}))
	defer srv.Close()

	c := newReleaseChecker("v0.2.4", srv.URL, time.Hour, http.DefaultClient, nil)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		c.Run(ctx) // does an immediate refresh, then blocks on the ticker
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
