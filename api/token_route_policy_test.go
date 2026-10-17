package api_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Read registrations rather than maintaining a route list: every new unsafe
// route must pass this test, including registrations that bypass authentication.
func TestTokenScopesEveryMutatingRoute(t *testing.T) {
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
			method, _, _ := strings.Cut(pattern, " ")
			if method != "GET" && method != "HEAD" && method != "OPTIONS" {
				routes = append(routes, pattern)
			}
			return true
		})
	}
	if len(routes) == 0 {
		t.Fatal("no mutating registrations found")
	}
	t.Logf("checking %d registered mutating routes", len(routes))
	for _, pattern := range routes {
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
			privilege := strings.HasPrefix(path, "/v1/admin/") || pattern == "POST /v1/teams" || pattern == "PATCH /v1/teams/{teamID}" || strings.Contains(path, "/members") || strings.Contains(path, "/invitations") || strings.Contains(path, "/ai-service-config") || strings.Contains(path, "/accounts")
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
