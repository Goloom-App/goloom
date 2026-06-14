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

func TestScopeSatisfiedHierarchy(t *testing.T) {
	cases := []struct {
		granted  []string
		required string
		want     bool
	}{
		{[]string{ScopeWrite}, ScopeWriteDraft, true},
		{[]string{ScopeWrite}, ScopeWriteSchedule, true},
		{[]string{ScopeWriteDraft}, ScopeWriteSchedule, false},
		{[]string{ScopeWriteDraft}, ScopeWrite, false},
		{[]string{ScopeDelete}, ScopeDeleteDraft, true},
		{[]string{ScopeDelete}, ScopeDeleteSchedule, true},
		{[]string{ScopeWrite}, ScopeRead, false},
		{[]string{ScopeRead}, ScopeRead, true},
	}
	for _, c := range cases {
		if got := ScopeSatisfied(c.granted, c.required); got != c.want {
			t.Errorf("ScopeSatisfied(%v, %q) = %v, want %v", c.granted, c.required, got, c.want)
		}
	}
}

func TestPrincipalAllows(t *testing.T) {
	// Unscoped token: full access (backward compatible).
	if !PrincipalAllows(domain.AuthenticatedPrincipal{Kind: "api_token"}, ScopeDelete) {
		t.Fatal("unscoped token must be allowed")
	}
	// Browser session bypasses scope checks entirely.
	if !PrincipalAllows(domain.AuthenticatedPrincipal{Kind: "oidc"}, ScopeWrite) {
		t.Fatal("oidc principal must bypass scopes")
	}
	// Scoped token is restricted to its grants.
	scoped := domain.AuthenticatedPrincipal{Kind: "api_token", Scopes: []string{ScopeRead}}
	if !PrincipalAllows(scoped, ScopeRead) {
		t.Fatal("read token must allow read")
	}
	if PrincipalAllows(scoped, ScopeWriteDraft) {
		t.Fatal("read-only token must not allow write:draft")
	}
}

func TestRequireTokenScopeRejects(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/teams/x/posts", nil)
	req = req.WithContext(security.WithPrincipal(req.Context(), domain.AuthenticatedPrincipal{
		Kind:   "api_token",
		Scopes: []string{ScopeRead},
	}))
	rec := httptest.NewRecorder()

	RequireTokenScope(ScopeWriteSchedule)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "scope_required") {
		t.Fatalf("expected scope_required body, got %q", rec.Body.String())
	}
}

func TestRequireTokenScopeAllowsViaHierarchy(t *testing.T) {
	called := false
	req := httptest.NewRequest(http.MethodPost, "/v1/teams/x/posts", nil)
	req = req.WithContext(security.WithPrincipal(req.Context(), domain.AuthenticatedPrincipal{
		Kind:   "api_token",
		Scopes: []string{ScopeWrite},
	}))
	rec := httptest.NewRecorder()

	RequireTokenScope(ScopeWriteSchedule)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !called {
		t.Fatalf("broad write scope must satisfy write:schedule (status=%d called=%v)", rec.Code, called)
	}
}

func TestRequireTokenScopeUnscopedAllows(t *testing.T) {
	ctx := context.Background()
	service, plaintext := newScopeTestService(t, ctx, "", nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/teams", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()

	service.RequireAuth(RequireTokenScope(ScopeWrite)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unscoped token must pass scope gate: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestPrincipalHasTeamAccessTeamBinding(t *testing.T) {
	ctx := context.Background()
	enc, err := security.NewEncrypter("auth-scope-test-secret-32bytes!")
	if err != nil {
		t.Fatal(err)
	}
	store, err := sqlite.New(ctx, "file:"+uuid.NewString()+"?mode=memory&cache=shared", enc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	admin, err := store.UpsertOIDCUser(ctx, "admin-"+uuid.NewString(), "admin@x", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	teamA, err := store.CreateTeam(ctx, admin.ID, domain.CreateTeamInput{Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	teamB, err := store.CreateTeam(ctx, admin.ID, domain.CreateTeamInput{Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{store: store}

	bound := domain.AuthenticatedPrincipal{User: admin, Kind: "api_token", TokenTeamID: &teamA.ID}
	if ok, _ := svc.PrincipalHasTeamAccess(ctx, bound, teamA.ID, domain.RoleOwner); !ok {
		t.Fatal("team-bound token must access its own team")
	}
	if ok, _ := svc.PrincipalHasTeamAccess(ctx, bound, teamB.ID, domain.RoleOwner); ok {
		t.Fatal("team-bound token must be denied on a foreign team, even as admin")
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
	plaintext, _, err := store.CreateUserAPIToken(ctx, user.ID, "scope-test", nil, scopes, teamID, "")
	if err != nil {
		t.Fatal(err)
	}
	return &Service{store: store}, plaintext
}
