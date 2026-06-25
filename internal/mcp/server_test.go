package mcp

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpFixture struct {
	store   *sqlite.Store
	handler *Handler
	user    domain.User
	team    domain.Team
	account domain.SocialAccount
}

func newMCPFixture(t *testing.T) mcpFixture {
	t.Helper()
	ctx := context.Background()
	enc, err := security.NewEncrypter("mcp-test-secret-32-bytes-long!!!!")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.New(ctx, "file:"+uuid.NewString()+"?mode=memory&cache=shared", enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	authSvc, err := auth.New(ctx, config.Config{}, s)
	if err != nil {
		t.Fatal(err)
	}
	// Burn the first-user-is-admin slot so test users are regular users.
	if _, err := s.UpsertOIDCUser(ctx, "seed-admin", "seed@mcp.test", "Seed"); err != nil {
		t.Fatal(err)
	}
	u, err := s.UpsertOIDCUser(ctx, "mcp-"+uuid.NewString(), "mcp@test", "MCP")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "mcp-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m.example", Username: "mcp", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := provider.NewRegistry(
		provider.NewBlueskyProvider(),
		provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}),
	)
	handler := NewHandler(logger, s, authSvc, registry, config.Config{})
	return mcpFixture{store: s, handler: handler, user: u, team: team, account: acc}
}

// apiToken creates a personal API token with the given scopes.
func (f mcpFixture) apiToken(t *testing.T, scopes string) string {
	token, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, "mcp-test", nil, scopes, nil, "")
	if err != nil {
		t.Fatalf("CreateUserAPIToken: %v", err)
	}
	return token
}

func TestExtractBearerToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if ExtractBearerToken(req) != "" {
		t.Fatal("no header must yield empty token")
	}
	req.Header.Set("Authorization", "Bearer abc")
	if got := ExtractBearerToken(req); got != "abc" {
		t.Fatalf("token = %q", got)
	}
	req.Header.Set("Authorization", "bearer xyz ")
	if got := ExtractBearerToken(req); got != "xyz" {
		t.Fatalf("case-insensitive token = %q", got)
	}
	req.Header.Set("Authorization", "Basic abc")
	if ExtractBearerToken(req) != "" {
		t.Fatal("non-bearer scheme must yield empty token")
	}
}

func TestServeHTTPAuthGate(t *testing.T) {
	f := newMCPFixture(t)

	call := func(authorize func(*http.Request)) int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		if authorize != nil {
			authorize(req)
		}
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call(nil); code != http.StatusUnauthorized {
		t.Fatalf("missing token: got %d, want 401", code)
	}
	if code := call(func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") }); code != http.StatusUnauthorized {
		t.Fatalf("invalid token: got %d, want 401", code)
	}
	// A scoped token lacking read is rejected at the MCP gate.
	noRead := f.apiToken(t, `["write:draft"]`)
	if code := call(func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+noRead) }); code != http.StatusForbidden {
		t.Fatalf("token without read scope: got %d, want 403", code)
	}
	// Unscoped tokens have full access and pass the gate.
	unscoped := f.apiToken(t, "")
	if code := call(func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+unscoped) }); code == http.StatusUnauthorized || code == http.StatusForbidden {
		t.Fatalf("unscoped token must pass the auth gate, got %d", code)
	}
	// A token with read passes the gate (whatever the MCP transport answers, it
	// must not be the auth layer's 401/403).
	scoped := f.apiToken(t, `["read"]`)
	if code := call(func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+scoped) }); code == http.StatusUnauthorized || code == http.StatusForbidden {
		t.Fatalf("token with read scope must pass the auth gate, got %d", code)
	}
}

// bearerRoundTripper injects a static bearer token on every request, the way a
// real remote MCP client authenticates against goloom.
type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (b bearerRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// TestStreamableTransportEndToEnd drives the handler with a real MCP client over
// the modern Streamable HTTP transport. It guards the contract that broke in
// production (clients hitting the endpoint over the current SDK transport must
// connect and enumerate tools), not just the auth gate.
func TestStreamableTransportEndToEnd(t *testing.T) {
	f := newMCPFixture(t)
	token := f.apiToken(t, `["read"]`)

	srv := httptest.NewServer(f.handler)
	t.Cleanup(srv.Close)

	httpClient := &http.Client{Transport: bearerRoundTripper{token: token, base: http.DefaultTransport}}

	client := mcp.NewClient(&mcp.Implementation{Name: "goloom-test-client", Version: "0"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   srv.URL + "/",
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		t.Fatalf("connect over streamable transport: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools over streamable transport: %v", err)
	}
	if len(tools.Tools) == 0 {
		t.Fatal("expected the server to advertise at least one tool")
	}
}

// TestToolsListWithoutRetainedSession reproduces the agent-reported symptom:
// a tools/list call that does not ride on a server-retained session must still
// succeed instead of failing with "method ... is invalid during session
// initialization". Stateless mode auto-initializes each request, so the server
// tolerates clients/proxies that don't preserve the Mcp-Session-Id across calls.
func TestToolsListWithoutRetainedSession(t *testing.T) {
	f := newMCPFixture(t)
	token := f.apiToken(t, `["read"]`)

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)

	payload := rec.Body.String()
	if strings.Contains(payload, "invalid during session initialization") {
		t.Fatalf("tools/list rejected during init (status %d): %s", rec.Code, payload)
	}
	if !strings.Contains(payload, `"tools"`) {
		t.Fatalf("expected a tools list in the response (status %d): %s", rec.Code, payload)
	}
}
