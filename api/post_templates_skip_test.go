package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/scheduler"
	"git.f4mily.net/goloom/internal/security"
)

func postSkip(t *testing.T, a *API, u domain.User, teamID, templateID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+teamID+"/post-templates/"+templateID+"/skip", bytes.NewReader(raw))
	req.SetPathValue("teamID", teamID)
	req.SetPathValue("templateID", templateID)
	req = req.WithContext(security.WithPrincipal(context.Background(), domain.AuthenticatedPrincipal{User: u}))
	rec := httptest.NewRecorder()
	a.handleSkipPostTemplateOccurrence(rec, req)
	return rec
}

// Skip must go through the scheduler so the occurrence is actually torn down and
// re-walked, not merely recorded as a skip row.
func TestPostTemplates_skipRoutesThroughScheduler(t *testing.T) {
	s := newValidationMemoryStore(t)
	a := newTestAPI(t, s)
	occ := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	var gotOcc time.Time
	var gotShift *time.Time
	called := false
	a.metricsSync = &regenerateStub{
		skipFn: func(ctx context.Context, teamID, templateID string, occurrenceAt time.Time, shiftTo *time.Time) (domain.PostTemplateRegenerateResult, error) {
			called = true
			gotOcc, gotShift = occurrenceAt, shiftTo
			return domain.PostTemplateRegenerateResult{DeletedPosts: 6, RegeneratedOccurrences: 3}, nil
		},
	}

	ctx := context.Background()
	u, team, acc := seedRegenerateTemplateFixtures(t, s, ctx)
	tmpl := createRegenerateTestTemplate(t, a, u, team, acc)

	rec := postSkip(t, a, u, team.ID, tmpl.ID, map[string]any{"occurrence_at": occ.Format(time.RFC3339)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("skip did not reach the scheduler")
	}
	if !gotOcc.Equal(occ) {
		t.Errorf("occurrence = %s, want %s", gotOcc, occ)
	}
	if gotShift != nil {
		t.Errorf("expected a pure skip, got shift to %s", gotShift)
	}
	var out domain.PostTemplateRegenerateResult
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.DeletedPosts != 6 || out.RegeneratedOccurrences != 3 {
		t.Errorf("unexpected result: %+v", out)
	}
}

func TestPostTemplates_skipWithShiftPassesTarget(t *testing.T) {
	s := newValidationMemoryStore(t)
	a := newTestAPI(t, s)
	occ := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	shift := time.Date(2026, 9, 5, 18, 0, 0, 0, time.UTC)

	var gotShift *time.Time
	a.metricsSync = &regenerateStub{
		skipFn: func(ctx context.Context, teamID, templateID string, occurrenceAt time.Time, shiftTo *time.Time) (domain.PostTemplateRegenerateResult, error) {
			gotShift = shiftTo
			return domain.PostTemplateRegenerateResult{}, nil
		},
	}

	ctx := context.Background()
	u, team, acc := seedRegenerateTemplateFixtures(t, s, ctx)
	tmpl := createRegenerateTestTemplate(t, a, u, team, acc)

	rec := postSkip(t, a, u, team.ID, tmpl.ID, map[string]any{
		"occurrence_at": occ.Format(time.RFC3339),
		"shift_to":      shift.Format(time.RFC3339),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if gotShift == nil || !gotShift.Equal(shift) {
		t.Errorf("shift target = %v, want %s", gotShift, shift)
	}
}

// A round that is already published must not be silently torn down.
func TestPostTemplates_skipBlockedByPostedRound(t *testing.T) {
	s := newValidationMemoryStore(t)
	a := newTestAPI(t, s)
	a.metricsSync = &regenerateStub{
		skipFn: func(ctx context.Context, teamID, templateID string, occurrenceAt time.Time, shiftTo *time.Time) (domain.PostTemplateRegenerateResult, error) {
			return domain.PostTemplateRegenerateResult{}, scheduler.ErrRegenerateBlocked
		},
	}

	ctx := context.Background()
	u, team, acc := seedRegenerateTemplateFixtures(t, s, ctx)
	tmpl := createRegenerateTestTemplate(t, a, u, team, acc)

	rec := postSkip(t, a, u, team.ID, tmpl.ID, map[string]any{
		"occurrence_at": time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", rec.Code, rec.Body.String())
	}
	// writeError resolves the code against the locale catalog, so the body
	// carries the resolved message rather than the raw code.
	if !strings.Contains(rec.Body.String(), "Cannot skip") {
		t.Errorf("expected the skip_blocked_posted message, got %s", rec.Body.String())
	}
}

// Without a scheduler the endpoint still records the skip rather than failing.
func TestPostTemplates_skipWithoutSchedulerRecordsOnly(t *testing.T) {
	s := newValidationMemoryStore(t)
	a := newTestAPI(t, s)
	a.metricsSync = nil

	ctx := context.Background()
	u, team, acc := seedRegenerateTemplateFixtures(t, s, ctx)
	tmpl := createRegenerateTestTemplate(t, a, u, team, acc)
	occ := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	rec := postSkip(t, a, u, team.ID, tmpl.ID, map[string]any{"occurrence_at": occ.Format(time.RFC3339)})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d, want 204: %s", rec.Code, rec.Body.String())
	}
	skipped, err := s.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, occ)
	if err != nil {
		t.Fatal(err)
	}
	if !skipped {
		t.Error("skip row was not recorded")
	}
}
