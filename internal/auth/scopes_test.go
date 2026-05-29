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

func TestScope(t *testing.T) {
	t.Run("RequireScopeRejects", runRequireScopeRejects)
	t.Run("RequireScopeAllows", runRequireScopeAllows)
	t.Run("BackwardCompat", runBackwardCompat)
	t.Run("OIDCBypassScopes", runOIDCBypassScopes)
}

func TestRequireScopeRejects(t *testing.T) {
	runRequireScopeRejects(t)
}

func TestRequireScopeAllows(t *testing.T) {
	runRequireScopeAllows(t)
}

func TestBackwardCompat(t *testing.T) {
	runBackwardCompat(t)
}

func TestOIDCBypassScopes(t *testing.T) {
	runOIDCBypassScopes(t)
}

func runRequireScopeRejects(t *testing.T) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/ai/context", nil)
	req = req.WithContext(security.WithPrincipal(req.Context(), domain.AuthenticatedPrincipal{
		Kind:   "api_token",
		Scopes: []string{ScopeAIReadContext},
	}))
	rec := httptest.NewRecorder()

	RequireScope(ScopeAIWriteDrafts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ai_scope_required") {
		t.Fatalf("expected ai_scope_required body, got %q", rec.Body.String())
	}
}

func runRequireScopeAllows(t *testing.T) {
	t.Helper()
	called := false
	req := httptest.NewRequest(http.MethodGet, "/v1/ai/context", nil)
	req = req.WithContext(security.WithPrincipal(req.Context(), domain.AuthenticatedPrincipal{
		Kind:   "api_token",
		Scopes: []string{ScopeAIReadContext, ScopeAIWriteDrafts},
	}))
	rec := httptest.NewRecorder()

	RequireScope(ScopeAIReadContext, ScopeAIWriteDrafts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func runBackwardCompat(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	service, plaintext := newScopeTestService(t, ctx, "", nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/teams", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()

	service.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func runOIDCBypassScopes(t *testing.T) {
	t.Helper()
	called := false
	req := httptest.NewRequest(http.MethodPost, "/v1/ai/jobs", nil)
	req = req.WithContext(security.WithPrincipal(req.Context(), domain.AuthenticatedPrincipal{Kind: "oidc"}))
	rec := httptest.NewRecorder()

	RequireScope(ScopeAITriggerJobs)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !called {
		t.Fatal("expected oidc request to bypass scope check")
	}
}

func newScopeTestService(t *testing.T, ctx context.Context, scopes string, teamID *string) (*Service, string) {
	t.Helper()
	enc, err := security.NewEncrypter("auth-scope-test-secret-32bytes!")
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.New(ctx, "file:"+uuid.NewString()+"?mode=memory&cache=shared", enc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	user, err := store.UpsertOIDCUser(ctx, "scope-user-"+uuid.NewString(), "scope@example.test", "Scope User")
	if err != nil {
		t.Fatal(err)
	}
	plaintext, _, err := store.CreateUserAPIToken(ctx, user.ID, "scope-test", nil, scopes, teamID)
	if err != nil {
		t.Fatal(err)
	}
	return &Service{store: store}, plaintext
}
