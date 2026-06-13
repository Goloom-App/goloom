package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

type authFixture struct {
	store   *sqlite.Store
	service *auth.Service
}

func newAuthFixture(t *testing.T) authFixture {
	t.Helper()
	ctx := context.Background()
	enc, err := security.NewEncrypter("auth-test-secret-32-bytes-long!!!")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.New(ctx, "file:"+uuid.NewString()+"?mode=memory&cache=shared", enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	service, err := auth.New(ctx, config.Config{}, s)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	// The first user of a fresh database becomes admin; burn that slot so
	// users created by tests are regular users.
	if _, err := s.UpsertOIDCUser(ctx, "seed-admin", "seed@test", "Seed"); err != nil {
		t.Fatal(err)
	}
	return authFixture{store: s, service: service}
}

func (f authFixture) sessionToken(t *testing.T, userID string) string {
	t.Helper()
	token, _, err := f.store.CreateSessionAPIToken(context.Background(), userID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSessionAPIToken: %v", err)
	}
	return token
}

func (f authFixture) user(t *testing.T) domain.User {
	t.Helper()
	u, err := f.store.UpsertOIDCUser(context.Background(), "auth-"+uuid.NewString(), "auth@test", "Auth")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func okHandler(t *testing.T, sawPrincipal *domain.AuthenticatedPrincipal) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sawPrincipal != nil {
			if p := auth.PrincipalFromContext(r.Context()); p != nil {
				*sawPrincipal = *p
			}
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestNewWithoutOIDCConfig(t *testing.T) {
	f := newAuthFixture(t)
	if f.service.OIDCOAuthReady() {
		t.Fatal("OIDC must not be ready without issuer/client id")
	}
	if _, err := f.service.OIDCAuthCodeURL("state", "nonce", "verifier"); err == nil {
		t.Fatal("OIDCAuthCodeURL must fail when oauth is not configured")
	}
	if _, _, err := f.service.OIDCExchangeCode(context.Background(), "code", "nonce", "verifier"); err == nil {
		t.Fatal("OIDCExchangeCode must fail when oauth is not configured")
	}
}

func TestRequireAuth(t *testing.T) {
	f := newAuthFixture(t)
	u := f.user(t)
	token := f.sessionToken(t, u.ID)

	var principal domain.AuthenticatedPrincipal
	handler := f.service.RequireAuth(okHandler(t, &principal))

	t.Run("MissingToken", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("InvalidToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer not-a-real-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("ValidSessionToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if principal.User.ID != u.ID {
			t.Fatalf("principal user = %q, want %q", principal.User.ID, u.ID)
		}
	})
}

func TestAcceptQueryTokenPromotesToken(t *testing.T) {
	f := newAuthFixture(t)
	u := f.user(t)
	token := f.sessionToken(t, u.ID)

	handler := f.service.AcceptQueryToken("token")(f.service.RequireAuth(okHandler(t, nil)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?token="+token, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("query token must authenticate, got %d", rec.Code)
	}

	// An existing Authorization header must win over the query parameter.
	req := httptest.NewRequest(http.MethodGet, "/?token="+token, nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("header token must take precedence, got %d", rec.Code)
	}
}

func TestRequireTeamRole(t *testing.T) {
	f := newAuthFixture(t)
	owner := f.user(t)
	outsider := f.user(t)
	team, err := f.store.CreateTeam(context.Background(), owner.ID, domain.CreateTeamInput{Name: "auth-team-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /teams/{teamID}", f.service.RequireAuth(
		f.service.RequireTeamRole("teamID", domain.RoleOwner, domain.RoleEditor)(okHandler(t, nil)),
	))

	call := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, "/teams/"+team.ID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call(f.sessionToken(t, owner.ID)); code != http.StatusOK {
		t.Fatalf("owner must pass, got %d", code)
	}
	if code := call(f.sessionToken(t, outsider.ID)); code != http.StatusForbidden {
		t.Fatalf("non-member must be forbidden, got %d", code)
	}

	admin, err := f.store.SetUserAdmin(context.Background(), outsider.ID, true)
	if err != nil || !admin.IsAdmin {
		t.Fatalf("SetUserAdmin: %v", err)
	}
	if code := call(f.sessionToken(t, admin.ID)); code != http.StatusOK {
		t.Fatalf("global admin must pass any team, got %d", code)
	}
}

func TestRequireTeamRoleWithoutPrincipal(t *testing.T) {
	f := newAuthFixture(t)
	handler := f.service.RequireTeamRole("teamID")(okHandler(t, nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing principal must yield 401, got %d", rec.Code)
	}
}

func TestRequireAdmin(t *testing.T) {
	f := newAuthFixture(t)
	regular := f.user(t)
	adminUser := f.user(t)
	if _, err := f.store.SetUserAdmin(context.Background(), adminUser.ID, true); err != nil {
		t.Fatal(err)
	}

	handler := f.service.RequireAuth(f.service.RequireAdmin(okHandler(t, nil)))
	call := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call(f.sessionToken(t, regular.ID)); code != http.StatusForbidden {
		t.Fatalf("regular user must be forbidden, got %d", code)
	}
	if code := call(f.sessionToken(t, adminUser.ID)); code != http.StatusOK {
		t.Fatalf("admin must pass, got %d", code)
	}

	rec := httptest.NewRecorder()
	f.service.RequireAdmin(okHandler(t, nil)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing principal must yield 401, got %d", rec.Code)
	}
}

func TestCurrentPrincipal(t *testing.T) {
	f := newAuthFixture(t)
	u := f.user(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := f.service.CurrentPrincipal(req); err == nil {
		t.Fatal("request without principal must error")
	}

	ctx := security.WithPrincipal(req.Context(), domain.AuthenticatedPrincipal{User: u, Kind: "oidc"})
	principal, err := f.service.CurrentPrincipal(req.WithContext(ctx))
	if err != nil || principal.User.ID != u.ID {
		t.Fatalf("CurrentPrincipal: %+v %v", principal, err)
	}
}

func TestRequireTokenScope(t *testing.T) {
	f := newAuthFixture(t)
	u := f.user(t)
	handler := auth.RequireTokenScope(auth.ScopeWrite)(okHandler(t, nil))

	serveAs := func(p *domain.AuthenticatedPrincipal) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if p != nil {
			req = req.WithContext(security.WithPrincipal(req.Context(), *p))
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := serveAs(nil); code != http.StatusUnauthorized {
		t.Fatalf("no principal: got %d, want 401", code)
	}
	// Unscoped token: full access (backward compatible).
	if code := serveAs(&domain.AuthenticatedPrincipal{User: u, Kind: "api_token"}); code != http.StatusOK {
		t.Fatalf("unscoped token: got %d, want 200", code)
	}
	// Scoped token lacking the required scope is rejected.
	if code := serveAs(&domain.AuthenticatedPrincipal{User: u, Kind: "api_token", Scopes: []string{auth.ScopeRead}}); code != http.StatusForbidden {
		t.Fatalf("read-only token: got %d, want 403", code)
	}
	if code := serveAs(&domain.AuthenticatedPrincipal{User: u, Kind: "api_token", Scopes: []string{auth.ScopeWrite}}); code != http.StatusOK {
		t.Fatalf("token with scope: got %d, want 200", code)
	}
	if code := serveAs(&domain.AuthenticatedPrincipal{User: u, Kind: "oidc"}); code != http.StatusOK {
		t.Fatalf("oidc principals bypass scopes: got %d, want 200", code)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	auth.WriteJSON(rec, http.StatusTeapot, map[string]string{"hello": "world"})
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type = %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["hello"] != "world" {
		t.Fatalf("body = %q err=%v", rec.Body.String(), err)
	}
}
