package api_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Read registrations so new routes cannot silently bypass the token policy.
func registeredRoutes(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var routes []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				t.Fatal("route pattern must be literal")
			}
			pattern, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			routes = append(routes, pattern)
			return true
		})
	}
	if len(routes) == 0 {
		t.Fatal("no registrations found")
	}
	return routes
}

func TestTokenScopesEveryMutatingRoute(t *testing.T) {
	for _, pattern := range registeredRoutes(t) {
		method, _, _ := strings.Cut(pattern, " ")
		if method == "GET" || method == "HEAD" || method == "OPTIONS" {
			continue
		}
		t.Run(pattern, func(t *testing.T) {
			f := newEndpointFixture(t)
			bearer, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, "reader", nil, `["read"]`, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			f.bearer = bearer
			method, path, _ := strings.Cut(pattern, " ")
			path = strings.ReplaceAll(path, "{teamID}", f.team.ID)
			path = strings.ReplaceAll(path, "{accountID}", f.account.ID)
			// Any remaining resource ID intentionally misses: scope denial must precede
			// body decoding and resource lookup.
			for strings.Contains(path, "{") {
				start := strings.Index(path, "{")
				end := strings.Index(path[start:], "}") + start
				path = path[:start] + "missing" + path[end+1:]
			}
			readOperation := pattern == "POST /v1/auth/oidc/start" || pattern == "POST /v1/auth/session/token" || pattern == "POST /v1/auth/logout" || pattern == "POST /v1/me/api-tokens" || pattern == "DELETE /v1/me/api-tokens/{tokenID}" || strings.HasSuffix(pattern, "/posts/validate") || strings.HasSuffix(pattern, "/ai/prompt-preview") || strings.HasSuffix(pattern, "/ai/chat")
			rec := f.do(t, method, path, nil)
			denied := rec.Code == http.StatusForbidden && (strings.Contains(rec.Body.String(), "scope_required") || rec.Header().Get("X-Error-Code") == "scope_required")
			if !readOperation && !denied {
				t.Fatalf("read-only token: status=%d body=%s, want 403 scope_required", rec.Code, rec.Body.String())
			}
			if readOperation && denied {
				t.Fatalf("read/delegated operation blocked: %s", rec.Body.String())
			}
			// Broad write/delete scopes suffice for ordinary mutations; privilege
			// management needs an unscoped token. Handlers may still reject empty input.
			privilege := strings.Contains(path, "/me/push-subscriptions") || strings.HasPrefix(path, "/v1/admin/") || pattern == "POST /v1/teams" || pattern == "PATCH /v1/teams/{teamID}" || strings.Contains(path, "/members") || strings.Contains(path, "/invitations") || strings.Contains(path, "/ai-service-config") || strings.Contains(path, "/accounts")
			scopes := `["read","write","delete"]`
			if privilege {
				scopes = ""
			}
			bearer, _, err = f.store.CreateUserAPIToken(context.Background(), f.user.ID, "allowed", nil, scopes, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			f.bearer = bearer
			rec = f.do(t, method, path, nil)
			if rec.Code == http.StatusForbidden && (rec.Header().Get("X-Error-Code") == "scope_required" || strings.Contains(rec.Body.String(), "scope_required")) {
				t.Fatalf("sufficient authority rejected: %s", rec.Body.String())
			}
		})
	}
}

// Cancel only after SSE starts, leaving auth and DB reads fully exercised.
type cancelStreamRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (r cancelStreamRecorder) Flush() { r.ResponseRecorder.Flush(); r.cancel() }

func TestTokenScopesEveryReadRoute(t *testing.T) {
	for _, pattern := range registeredRoutes(t) {
		method, path, _ := strings.Cut(pattern, " ")
		if method != "GET" {
			continue
		}
		public := path == "/healthz" || path == "/v1/version" || path == "/v1/discovery" || path == "/v1/providers" || path == "/v1/auth/status" || strings.HasPrefix(path, "/v1/oauth/")
		crossTeam := path == "/v1/users" || path == "/v1/teams" || path == "/v1/me/review-counts" || path == "/v1/me/team-notification-prefs" || path == "/v1/me/push-subscriptions"
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			for _, kind := range []string{"without read", "team bound", "reader", "browser"} {
				t.Run(method+" "+path+"/"+kind, func(t *testing.T) {
					f := newEndpointFixture(t)
					scopes := `["write:draft"]`
					var teamID *string
					switch kind {
					case "team bound":
						scopes = ""
						teamID = &f.team.ID
					case "reader":
						scopes = `["read"]`
					case "browser":
						scopes = ""
					}
					bearer, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, kind, nil, scopes, teamID, "")
					if kind == "browser" {
						bearer, _, err = f.store.CreateSessionAPIToken(context.Background(), f.user.ID, time.Hour)
					}
					if err != nil {
						t.Fatal(err)
					}
					target := strings.ReplaceAll(path, "{teamID}", f.team.ID)
					target = strings.ReplaceAll(target, "{accountID}", f.account.ID)
					for strings.Contains(target, "{") {
						start := strings.Index(target, "{")
						end := strings.Index(target[start:], "}") + start
						target = target[:start] + "missing" + target[end+1:]
					}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					req := httptest.NewRequest(method, target, nil).WithContext(ctx)
					if kind == "browser" {
						req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: bearer})
					} else {
						req.Header.Set("Authorization", "Bearer "+bearer)
					}
					rec := httptest.NewRecorder()
					f.handler.ServeHTTP(cancelStreamRecorder{rec, cancel}, req)
					denied := rec.Code == http.StatusForbidden && (rec.Header().Get("X-Error-Code") == "scope_required" || strings.Contains(rec.Body.String(), "scope_required"))
					wantDenied := !public && (kind == "without read" || ((strings.HasPrefix(path, "/v1/admin/") || path == "/v1/me/push-subscriptions") && kind != "browser") || (crossTeam && kind == "team bound"))
					if denied != wantDenied {
						t.Fatalf("status=%d body=%s scope denial=%v, want %v", rec.Code, rec.Body.String(), denied, wantDenied)
					}
				})
			}
		}
	}
}

func TestReadTokenAIJobQueryStream(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(strconv.FormatBool(bound), func(t *testing.T) {
			f := newEndpointFixture(t)
			var teamID *string
			if bound {
				teamID = &f.team.ID
			}
			bearer, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, "SSE reader", nil, `["read"]`, teamID, "")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+f.team.ID+"/ai-jobs/stream?token="+bearer, nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			f.handler.ServeHTTP(cancelStreamRecorder{rec, cancel}, req)
			requireStatus(t, rec, http.StatusOK)
			if rec.Header().Get("Content-Type") != "text/event-stream" {
				t.Fatalf("not SSE: %v", rec.Header())
			}
		})
	}
}
