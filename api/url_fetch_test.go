package api

import (
	"context"
	"git.f4mily.net/goloom/internal/netguard"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A non-2xx response must surface the HTTP status code, so a failed fetch is
// diagnosable instead of an opaque "url fetch failed" (the niri link the agent
// could not load gave no clue why).
func TestFetchURLBody_SurfacesStatusCode(t *testing.T) {
	allowLoopbackFetch(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer server.Close()

	_, err := fetchURLBody(context.Background(), server.URL)
	if err == nil {
		t.Fatal("a 403 response must produce an error")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error must name the HTTP status, got %q", err)
	}
}

// fetch_url targets are model-controlled (and thus injection-controllable), so
// fetches must never reach loopback, private or link-local addresses.
func TestFetchURLBody_BlocksPrivateTargets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("internal secret"))
	}))
	defer server.Close()

	for _, target := range []string{server.URL, "http://169.254.169.254/latest/meta-data/"} {
		body, err := fetchURLBody(context.Background(), target)
		if err == nil {
			t.Fatalf("fetch of %s must be blocked, got body %q", target, body)
		}
	}
}

// allowLoopbackFetch lets a test fetch from its own httptest server.
func allowLoopbackFetch(t *testing.T) {
	t.Helper()
	prev := urlFetchPolicy
	urlFetchPolicy = netguard.NewPolicy([]string{"127.0.0.1"})
	t.Cleanup(func() { urlFetchPolicy = prev })
}
