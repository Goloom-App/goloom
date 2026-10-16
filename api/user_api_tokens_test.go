package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
)

func TestScopedAPITokenCannotMintFullAccessToken(t *testing.T) {
	f := newEndpointFixture(t)
	bearer, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, "reader", nil, `["read"]`, &f.team.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	f.bearer = bearer

	rec := f.do(t, http.MethodPost, "/v1/me/api-tokens", map[string]any{"name": "x"})
	requireStatus(t, rec, http.StatusForbidden)
}

func TestAPITokenChildExpiryCannotExceedRestrictedParent(t *testing.T) {
	for _, restriction := range []string{"scopes", "team", "both", "renamed", "none", "browser"} {
		for _, expiry := range []string{"later", "equal", "earlier", "omitted", "empty"} {
			t.Run(restriction+"/"+expiry, func(t *testing.T) {
				f := newEndpointFixture(t)
				ctx := context.Background()
				parentExpiry := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
				var scopes []string
				var teamID *string
				if restriction == "scopes" || restriction == "both" || restriction == "renamed" {
					scopes = []string{"read"}
				}
				if restriction == "team" || restriction == "both" {
					teamID = &f.team.ID
				}
				name := "parent"
				if restriction == "browser" || restriction == "renamed" {
					name = domain.WebSessionAPITokenName
				}
				rawScopes, _ := json.Marshal(scopes)
				bearer, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, name, &parentExpiry, string(rawScopes), teamID, "")
				if err != nil {
					t.Fatal(err)
				}
				f.bearer = bearer
				input := map[string]any{"name": "child", "scopes": scopes, "team_id": teamID}
				switch expiry {
				case "later":
					input["expires_at"] = parentExpiry.Add(time.Second).Format(time.RFC3339)
				case "equal":
					input["expires_at"] = parentExpiry.Format(time.RFC3339)
				case "earlier":
					input["expires_at"] = parentExpiry.Add(-time.Hour).Format(time.RFC3339)
				case "empty":
					input["expires_at"] = ""
				}
				before, err := f.store.ListUserAPITokens(ctx, f.user.ID)
				if err != nil {
					t.Fatal(err)
				}
				rec := f.do(t, http.MethodPost, "/v1/me/api-tokens", input)
				restricted := restriction != "none" && restriction != "browser"
				if restricted && (expiry == "later" || expiry == "omitted" || expiry == "empty") {
					requireStatus(t, rec, http.StatusForbidden)
					if rec.Header().Get("X-Error-Code") != "api_token_exceeds_authority" {
						t.Fatalf("error code = %q", rec.Header().Get("X-Error-Code"))
					}
					after, err := f.store.ListUserAPITokens(ctx, f.user.ID)
					if err != nil || len(after) != len(before) {
						t.Fatalf("rejected create changed tokens: before=%d after=%d err=%v", len(before), len(after), err)
					}
					return
				}
				requireStatus(t, rec, http.StatusCreated)
				created := decodeJSON[struct {
					APIToken domain.APIToken `json:"api_token"`
				}](t, rec)
				if created.APIToken.ExpiresAt == nil || restricted && created.APIToken.ExpiresAt.After(parentExpiry) {
					t.Fatalf("child expiry = %v, parent = %v", created.APIToken.ExpiresAt, parentExpiry)
				}
			})
		}
	}
}

// Supply legacy principals that the token-creation API cannot mint today.
type tokenPrincipalStore struct {
	store.Store
	principal domain.AuthenticatedPrincipal
}

func (s tokenPrincipalStore) LookupAPIToken(context.Context, string) (domain.AuthenticatedPrincipal, error) {
	return s.principal, nil
}

func TestAPITokenChildExpiryLegacyPrincipal(t *testing.T) {
	longExpiry := time.Now().UTC().AddDate(0, 0, 100)
	for _, tc := range []struct {
		name         string
		parentExpiry *time.Time
		childExpiry  string
	}{
		{name: "no expiry default"},
		{name: "no expiry explicit", childExpiry: "2099-01-01T00:00:00Z"},
		{name: "default within long parent", parentExpiry: &longExpiry},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEndpointFixture(t)
			principal := domain.AuthenticatedPrincipal{User: f.user, Kind: "oidc", Scopes: []string{"read"}, TokenExpiresAt: tc.parentExpiry}
			authSvc, err := auth.New(context.Background(), config.Config{}, tokenPrincipalStore{Store: f.store, principal: principal})
			if err != nil {
				t.Fatal(err)
			}
			catalog, err := i18n.Load()
			if err != nil {
				t.Fatal(err)
			}
			f.handler = api.New(slog.Default(), f.store, authSvc, provider.NewRegistry(), config.Config{}, nil, catalog, nil, nil).Handler(security.NewLimiter(1000, 1000), nil)
			input := map[string]any{"name": "child", "scopes": []string{"read"}}
			if tc.childExpiry != "" {
				input["expires_at"] = tc.childExpiry
			}
			rec := f.do(t, http.MethodPost, "/v1/me/api-tokens", input)
			requireStatus(t, rec, http.StatusCreated)
			created := decodeJSON[struct {
				APIToken domain.APIToken `json:"api_token"`
			}](t, rec)
			if created.APIToken.ExpiresAt == nil {
				t.Fatal("missing child expiry")
			}
			if tc.parentExpiry != nil && created.APIToken.ExpiresAt.After(*tc.parentExpiry) {
				t.Fatal("child outlives parent")
			}
			if tc.childExpiry != "" && created.APIToken.ExpiresAt.Format(time.RFC3339) != tc.childExpiry {
				t.Fatal("explicit expiry changed")
			}
		})
	}
}

func TestScopedAPITokenCannotRevokeFullAccessToken(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	bearer, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "reader", nil, `["read"]`, &f.team.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	target, meta, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "full-access", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	f.bearer = bearer

	rec := f.do(t, http.MethodDelete, "/v1/me/api-tokens/"+meta.ID, nil)
	requireStatus(t, rec, http.StatusForbidden)
	if _, err := f.store.LookupAPIToken(ctx, target); err != nil {
		t.Fatalf("denied revoke removed token: %v", err)
	}
}

// Create and revoke must enforce the same authority boundary, including for
// administrators (the fixture's owner is an admin).
func TestAPITokenManagementAuthority(t *testing.T) {
	cases := []struct {
		name         string
		callerScopes []string
		targetScopes []string
		callerTeam   string
		targetTeam   string
		allowed      bool
	}{
		{name: "same scopes and team", callerScopes: []string{"read"}, targetScopes: []string{"read"}, callerTeam: "a", targetTeam: "a", allowed: true},
		{name: "subset", callerScopes: []string{"read", "write"}, targetScopes: []string{"read"}, allowed: true},
		{name: "write implies draft", callerScopes: []string{"write"}, targetScopes: []string{"write:draft"}, allowed: true},
		{name: "write implies schedule", callerScopes: []string{"write"}, targetScopes: []string{"write:schedule"}, allowed: true},
		{name: "delete implies draft and schedule", callerScopes: []string{"delete"}, targetScopes: []string{"delete:draft", "delete:schedule"}, allowed: true},
		{name: "draft cannot grant write", callerScopes: []string{"write:draft"}, targetScopes: []string{"write"}},
		{name: "leaves cannot grant write", callerScopes: []string{"write:draft", "write:schedule"}, targetScopes: []string{"write"}},
		{name: "draft cannot grant schedule", callerScopes: []string{"write:draft"}, targetScopes: []string{"write:schedule"}},
		{name: "delete draft cannot grant delete", callerScopes: []string{"delete:draft"}, targetScopes: []string{"delete"}},
		{name: "read cannot grant write", callerScopes: []string{"read"}, targetScopes: []string{"read", "write"}},
		{name: "write does not imply read", callerScopes: []string{"write"}, targetScopes: []string{"read"}},
		{name: "scoped cannot grant full access", callerScopes: []string{"read"}},
		{name: "all explicit scopes still cannot grant full access", callerScopes: auth.AllScopes},
		{name: "full access can grant full access", allowed: true},
		{name: "full access can grant subset", targetScopes: []string{"read"}, allowed: true},
		{name: "bound full access same team", callerTeam: "a", targetTeam: "a", allowed: true},
		{name: "bound full access cannot unbind", callerTeam: "a"},
		{name: "bound full access cannot change team", callerTeam: "a", targetTeam: "b"},
		{name: "bound scoped cannot unbind", callerScopes: []string{"read"}, targetScopes: []string{"read"}, callerTeam: "a"},
		{name: "bound scoped cannot change team", callerScopes: []string{"read"}, targetScopes: []string{"read"}, callerTeam: "a", targetTeam: "b"},
		{name: "unbound scoped cannot change binding", callerScopes: []string{"read"}, targetScopes: []string{"read"}, targetTeam: "a"},
		{name: "unrestricted can choose binding", targetTeam: "a", allowed: true},
	}
	for _, tc := range cases {
		for _, action := range []string{"create", "revoke"} {
			t.Run(tc.name+"/"+action, func(t *testing.T) {
				f := newEndpointFixture(t)
				ctx := context.Background()
				otherTeam, err := f.store.CreateTeam(ctx, f.user.ID, domain.CreateTeamInput{Name: "other"})
				if err != nil {
					t.Fatal(err)
				}
				bindings := map[string]*string{"a": &f.team.ID, "b": &otherTeam.ID}
				callerScopes, _ := json.Marshal(tc.callerScopes)
				bearer, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "caller", nil, string(callerScopes), bindings[tc.callerTeam], "")
				if err != nil {
					t.Fatal(err)
				}
				f.bearer = bearer
				var rec *httptest.ResponseRecorder
				var target string
				if action == "create" {
					rec = f.do(t, http.MethodPost, "/v1/me/api-tokens", map[string]any{
						"name": "child", "scopes": tc.targetScopes, "team_id": bindings[tc.targetTeam],
						"expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
					})
				} else {
					scopes, _ := json.Marshal(tc.targetScopes)
					var meta domain.APIToken
					target, meta, err = f.store.CreateUserAPIToken(ctx, f.user.ID, "target", nil, string(scopes), bindings[tc.targetTeam], "")
					if err != nil {
						t.Fatal(err)
					}
					rec = f.do(t, http.MethodDelete, "/v1/me/api-tokens/"+meta.ID, nil)
				}
				if !tc.allowed {
					requireStatus(t, rec, http.StatusForbidden)
					if code := rec.Header().Get("X-Error-Code"); code != "api_token_exceeds_authority" {
						t.Fatalf("error code = %q", code)
					}
					if action == "revoke" {
						if _, err := f.store.LookupAPIToken(ctx, target); err != nil {
							t.Fatalf("denied revoke removed token: %v", err)
						}
					} else {
						tokens, err := f.store.ListUserAPITokens(ctx, f.user.ID)
						if err != nil || len(tokens) != 2 {
							t.Fatalf("denied create changed tokens: count=%d err=%v", len(tokens), err)
						}
					}
					return
				}
				if action == "create" {
					requireStatus(t, rec, http.StatusCreated)
					created := decodeJSON[struct {
						Token string `json:"token"`
					}](t, rec)
					principal, err := f.store.LookupAPIToken(ctx, created.Token)
					if err != nil {
						t.Fatal(err)
					}
					if !slices.Equal(principal.Scopes, tc.targetScopes) || !equalTeamIDs(principal.TokenTeamID, bindings[tc.targetTeam]) {
						t.Fatalf("created scopes=%v team=%v", principal.Scopes, principal.TokenTeamID)
					}
				} else {
					requireStatus(t, rec, http.StatusNoContent)
					if _, err := f.store.LookupAPIToken(ctx, target); !errors.Is(err, sql.ErrNoRows) {
						t.Fatalf("revoked token still usable: %v", err)
					}
				}
			})
		}
	}
}

func equalTeamIDs(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func TestAPITokenCreateNormalizesBeforeAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scopes []string
		team   string
		status int
	}{
		{"normalized subset", []string{" read ", "read", " "}, "same", http.StatusCreated},
		{"empty scopes", []string{}, "same", http.StatusForbidden},
		{"blank scopes", []string{" ", ""}, "same", http.StatusForbidden},
		{"blank team", []string{"read"}, " ", http.StatusForbidden},
		{"unknown scope", []string{"root"}, "same", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEndpointFixture(t)
			bearer, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, "reader", nil, `["read"]`, &f.team.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			f.bearer = bearer
			team := tc.team
			if team == "same" {
				team = " " + f.team.ID + " "
			}
			rec := f.do(t, http.MethodPost, "/v1/me/api-tokens", map[string]any{"name": "x", "scopes": tc.scopes, "team_id": team, "expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339)})
			requireStatus(t, rec, tc.status)
			if tc.status == http.StatusCreated {
				created := decodeJSON[struct {
					APIToken domain.APIToken `json:"api_token"`
				}](t, rec)
				if !slices.Equal(created.APIToken.Scopes, []string{"read"}) || !equalTeamIDs(created.APIToken.TeamID, &f.team.ID) {
					t.Fatalf("created token not normalized: %+v", created.APIToken)
				}
			}
		})
	}
}

func TestAPITokenManagementBrowserSession(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	session, _, err := f.store.CreateSessionAPIToken(ctx, f.user.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, bound := range []bool{false, true} {
		t.Run(fmt.Sprint("bound=", bound), func(t *testing.T) {
			input := map[string]any{"name": "browser-created"}
			if bound {
				input["team_id"] = f.team.ID
				input["scopes"] = []string{"read"}
			}
			raw, _ := json.Marshal(input)
			rec := doBrowserTokenRequest(f.handler, session, http.MethodPost, "/v1/me/api-tokens", raw)
			requireStatus(t, rec, http.StatusCreated)
			created := decodeJSON[struct {
				Token    string          `json:"token"`
				APIToken domain.APIToken `json:"api_token"`
			}](t, rec)
			if bound != (created.APIToken.TeamID != nil) || !bound && len(created.APIToken.Scopes) != 0 {
				t.Fatalf("unexpected browser token: %+v", created.APIToken)
			}
			rec = doBrowserTokenRequest(f.handler, session, http.MethodDelete, "/v1/me/api-tokens/"+created.APIToken.ID, nil)
			requireStatus(t, rec, http.StatusNoContent)
			if _, err := f.store.LookupAPIToken(ctx, created.Token); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("revoked token still usable: %v", err)
			}
		})
	}
}

func doBrowserTokenRequest(handler http.Handler, session, method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "test-csrf"})
	req.Header.Set("X-CSRF-Token", "test-csrf")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAPITokenManagementUnauthenticated(t *testing.T) {
	f := newEndpointFixture(t)
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		path := "/v1/me/api-tokens"
		if method == http.MethodDelete {
			path += "/unknown"
		}
		req := httptest.NewRequest(method, path, strings.NewReader(`{"name":"x"}`))
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		requireStatus(t, rec, http.StatusUnauthorized)
	}
}

func TestAPITokenRevokeSelfAndOwnership(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	bearer, meta, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "reader", nil, `["read"]`, &f.team.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	f.bearer = bearer
	requireStatus(t, f.do(t, http.MethodDelete, "/v1/me/api-tokens/unknown", nil), http.StatusNotFound)
	other, err := f.store.UpsertOIDCUser(ctx, "other", "other@example.test", "Other")
	if err != nil {
		t.Fatal(err)
	}
	otherBearer, otherMeta, err := f.store.CreateUserAPIToken(ctx, other.ID, "other-reader", nil, `["read"]`, &f.team.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, f.do(t, http.MethodDelete, "/v1/me/api-tokens/"+otherMeta.ID, nil), http.StatusNotFound)
	if _, err := f.store.LookupAPIToken(ctx, otherBearer); err != nil {
		t.Fatalf("other user's token removed: %v", err)
	}
	requireStatus(t, f.do(t, http.MethodDelete, "/v1/me/api-tokens/"+meta.ID, nil), http.StatusNoContent)
	if _, err := f.store.LookupAPIToken(ctx, bearer); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("self-revoked token still usable: %v", err)
	}
}

// Authorization must never fall back to revoking without target metadata.
type failingTokenManagementStore struct {
	store.Store
	failList bool
	revoked  bool
}

func (s *failingTokenManagementStore) ListUserAPITokens(ctx context.Context, userID string) ([]domain.APIToken, error) {
	if s.failList {
		return nil, errors.New("private database failure")
	}
	return s.Store.ListUserAPITokens(ctx, userID)
}

func (s *failingTokenManagementStore) RevokeUserAPIToken(context.Context, string, string) error {
	s.revoked = true
	return errors.New("private database failure")
}

func TestAPITokenRevokeStoreFailures(t *testing.T) {
	for _, failList := range []bool{true, false} {
		t.Run(fmt.Sprint("list=", failList), func(t *testing.T) {
			f := newEndpointFixture(t)
			ctx := context.Background()
			target, meta, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "target", nil, "", nil, "")
			if err != nil {
				t.Fatal(err)
			}
			wrapped := &failingTokenManagementStore{Store: f.store, failList: failList}
			authSvc, err := auth.New(ctx, config.Config{}, f.store)
			if err != nil {
				t.Fatal(err)
			}
			catalog, err := i18n.Load()
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			f.handler = api.New(logger, wrapped, authSvc, provider.NewRegistry(), config.Config{}, nil, catalog, nil, nil).Handler(security.NewLimiter(1000, 1000), nil)

			rec := f.do(t, http.MethodDelete, "/v1/me/api-tokens/"+meta.ID, nil)
			requireStatus(t, rec, http.StatusInternalServerError)
			if code := rec.Header().Get("X-Error-Code"); code != "internal_error" {
				t.Fatalf("error code = %q", code)
			}
			if strings.Contains(rec.Body.String(), "private database failure") || !strings.Contains(logs.String(), "private database failure") {
				t.Fatalf("failure must be logged without leaking into response: body=%q logs=%q", rec.Body.String(), logs.String())
			}
			if wrapped.revoked == failList {
				t.Fatalf("revoke attempted = %v with list failure = %v", wrapped.revoked, failList)
			}
			if _, err := f.store.LookupAPIToken(ctx, target); err != nil {
				t.Fatalf("failed revoke removed token: %v", err)
			}
		})
	}
}
