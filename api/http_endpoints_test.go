package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

// endpointFixture boots the full API mux against an in-memory store with one
// admin owner (first user), an AI-enabled team, and one connected account.
type endpointFixture struct {
	handler http.Handler
	store   *sqlitestore.Store
	bearer  string
	team    domain.Team
	account domain.SocialAccount
	user    domain.User
}

func newEndpointFixture(t *testing.T) endpointFixture {
	t.Helper()
	ctx := context.Background()
	s := newMemorySQLite(t)

	u, err := s.UpsertOIDCUser(ctx, "ep-"+uuid.NewString(), "ep@example.test", "Endpoint")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ep-" + uuid.NewString(), Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	aiEnabled := true
	if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &aiEnabled}); err != nil {
		t.Fatal(err)
	}
	team.IsAIEnabled = true
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m.example", Username: "ep", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "endpoints", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return endpointFixture{
		handler: analyticsTestHandler(t, s),
		store:   s,
		bearer:  bearer,
		team:    team,
		account: acc,
		user:    u,
	}
}

func (f endpointFixture) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+f.bearer)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	return rec
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return out
}

func requireStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, want, rec.Body.String())
	}
}

func TestPublicEndpoints(t *testing.T) {
	f := newEndpointFixture(t)

	requireStatus(t, f.do(t, http.MethodGet, "/healthz", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, "/v1/providers", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, "/v1/auth/status", nil), http.StatusOK)
}

func TestMeAndAPITokens(t *testing.T) {
	f := newEndpointFixture(t)

	rec := f.do(t, http.MethodGet, "/v1/me", nil)
	requireStatus(t, rec, http.StatusOK)

	rec = f.do(t, http.MethodPost, "/v1/me/api-tokens", map[string]any{"name": "extra-token"})
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("create token status = %d (body: %s)", rec.Code, rec.Body.String())
	}

	rec = f.do(t, http.MethodGet, "/v1/me/api-tokens", nil)
	requireStatus(t, rec, http.StatusOK)
	list := decodeJSON[map[string]json.RawMessage](t, rec)
	if _, ok := list["items"]; !ok {
		t.Fatalf("token list missing items: %s", rec.Body.String())
	}
}

func TestTeamLifecycleEndpoints(t *testing.T) {
	f := newEndpointFixture(t)

	rec := f.do(t, http.MethodPost, "/v1/teams", map[string]any{"name": "Zweites Team", "description": "x"})
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("create team status = %d (body: %s)", rec.Code, rec.Body.String())
	}

	rec = f.do(t, http.MethodGet, "/v1/teams", nil)
	requireStatus(t, rec, http.StatusOK)

	rec = f.do(t, http.MethodPatch, "/v1/teams/"+f.team.ID, map[string]any{
		"name": "Umbenannt", "brand_color": "#336699",
	})
	requireStatus(t, rec, http.StatusOK)
	team := decodeJSON[domain.Team](t, rec)
	if team.Name != "Umbenannt" || team.BrandColor != "#336699" {
		t.Fatalf("patched team: %+v", team)
	}

	rec = f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/members", nil)
	requireStatus(t, rec, http.StatusOK)
}

func TestTeamProfileEndpoints(t *testing.T) {
	f := newEndpointFixture(t)
	base := "/v1/teams/" + f.team.ID + "/profile"

	requireStatus(t, f.do(t, http.MethodGet, base, nil), http.StatusNotFound)

	rec := f.do(t, http.MethodPut, base, map[string]any{
		"style_metadata": map[string]any{"tonality": "locker", "max_hashtags": 3},
	})
	requireStatus(t, rec, http.StatusOK)

	rec = f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)
	profile := decodeJSON[domain.TeamProfile](t, rec)
	if profile.StyleMetadata.Tonality != "locker" {
		t.Fatalf("profile: %+v", profile)
	}

	// Update path (profile exists now).
	rec = f.do(t, http.MethodPut, base, map[string]any{
		"style_metadata": map[string]any{"tonality": "seriös"},
	})
	requireStatus(t, rec, http.StatusOK)

	requireStatus(t, f.do(t, http.MethodDelete, base, nil), http.StatusNoContent)
	requireStatus(t, f.do(t, http.MethodGet, base, nil), http.StatusNotFound)
}

func TestStyleExampleEndpoints(t *testing.T) {
	f := newEndpointFixture(t)
	base := "/v1/teams/" + f.team.ID + "/style-examples"

	rec := f.do(t, http.MethodPost, base, map[string]any{"content": "So klingen wir."})
	requireStatus(t, rec, http.StatusCreated)
	created := decodeJSON[domain.StyleExample](t, rec)
	if created.ID == "" {
		t.Fatalf("style example: %+v", created)
	}

	rec = f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)

	requireStatus(t, f.do(t, http.MethodDelete, base+"/"+created.ID, nil), http.StatusNoContent)
}

func TestRSSFeedEndpoints(t *testing.T) {
	f := newEndpointFixture(t)
	base := "/v1/teams/" + f.team.ID + "/rss-feeds"

	rec := f.do(t, http.MethodPost, base, map[string]any{
		"feed_url": "https://blog.example/feed.xml",
		"name":     "Blog",
	})
	requireStatus(t, rec, http.StatusCreated)
	feed := decodeJSON[domain.RSSFeedConfig](t, rec)
	if feed.ID == "" || feed.Name != "Blog" {
		t.Fatalf("feed: %+v", feed)
	}

	rec = f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)

	rec = f.do(t, http.MethodPatch, base+"/"+feed.ID, map[string]any{"name": "Blog 2"})
	requireStatus(t, rec, http.StatusOK)
	updated := decodeJSON[domain.RSSFeedConfig](t, rec)
	if updated.Name != "Blog 2" {
		t.Fatalf("updated feed: %+v", updated)
	}

	requireStatus(t, f.do(t, http.MethodDelete, base+"/"+feed.ID, nil), http.StatusNoContent)
}

func TestAIServiceConfigEndpoints(t *testing.T) {
	f := newEndpointFixture(t)
	base := "/v1/teams/" + f.team.ID + "/ai-service-config"

	rec := f.do(t, http.MethodPut, base, map[string]any{
		"provider": "openai", "model": "gpt-test", "api_key": "sk-secret",
	})
	requireStatus(t, rec, http.StatusOK)

	rec = f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)
	cfg := decodeJSON[map[string]any](t, rec)
	if cfg["provider"] != "openai" {
		t.Fatalf("config: %v", cfg)
	}
	if raw, ok := cfg["api_key"]; ok && raw != "" {
		t.Fatalf("api key must never be returned, got %v", raw)
	}
	if cfg["api_key_set"] != true {
		t.Fatalf("api_key_set missing: %v", cfg)
	}
}

func TestAnalyticsEndpoints(t *testing.T) {
	f := newEndpointFixture(t)
	prefix := "/v1/teams/" + f.team.ID

	requireStatus(t, f.do(t, http.MethodGet, prefix+"/analytics/hashtags", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, prefix+"/analytics/engagement-heatmap", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, prefix+"/analytics/engagement-hours", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, prefix+"/analytics/account/"+f.account.ID+"/growth", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, prefix+"/versions", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, prefix+"/review-queue", nil), http.StatusOK)
}

func TestPostVersionEndpoints(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	principal := domain.AuthenticatedPrincipal{User: f.user}
	post, err := f.store.CreateScheduledPost(ctx, f.team.ID, principal, domain.CreatePostInput{
		Content: "haupttext", ScheduledAt: time.Now().UTC().Add(time.Hour),
		TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := "/v1/teams/" + f.team.ID + "/posts/" + post.ID + "/versions"

	requireStatus(t, f.do(t, http.MethodGet, base, nil), http.StatusOK)

	rec := f.do(t, http.MethodPatch, base, map[string]any{
		"versions": []map[string]string{{"account_id": f.account.ID, "content": "kurzfassung"}},
	})
	requireStatus(t, rec, http.StatusOK)

	rec = f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)
	versions := decodeJSON[map[string][]domain.PostVersion](t, rec)
	if len(versions["items"]) != 1 || versions["items"][0].Content != "kurzfassung" {
		t.Fatalf("versions: %+v", versions)
	}
}

func TestExternalPostMonitorEndpoints(t *testing.T) {
	f := newEndpointFixture(t)
	base := "/v1/teams/" + f.team.ID + "/external-post-monitor"

	requireStatus(t, f.do(t, http.MethodGet, base, nil), http.StatusOK)

	rec := f.do(t, http.MethodPut, base, map[string]any{"enabled": true, "interval_minutes": 30})
	requireStatus(t, rec, http.StatusOK)

	rec = f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)
	monitor := decodeJSON[map[string]any](t, rec)
	if monitor["enabled"] != true {
		t.Fatalf("monitor: %v", monitor)
	}
}

func TestAdminEndpoints(t *testing.T) {
	f := newEndpointFixture(t)
	if !f.user.IsAdmin {
		t.Fatal("fixture user must be the first (admin) user")
	}

	requireStatus(t, f.do(t, http.MethodGet, "/v1/admin/metrics", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, "/v1/admin/users", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, "/v1/admin/runtime-config", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, "/v1/admin/sync-status", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, "/v1/admin/ai-enabled-teams", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodGet, "/v1/admin/logs", nil), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodPost, "/v1/admin/logs/prune", map[string]any{"older_than_days": 30}), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodPost, "/v1/admin/repair-future-posted", nil), http.StatusOK)
}

func TestAdminEndpointsForbiddenForNonAdmin(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	other, err := f.store.UpsertOIDCUser(ctx, "nonadmin-"+uuid.NewString(), "na@example.test", "NA")
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := f.store.CreateUserAPIToken(ctx, other.ID, "na", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	requireStatus(t, rec, http.StatusForbidden)
}

func TestPostScopeAndTeamBindingEnforcement(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	post := func(token string) int {
		body, _ := json.Marshal(map[string]any{
			"title":           "scoped post",
			"content":         "scoped post",
			"scheduled_at":    time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339),
			"target_accounts": []string{f.account.ID},
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+f.team.ID+"/posts", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// read-only token may not schedule a post.
	readOnly, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "read", nil, `["read"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if code := post(readOnly); code != http.StatusForbidden {
		t.Fatalf("read-only schedule: got %d, want 403", code)
	}

	// write:schedule token may schedule.
	scheduler, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "sched", nil, `["write:schedule"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if code := post(scheduler); code != http.StatusCreated {
		t.Fatalf("write:schedule schedule: got %d, want 201", code)
	}

	// A token bound to a different team is rejected even though the user owns f.team.
	otherTeam, err := f.store.CreateTeam(ctx, f.user.ID, domain.CreateTeamInput{Name: "other-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	bound, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "bound", nil, "", &otherTeam.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if code := post(bound); code != http.StatusForbidden {
		t.Fatalf("team-bound token on foreign team: got %d, want 403", code)
	}
}

func TestScopeGatesOnHTTPRoutes(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	get := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+f.team.ID+"/ai-context", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		return rec.Code
	}

	// Unscoped tokens keep full access (backward compatible).
	unscoped, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "unscoped", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if code := get(unscoped); code != http.StatusOK {
		t.Fatalf("unscoped: got %d, want 200", code)
	}

	// A read-scoped token may read AI context.
	reader, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "reader", nil, `["read"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if code := get(reader); code != http.StatusOK {
		t.Fatalf("reader: got %d, want 200", code)
	}

	// A write-only token cannot read.
	writer, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "writer", nil, `["write:draft"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if code := get(writer); code != http.StatusForbidden {
		t.Fatalf("writer: got %d, want 403", code)
	}
}

func TestTeamMediaRename(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	created, err := f.store.CreateMediaItem(ctx, domain.MediaItem{
		TeamID: f.team.ID, Sha256: "ren-" + uuid.NewString(), Filename: "old.png", MimeType: "image/png", SizeBytes: 12,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := f.do(t, http.MethodPatch, "/v1/teams/"+f.team.ID+"/media/"+created.ID, map[string]any{"filename": "renamed.png"})
	requireStatus(t, rec, http.StatusOK)
	item := decodeJSON[domain.MediaItem](t, rec)
	if item.Filename != "renamed.png" {
		t.Fatalf("filename = %q, want renamed.png", item.Filename)
	}

	// Empty filename is rejected.
	rec = f.do(t, http.MethodPatch, "/v1/teams/"+f.team.ID+"/media/"+created.ID, map[string]any{"filename": "  "})
	requireStatus(t, rec, http.StatusBadRequest)

	// Unknown media id is a 404.
	rec = f.do(t, http.MethodPatch, "/v1/teams/"+f.team.ID+"/media/"+uuid.NewString(), map[string]any{"filename": "x.png"})
	requireStatus(t, rec, http.StatusNotFound)
}

func TestSessionCookieLoginAndLogout(t *testing.T) {
	f := newEndpointFixture(t)

	// Exchange a valid bearer token for a cookie session.
	body, _ := json.Marshal(map[string]string{"token": f.bearer})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/session/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	requireStatus(t, rec, http.StatusOK)

	var session, csrf *http.Cookie
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case "goloom_session":
			session = c
		case "goloom_csrf":
			csrf = c
		}
	}
	if session == nil || session.Value == "" || !session.HttpOnly {
		t.Fatalf("expected HttpOnly session cookie, got %#v", session)
	}
	if csrf == nil || csrf.Value == "" || csrf.HttpOnly {
		t.Fatalf("expected readable csrf cookie, got %#v", csrf)
	}

	// The cookie authenticates GET /v1/me (no Authorization header).
	meReq := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	meReq.AddCookie(session)
	meRec := httptest.NewRecorder()
	f.handler.ServeHTTP(meRec, meReq)
	requireStatus(t, meRec, http.StatusOK)

	// Logout (cookie POST) needs the CSRF token; then it revokes the session.
	logoutReq := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	logoutReq.AddCookie(session)
	logoutReq.AddCookie(csrf)
	logoutReq.Header.Set("X-CSRF-Token", csrf.Value)
	logoutRec := httptest.NewRecorder()
	f.handler.ServeHTTP(logoutRec, logoutReq)
	requireStatus(t, logoutRec, http.StatusNoContent)

	// The revoked session no longer authenticates.
	goneReq := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	goneReq.AddCookie(session)
	goneRec := httptest.NewRecorder()
	f.handler.ServeHTTP(goneRec, goneReq)
	requireStatus(t, goneRec, http.StatusUnauthorized)
}
