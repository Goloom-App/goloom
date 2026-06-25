package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A non-2xx response must surface the HTTP status code, so a failed fetch is
// diagnosable instead of an opaque "url fetch failed" (the niri link the agent
// could not load gave no clue why).
func TestFetchURLBody_SurfacesStatusCode(t *testing.T) {
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
