package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/version"
)

type fakeReleaseProvider struct{ status version.ReleaseStatus }

func (f fakeReleaseProvider) Status() version.ReleaseStatus { return f.status }

func getVersionJSON(t *testing.T, h http.Handler) version.ReleaseStatus {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/version status = %d, want 200", rec.Code)
	}
	var got version.ReleaseStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return got
}

func TestVersionEndpointWithoutChecker(t *testing.T) {
	s := newValidationMemoryStore(t)
	a := newTestAPI(t, s)
	h := a.Handler(security.NewLimiter(10_000, 10_000), nil)

	got := getVersionJSON(t, h)
	if got.Current == "" {
		t.Fatal("current version is empty")
	}
	if got.Latest != "" || got.UpdateAvailable {
		t.Fatalf("without checker latest/update should be empty, got %+v", got)
	}
}

func TestVersionEndpointWithChecker(t *testing.T) {
	s := newValidationMemoryStore(t)
	a := newTestAPI(t, s)
	a.SetReleaseChecker(fakeReleaseProvider{status: version.ReleaseStatus{
		Current:         "v0.2.4",
		Latest:          "v0.9.0",
		UpdateAvailable: true,
	}})
	h := a.Handler(security.NewLimiter(10_000, 10_000), nil)

	got := getVersionJSON(t, h)
	if got.Current != "v0.2.4" || got.Latest != "v0.9.0" || !got.UpdateAvailable {
		t.Fatalf("unexpected status: %+v", got)
	}
}
