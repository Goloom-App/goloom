package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	"io"
	"log/slog"
	"strings"
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

// principalContextForTools guards the value/pointer contract between
// ServeHTTP (stores the principal) and principalFromContext (reads it in
// every tool handler). If they disagree, all MCP tools fail "unauthorized".
func TestPrincipalRoundTripsIntoToolHandlers(t *testing.T) {
	f := newMCPFixture(t)
	principal, err := f.store.LookupAPIToken(context.Background(), f.apiToken(t, `["read","write"]`))
	if err != nil {
		t.Fatalf("LookupAPIToken: %v", err)
	}

	// Exactly what ServeHTTP does before delegating to the MCP session.
	ctx := WithPrincipal(context.Background(), principal)

	_, out, err := f.handler.handleGetTeams(ctx, nil, GetTeamsInput{})
	if err != nil {
		t.Fatalf("handleGetTeams must see the principal, got error: %v", err)
	}
	found := false
	for _, team := range out.Teams {
		if team.TeamID == f.team.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected team %s in result, got %+v", f.team.ID, out.Teams)
	}
}

func TestToolHandlersRejectMissingPrincipal(t *testing.T) {
	f := newMCPFixture(t)
	if _, _, err := f.handler.handleGetTeams(context.Background(), nil, GetTeamsInput{}); err == nil {
		t.Fatal("missing principal must be rejected")
	}
	if _, _, err := f.handler.handleDraftPost(context.Background(), nil, DraftPostInput{}); err == nil {
		t.Fatal("missing principal must be rejected")
	}
}

func TestDraftAndModifyPostHandlers(t *testing.T) {
	f := newMCPFixture(t)
	principal, err := f.store.LookupAPIToken(context.Background(), f.apiToken(t, `["read","write"]`))
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPrincipal(context.Background(), principal)

	_, draft, err := f.handler.handleDraftPost(ctx, nil, DraftPostInput{
		TeamID:         f.team.ID,
		Title:          "MCP Draft",
		Content:        "Hallo vom MCP-Test",
		TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatalf("handleDraftPost: %v", err)
	}
	if draft.PostID == "" {
		t.Fatal("draft post id missing")
	}

	post, err := f.store.GetScheduledPost(context.Background(), f.team.ID, draft.PostID)
	if err != nil {
		t.Fatalf("draft not in store: %v", err)
	}
	if post.Status != domain.PostStatusDraft {
		t.Fatalf("status = %s, want draft", post.Status)
	}

	newContent := "Geänderter Inhalt"
	_, modified, err := f.handler.handleModifyPost(ctx, nil, ModifyPostInput{
		TeamID:  f.team.ID,
		PostID:  draft.PostID,
		Content: &newContent,
	})
	if err != nil {
		t.Fatalf("handleModifyPost: %v", err)
	}
	if modified.PostID != draft.PostID {
		t.Fatalf("modify result: %+v", modified)
	}
	post, err = f.store.GetScheduledPost(context.Background(), f.team.ID, draft.PostID)
	if err != nil {
		t.Fatal(err)
	}
	if post.Content != newContent {
		t.Fatalf("content = %q, want %q", post.Content, newContent)
	}
}

func TestGetCalendarHandler(t *testing.T) {
	f := newMCPFixture(t)
	principal, err := f.store.LookupAPIToken(context.Background(), f.apiToken(t, `["read","write"]`))
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPrincipal(context.Background(), principal)

	scheduledAt := time.Now().UTC().Add(24 * time.Hour)
	if _, err := f.store.CreateScheduledPost(context.Background(), f.team.ID, principal, domain.CreatePostInput{
		Content: "Kalendereintrag", ScheduledAt: scheduledAt, TargetAccounts: []string{f.account.ID},
	}); err != nil {
		t.Fatal(err)
	}

	_, out, err := f.handler.handleGetCalendar(ctx, nil, GetCalendarInput{
		TeamID:   f.team.ID,
		FromDate: time.Now().UTC().Format(time.RFC3339),
		ToDate:   time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("handleGetCalendar: %v", err)
	}
	if len(out.Posts) != 1 || out.Posts[0].Content != "Kalendereintrag" {
		t.Fatalf("calendar posts: %+v", out.Posts)
	}
}

func TestParseWeekday(t *testing.T) {
	cases := map[string]int{
		"monday": 1, "Monday": 1, "sunday": 0, "saturday": 6, "unknown": -1, "": -1,
	}
	for in, want := range cases {
		if got := ParseWeekday(in); got != want {
			t.Fatalf("ParseWeekday(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestTruncateString(t *testing.T) {
	if got := TruncateString("hello", 10); got != "hello" {
		t.Fatalf("short string changed: %q", got)
	}
	got := TruncateString("hello world", 8)
	if len(got) > 8+len("...") || got == "hello world" {
		t.Fatalf("truncate = %q", got)
	}
}

func TestFindNextFreeSlot(t *testing.T) {
	after := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC) // a Monday
	before := after.Add(14 * 24 * time.Hour)

	slot, ok := FindNextFreeSlot(nil, after, before, -1)
	if !ok || slot.Before(after) || slot.After(before) {
		t.Fatalf("free calendar: slot=%v ok=%v", slot, ok)
	}

	slot, ok = FindNextFreeSlot(nil, after, before, 3 /* Wednesday */)
	if !ok || slot.Weekday() != time.Wednesday {
		t.Fatalf("weekday slot = %v (%s), ok=%v", slot, slot.Weekday(), ok)
	}

	if _, ok := FindNextFreeSlot(nil, after, after.Add(time.Hour), 3); ok {
		t.Fatal("no Wednesday inside one hour window")
	}
}

func TestCampaignHandlers(t *testing.T) {
	f := newMCPFixture(t)
	principal, err := f.store.LookupAPIToken(context.Background(), f.apiToken(t, `["write","delete"]`))
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPrincipal(context.Background(), principal)

	_, created, err := f.handler.handleCreateCampaign(ctx, nil, CreateCampaignInput{
		TeamID:           f.team.ID,
		Name:             "Montagsfrage",
		RequiredHashtags: []string{"#montag"},
	})
	if err != nil {
		t.Fatalf("handleCreateCampaign: %v", err)
	}
	if created.CampaignID == "" || created.Name != "Montagsfrage" {
		t.Fatalf("create result: %+v", created)
	}

	_, got, err := f.handler.handleGetCampaign(ctx, nil, GetCampaignInput{
		TeamID:     f.team.ID,
		CampaignID: created.CampaignID,
	})
	if err != nil {
		t.Fatalf("handleGetCampaign: %v", err)
	}
	if got.Name != "Montagsfrage" || !got.IsActive {
		t.Fatalf("get result: %+v", got)
	}
}

func TestScheduleGetAndDeletePostHandlers(t *testing.T) {
	f := newMCPFixture(t)
	principal, err := f.store.LookupAPIToken(context.Background(), f.apiToken(t, `["write","delete"]`))
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPrincipal(context.Background(), principal)

	scheduledAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	_, scheduled, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Geplanter Titel",
		Content:        "Geplanter MCP-Post",
		ScheduledAt:    scheduledAt.Format(time.RFC3339),
		TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatalf("handleSchedulePost: %v", err)
	}
	if scheduled.PostID == "" || scheduled.Status != string(domain.PostStatusPending) {
		t.Fatalf("schedule result: %+v", scheduled)
	}

	if _, _, err := f.handler.handleSchedulePost(ctx, nil, SchedulePostInput{
		TeamID: f.team.ID, Content: "x", ScheduledAt: "not-a-date", TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("invalid scheduled_at must error")
	}

	_, posts, err := f.handler.handleGetPosts(ctx, nil, GetPostsInput{TeamID: f.team.ID})
	if err != nil {
		t.Fatalf("handleGetPosts: %v", err)
	}
	if posts.Total != 1 {
		t.Fatalf("posts total = %d, want 1", posts.Total)
	}

	_, deleted, err := f.handler.handleDeletePost(ctx, nil, DeletePostInput{TeamID: f.team.ID, PostID: scheduled.PostID})
	if err != nil {
		t.Fatalf("handleDeletePost: %v", err)
	}
	if !deleted.Success {
		t.Fatalf("delete result: %+v", deleted)
	}
	if _, err := f.store.GetScheduledPost(context.Background(), f.team.ID, scheduled.PostID); err == nil {
		t.Fatal("post must be gone after delete")
	}
}

func TestGetPlatformsHandler(t *testing.T) {
	f := newMCPFixture(t)
	principal, err := f.store.LookupAPIToken(context.Background(), f.apiToken(t, `["read"]`))
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPrincipal(context.Background(), principal)

	_, out, err := f.handler.handleGetPlatforms(ctx, nil, GetPlatformsInput{TeamID: f.team.ID})
	if err != nil {
		t.Fatalf("handleGetPlatforms: %v", err)
	}
	if len(out.Accounts) != 1 || out.Accounts[0].AccountID != f.account.ID {
		t.Fatalf("accounts: %+v", out.Accounts)
	}
}

func TestForbiddenForOutsider(t *testing.T) {
	f := newMCPFixture(t)
	outsider, err := f.store.UpsertOIDCUser(context.Background(), "outsider-"+uuid.NewString(), "out@test", "Out")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := f.store.CreateUserAPIToken(context.Background(), outsider.ID, "out", nil, `["write","delete"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := f.store.LookupAPIToken(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithPrincipal(context.Background(), principal)

	if _, _, err := f.handler.handleDraftPost(ctx, nil, DraftPostInput{
		TeamID: f.team.ID, Content: "fremd", TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("outsider must not draft into a foreign team")
	}
	if _, _, err := f.handler.handleGetCalendar(ctx, nil, GetCalendarInput{TeamID: f.team.ID}); err == nil {
		t.Fatal("outsider must not read a foreign calendar")
	}
}

func TestGetAnalyticsTimeslotsHandler(t *testing.T) {
	f := newMCPFixture(t)
	ctx := context.Background()
	principal, err := f.store.LookupAPIToken(ctx, f.apiToken(t, `["ai:read:context"]`))
	if err != nil {
		t.Fatal(err)
	}

	// Two posts in the same Monday 10:00 UTC slot, plus engagement metrics.
	mon := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	for i, likes := range []int64{8, 4} {
		post, err := f.store.CreateScheduledPost(ctx, f.team.ID, principal, domain.CreatePostInput{
			Content: "x", ScheduledAt: mon.Add(time.Duration(i) * time.Minute), TargetAccounts: []string{f.account.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
			t.Fatal(err)
		}
		if err := f.store.UpsertPostMetrics(ctx, post.ID, f.account.ID, map[string]int64{"likes": likes}, ""); err != nil {
			t.Fatal(err)
		}
	}

	toolCtx := WithPrincipal(ctx, principal)
	_, out, err := f.handler.handleGetAnalyticsTimeslots(toolCtx, nil, GetAnalyticsTimeslotsInput{TeamID: f.team.ID})
	if err != nil {
		t.Fatalf("handleGetAnalyticsTimeslots: %v", err)
	}
	if out.Timezone != "UTC" {
		t.Fatalf("timezone = %q, want UTC", out.Timezone)
	}
	if len(out.Timeslots) != 1 {
		t.Fatalf("got %d timeslots, want 1: %#v", len(out.Timeslots), out.Timeslots)
	}
	slot := out.Timeslots[0]
	if slot.Weekday != "Monday" || slot.Hour != 10 || slot.Posts != 2 || slot.TotalEngagement != 12 || slot.AvgEngagement != 6 {
		t.Fatalf("slot = %#v", slot)
	}

	// Timezone shifts the bucket: Monday 10:00 UTC is 12:00 in Berlin (CEST).
	_, berlin, err := f.handler.handleGetAnalyticsTimeslots(toolCtx, nil, GetAnalyticsTimeslotsInput{
		TeamID: f.team.ID, Timezone: "Europe/Berlin",
	})
	if err != nil {
		t.Fatalf("handleGetAnalyticsTimeslots berlin: %v", err)
	}
	if len(berlin.Timeslots) != 1 || berlin.Timeslots[0].Hour != 12 {
		t.Fatalf("berlin slot = %#v, want hour 12", berlin.Timeslots)
	}

	// An invalid timezone is a user error, not a silent fallback.
	if _, _, err := f.handler.handleGetAnalyticsTimeslots(toolCtx, nil, GetAnalyticsTimeslotsInput{
		TeamID: f.team.ID, Timezone: "Mars/Olympus",
	}); err == nil {
		t.Fatal("invalid timezone must error")
	}
}
