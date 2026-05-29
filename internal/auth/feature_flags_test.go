package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func TestFeatureFlag(t *testing.T) {
	t.Run("AIDisabledReturns403", func(t *testing.T) {
		runFeatureFlagDisabled(t)
	})
	t.Run("AIEnabledPasses", func(t *testing.T) {
		runFeatureFlagEnabled(t)
	})
}

func runFeatureFlagDisabled(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	service, team, principal := newFeatureFlagTestService(t, ctx)

	req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+team.ID+"/ai", nil)
	req.SetPathValue("teamID", team.ID)
	req = req.WithContext(security.WithPrincipal(req.Context(), principal))
	rec := httptest.NewRecorder()

	called := false
	service.RequireAIEnabled("teamID")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("expected next handler not to run")
	}
	if !strings.Contains(rec.Body.String(), "ai_not_enabled") {
		t.Fatalf("expected ai_not_enabled body, got %q", rec.Body.String())
	}
}

func runFeatureFlagEnabled(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	service, team, principal := newFeatureFlagTestService(t, ctx)
	enabled := true
	updated, err := service.store.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{
		Name:        team.Name,
		Description: team.Description,
		IsAIEnabled: &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.IsAIEnabled {
		t.Fatal("expected team flag enabled")
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+team.ID+"/ai", nil)
	req.SetPathValue("teamID", team.ID)
	req = req.WithContext(security.WithPrincipal(req.Context(), principal))
	rec := httptest.NewRecorder()

	called := false
	service.RequireAIEnabled("teamID")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("expected next handler to run")
	}
}

func newFeatureFlagTestService(t *testing.T, ctx context.Context) (*Service, domain.Team, domain.AuthenticatedPrincipal) {
	t.Helper()
	enc, err := security.NewEncrypter("feature-flag-test-secret-32bytes")
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.New(ctx, "file:"+uuid.NewString()+"?mode=memory&cache=shared", enc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	owner, err := store.UpsertOIDCUser(ctx, "flag-owner-"+uuid.NewString(), "owner@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := store.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{
		Name:        "flag-team-" + uuid.NewString(),
		Description: "feature flag test team",
	})
	if err != nil {
		t.Fatal(err)
	}

	return &Service{store: store}, team, domain.AuthenticatedPrincipal{User: owner, Kind: "api_token"}
}
