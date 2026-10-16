package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func TestSessionTokenExchangeAuthority(t *testing.T) {
	allScopes, _ := json.Marshal(auth.AllScopes)
	for _, tc := range []struct {
		name      string
		scopes    string
		bound     bool
		bootstrap bool
		invalid   bool
		status    int
	}{
		{name: "scoped", scopes: `["read"]`, status: http.StatusForbidden},
		{name: "all explicit scopes", scopes: string(allScopes), status: http.StatusForbidden},
		{name: "team bound", bound: true, status: http.StatusForbidden},
		{name: "scoped and team bound", scopes: `["read"]`, bound: true, status: http.StatusForbidden},
		{name: "full authority", status: http.StatusOK},
		{name: "bootstrap recovery", bootstrap: true, status: http.StatusOK},
		{name: "invalid", invalid: true, status: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEndpointFixture(t)
			ctx := context.Background()
			userID := f.user.ID
			var teamID *string
			if tc.bound {
				teamID = &f.team.ID
			}
			token, _, err := f.store.CreateUserAPIToken(ctx, userID, "login", nil, tc.scopes, teamID, "")
			if err != nil {
				t.Fatal(err)
			}
			if tc.bootstrap {
				token = "bootstrap-recovery-token-for-session-test"
				if err := f.store.EnsureBootstrapAdmin(ctx, "recovery@example.test", "Recovery", token); err != nil {
					t.Fatal(err)
				}
				principal, err := f.store.LookupAPIToken(ctx, token)
				if err != nil {
					t.Fatal(err)
				}
				if len(principal.Scopes) != 0 || principal.TokenTeamID != nil {
					t.Fatalf("bootstrap token unexpectedly restricted: %+v", principal)
				}
				userID = principal.User.ID
			}
			if tc.invalid {
				token = "invalid-token"
			}
			before, err := f.store.ListUserAPITokens(ctx, userID)
			if err != nil {
				t.Fatal(err)
			}
			rec := exchangeSessionToken(f.handler, "  "+token+"  ", "en")
			// Check cookies and persisted sessions even if status is wrong.
			if tc.status != http.StatusOK {
				if cookies := rec.Result().Cookies(); len(cookies) != 0 {
					t.Errorf("denied exchange set cookies: %v", cookies)
				}
				after, err := f.store.ListUserAPITokens(ctx, userID)
				if err != nil || len(after) != len(before) {
					t.Errorf("denied exchange created session: before=%d after=%d err=%v", len(before), len(after), err)
				}
			}
			requireStatus(t, rec, tc.status)
			if tc.status == http.StatusForbidden {
				if code := rec.Header().Get("X-Error-Code"); code != "api_token_session_requires_full_access" {
					t.Fatalf("error code = %q", code)
				}
			}
			if tc.status != http.StatusOK {
				return
			}
			var session, csrf *http.Cookie
			for _, cookie := range rec.Result().Cookies() {
				switch cookie.Name {
				case auth.SessionCookieName:
					session = cookie
				case auth.CSRFCookieName:
					csrf = cookie
				}
			}
			if session == nil || session.Value == "" || !session.HttpOnly {
				t.Fatalf("expected HttpOnly session cookie: %+v", session)
			}
			if csrf == nil || csrf.Value == "" || csrf.HttpOnly {
				t.Fatalf("expected readable CSRF cookie: %+v", csrf)
			}
			req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
			req.AddCookie(session)
			me := httptest.NewRecorder()
			f.handler.ServeHTTP(me, req)
			requireStatus(t, me, http.StatusOK)
			if got := decodeJSON[struct {
				User domain.User `json:"user"`
			}](t, me).User.ID; got != userID {
				t.Fatalf("session user = %q, want %q", got, userID)
			}
		})
	}
}

type failingSessionStore struct{ store.Store }

func (s failingSessionStore) CreateSessionAPIToken(context.Context, string, time.Duration) (string, domain.APIToken, error) {
	return "", domain.APIToken{}, errors.New("private session database failure")
}

func TestSessionTokenExchangeStoreFailure(t *testing.T) {
	f := newEndpointFixture(t)
	authSvc, err := auth.New(context.Background(), config.Config{}, f.store)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := api.New(logger, failingSessionStore{f.store}, authSvc, provider.NewRegistry(), config.Config{}, nil, catalog, nil, nil).Handler(security.NewLimiter(1000, 1000), nil)
	rec := exchangeSessionToken(handler, f.bearer, "en")
	requireStatus(t, rec, http.StatusInternalServerError)
	if code := rec.Header().Get("X-Error-Code"); code != "internal_error" {
		t.Errorf("error code = %q", code)
	}
	if strings.Contains(rec.Body.String(), "private session database failure") || !strings.Contains(logs.String(), "private session database failure") {
		t.Errorf("failure must be logged without leaking into response: body=%q logs=%q", rec.Body.String(), logs.String())
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 0 {
		t.Errorf("failed exchange set cookies: %v", cookies)
	}
}

func TestSessionTokenExchangeLocalizedRejection(t *testing.T) {
	f := newEndpointFixture(t)
	token, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, "reader", nil, `["read"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for language, message := range map[string]string{
		"en": "Sign-in requires an API token without scope or team restrictions. Use an unrestricted token or the recovery token.",
		"de": "Die Anmeldung erfordert ein API-Token ohne Berechtigungs- oder Teameinschränkungen. Verwende ein uneingeschränktes Token oder das Wiederherstellungs-Token.",
	} {
		t.Run(language, func(t *testing.T) {
			rec := exchangeSessionToken(f.handler, token, language)
			requireStatus(t, rec, http.StatusForbidden)
			if got := strings.TrimSpace(rec.Body.String()); got != message {
				t.Fatalf("message = %q, want %q", got, message)
			}
		})
	}
}

func TestSessionTokenExchangeInvalidBody(t *testing.T) {
	for _, tc := range []struct {
		body string
		code string
	}{
		{`{`, "invalid_json_body"},
		{`{}`, "token_required"},
		{`{"token":"  "}`, "token_required"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			f := newEndpointFixture(t)
			req := httptest.NewRequest(http.MethodPost, "/v1/auth/session/token", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			f.handler.ServeHTTP(rec, req)
			requireStatus(t, rec, http.StatusBadRequest)
			if code := rec.Header().Get("X-Error-Code"); code != tc.code {
				t.Errorf("error code = %q, want %q", code, tc.code)
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Error("invalid request set cookies")
			}
		})
	}
}

func exchangeSessionToken(handler http.Handler, token, language string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"token": token})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/session/token", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", language)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
