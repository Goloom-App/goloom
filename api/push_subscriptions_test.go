package api_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/push"
	"git.f4mily.net/goloom/internal/security"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func pushTestStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	return newMemorySQLite(t)
}

func pushTestAPI(t *testing.T, s *sqlitestore.Store, sender *push.Sender) *api.API {
	t.Helper()
	ctx := context.Background()
	authSvc, err := auth.New(ctx, config.Config{}, s)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	reg := provider.NewRegistry(
		provider.NewBlueskyProvider(),
		provider.NewFriendicaProvider(),
		provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}),
	)
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatalf("i18n.Load: %v", err)
	}
	a := api.New(logger, s, authSvc, reg, config.Config{}, nil, catalog, nil, nil)
	if sender != nil {
		a.SetPushSender(sender)
	}
	return a
}

func pushTestHandler(t *testing.T, s *sqlitestore.Store, sender *push.Sender) http.Handler {
	t.Helper()
	return pushTestAPI(t, s, sender).Handler(security.NewLimiter(10_000, 10_000), nil)
}

func seedPushUser(t *testing.T, s *sqlitestore.Store) (string, string) {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "push-usr-"+uuid.NewString(), "push-"+uuid.NewString()+"@example.test", "Push Tester")
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "integration", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return bearer, u.ID
}

func seedPushTeam(t *testing.T, s *sqlitestore.Store, userID string) string {
	t.Helper()
	ctx := context.Background()
	team, err := s.CreateTeam(ctx, userID, domain.CreateTeamInput{Name: "push-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	return team.ID
}

func validSubBody(t *testing.T, endpoint string) []byte {
	t.Helper()
	// Real P-256 key material: webpush does ECDH against p256dh, so a bogus
	// string would fail before any HTTP request leaves the process.
	secret := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		t.Fatal(err)
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	b, _ := json.Marshal(map[string]any{
		"endpoint": endpoint,
		"keys": map[string]string{
			"p256dh": enc.EncodeToString(elliptic.Marshal(elliptic.P256(), priv.X, priv.Y)),
			"auth":   enc.EncodeToString(secret),
		},
	})
	return b
}

func pushBearer(t *testing.T, h http.Handler, bearer string, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doPush(t *testing.T, h http.Handler, bearer string, path string) *httptest.ResponseRecorder {
	t.Helper()
	return pushBearer(t, h, bearer, "GET", path, nil)
}

// virtualPushEndpoint returns an https push endpoint whose host is purely
// virtual ("push.<seed>.test.invalid"). The API now accepts only https
// endpoints, so delivery tests pair these with a relayClient that rewrites
// the request to a local httptest server — the https check stays enforced
// while no traffic ever leaves the test process.
func virtualPushEndpoint(seed string) string {
	return "https://push." + seed + ".test.invalid/push"
}

// relayClient is a webpush.HTTPClient that forwards every delivery request to
// the given httptest server, regardless of the (virtual https) endpoint.
type relayClient struct {
	mu     sync.Mutex
	target *httptest.Server
	seen   []*http.Request
}

func (c *relayClient) Do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.seen = append(c.seen, req)
	c.mu.Unlock()
	clone := req.Clone(req.Context())
	targetURL, _ := url.Parse(c.target.URL)
	u := *req.URL
	u.Scheme = targetURL.Scheme
	u.Host = targetURL.Host
	clone.URL = &u
	return c.target.Client().Transport.RoundTrip(clone)
}

func newRelay(t *testing.T, srv *httptest.Server) *relayClient {
	t.Helper()
	return &relayClient{target: srv}
}

// usedByRelay reports whether the relay forwarded a delivery request
// carrying the given Subscription header token (set by webpush from the
// VAPID audience — i.e. the real endpoint the browser registered).
func (c *relayClient) sawAudience(audience string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.seen {
		if r.Header.Get("Authorization") != "" && audience != "" {
			return true
		}
	}
	return len(c.seen) > 0
}

func TestPushSubscriptionCRUD(t *testing.T) {
	s := pushTestStore(t)
	bearer, userID := seedPushUser(t, s)
	_ = seedPushTeam(t, s, userID)
	pushSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer pushSvc.Close()
	relay := newRelay(t, pushSvc)
	sender := push.New(s, push.Config{HTTPClient: relay, Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})), TTL: 3600})
	h := pushTestHandler(t, s, sender)

	// create
	rec := pushBearer(t, h, bearer, "POST", "/v1/me/push-subscriptions", validSubBody(t, virtualPushEndpoint("crud")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created struct {
		Item struct {
			ID      string `json:"id"`
			Enabled *bool  `json:"enabled"`
		} `json:"item"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Item.Enabled == nil || !*created.Item.Enabled {
		t.Fatalf("create: enabled should default true, got %+v", created.Item)
	}

	// vault public key available
	rec = doPush(t, h, bearer, "/v1/me/push/vapid-key")
	if rec.Code != http.StatusOK {
		t.Fatalf("vapid-key: got %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var vk map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &vk); err != nil {
		t.Fatal(err)
	}
	if vk["public_key"] == "" {
		t.Fatal("vapid-key: empty public_key")
	}

	// list shows one subscription
	rec = doPush(t, h, bearer, "/v1/me/push-subscriptions")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: got %d, want %d", rec.Code, http.StatusOK)
	}
	var listed struct {
		Items []domain.PushSubscription `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 {
		t.Fatalf("list: got %d items, want 1", len(listed.Items))
	}

	// disable device notifications
	rec = pushBearer(t, h, bearer, "PATCH", "/v1/me/push-subscriptions/"+created.Item.ID, []byte(`{"enabled":false}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// delete -> 204, list empty
	rec = pushBearer(t, h, bearer, "DELETE", "/v1/me/push-subscriptions/"+created.Item.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d, want %d", rec.Code, http.StatusNoContent)
	}
	rec = doPush(t, h, bearer, "/v1/me/push-subscriptions")
	var after struct {
		Items []domain.PushSubscription `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &after)
	if len(after.Items) != 0 {
		t.Fatalf("after delete: got %d items, want 0", len(after.Items))
	}
}

func TestPushSubscriptionValidation(t *testing.T) {
	s := pushTestStore(t)
	bearer, _ := seedPushUser(t, s)
	h := pushTestHandler(t, s, nil)

	cases := []struct {
		name string
		body string
	}{
		{"bad json", `{`},
		{"missing endpoint", `{"keys":{"p256dh":"x","auth":"y"}}`},
		{"bad scheme", `{"endpoint":"ftp://x.test/1","keys":{"p256dh":"x","auth":"y"}}`},
		{"http scheme rejected (SSRF guard)", `{"endpoint":"http://x.test/1","keys":{"p256dh":"x","auth":"y"}}`},
		{"missing keys", `{"endpoint":"https://x.test/1"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := pushBearer(t, h, bearer, "POST", "/v1/me/push-subscriptions", []byte(tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("got %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestPushTestDeliveryAndFailures(t *testing.T) {
	s := pushTestStore(t)
	bearer, _ := seedPushUser(t, s)
	pushSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer pushSvc.Close()
	failSvc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failSvc.Close()
	sender := push.New(s, push.Config{HTTPClient: newRelay(t, pushSvc), Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})), TTL: 3600})
	h := pushTestHandler(t, s, sender)

	// deliver test to a live endpoint
	rec := pushBearer(t, h, bearer, "POST", "/v1/me/push-subscriptions", validSubBody(t, virtualPushEndpoint("live")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: got %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var created struct {
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	rec = pushBearer(t, h, bearer, "POST", "/v1/me/push-subscriptions/"+created.Item.ID+"/test", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("test send: got %d, want %d: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	// missing subscription
	rec = pushBearer(t, h, bearer, "POST", "/v1/me/push-subscriptions/does-not-exist/test", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("test missing: got %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	// failing endpoint -> 502
	failing := push.New(s, push.Config{HTTPClient: newRelay(t, failSvc), Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{})), TTL: 3600})
	a := pushTestAPI(t, s, sender)
	a.SetPushSender(failing)
	h2 := a.Handler(security.NewLimiter(10_000, 10_000), nil)
	rec = pushBearer(t, h2, bearer, "POST", "/v1/me/push-subscriptions", validSubBody(t, virtualPushEndpoint("failing")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create fail endpoint: got %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var fail struct {
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &fail)
	rec = pushBearer(t, h2, bearer, "POST", "/v1/me/push-subscriptions/"+fail.Item.ID+"/test", nil)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("test failing endpoint: got %d, want %d: %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestTeamNotificationPrefs(t *testing.T) {
	s := pushTestStore(t)
	ctx := context.Background()
	bearer, userID := seedPushUser(t, s)
	team, err := s.CreateTeam(ctx, userID, domain.CreateTeamInput{Name: "prefs-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	h := pushTestHandler(t, s, nil)

	// default: enabled, unknown team
	rec := doPush(t, h, bearer, "/v1/me/team-notification-prefs")
	if rec.Code != http.StatusOK {
		t.Fatalf("list prefs: got %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var prefs struct {
		Items []domain.TeamNotificationPref `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &prefs); err != nil {
		t.Fatal(err)
	}
	if len(prefs.Items) != 1 || !prefs.Items[0].Enabled {
		t.Fatalf("prefs: got %+v, want one enabled item", prefs.Items)
	}
	if prefs.Items[0].TeamName != team.Name {
		t.Fatalf("prefs: team_name %q, want %q (UI must not render the raw team ID)", prefs.Items[0].TeamName, team.Name)
	}

	// disable
	rec = pushBearer(t, h, bearer, "PATCH", "/v1/me/team-notification-prefs/"+team.ID, []byte(`{"enabled":false}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch prefs: got %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var patched struct {
		Item domain.TeamNotificationPref `json:"item"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Item.TeamName != team.Name {
		t.Fatalf("patched prefs: team_name %q, want %q", patched.Item.TeamName, team.Name)
	}
	rec = doPush(t, h, bearer, "/v1/me/team-notification-prefs")
	_ = json.Unmarshal(rec.Body.Bytes(), &prefs)
	if len(prefs.Items) != 1 || prefs.Items[0].Enabled {
		t.Fatalf("prefs after disable: got %+v, want disabled", prefs.Items)
	}

	// non-member cannot change prefs
	bearer2, _ := seedPushUser(t, s)
	rec = pushBearer(t, h, bearer2, "PATCH", "/v1/me/team-notification-prefs/"+team.ID, []byte(`{"enabled":true}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("patch prefs other user: got %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestMyReviewCounts(t *testing.T) {
	s := pushTestStore(t)
	ctx := context.Background()
	bearer, userID := seedPushUser(t, s)
	u := domain.User{ID: userID}
	team, err := s.CreateTeam(ctx, userID, domain.CreateTeamInput{Name: "counts-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	// one open review draft
	_, err = s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: u}, domain.CreatePostInput{
		Title:       "needs review",
		Content:     "draft body",
		ScheduledAt: time.Now().UTC(),
		Draft:       true,
		Source:      domain.PostSourceAutomation,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := pushTestHandler(t, s, nil)

	rec := doPush(t, h, bearer, "/v1/me/review-counts")
	if rec.Code != http.StatusOK {
		t.Fatalf("counts: got %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var counts struct {
		Total   int              `json:"total"`
		ByTeam  map[string]int   `json:"by_team"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &counts); err != nil {
		t.Fatal(err)
	}
	if counts.Total != 1 {
		t.Fatalf("counts: got total %d, want 1", counts.Total)
	}
	if counts.ByTeam[team.ID] != 1 {
		t.Fatalf("counts: by_team[%s] = %d, want 1", team.ID, counts.ByTeam[team.ID])
	}
}

func TestMyReviewCounts_ExcludesViewerMemberships(t *testing.T) {
	s := pushTestStore(t)
	ctx := context.Background()
	bearer, userID := seedPushUser(t, s)
	team, err := s.CreateTeam(ctx, userID, domain.CreateTeamInput{Name: "counts-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	// One open review draft in the owned team: this is what the counts must keep.
	_, err = s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: domain.User{ID: userID}}, domain.CreatePostInput{
		Title:       "needs review",
		Content:     "draft body",
		ScheduledAt: time.Now().UTC(),
		Draft:       true,
		Source:      domain.PostSourceAutomation,
	})
	if err != nil {
		t.Fatal(err)
	}
	// A second team where the user is only a viewer but still has open items.
	_, memberID := seedPushUser(t, s)
	viewerTeam, err := s.CreateTeam(ctx, memberID, domain.CreateTeamInput{Name: "viewer-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateScheduledPost(ctx, viewerTeam.ID, domain.AuthenticatedPrincipal{User: domain.User{ID: memberID}}, domain.CreatePostInput{
		Title:       "open in viewer team",
		Content:     "draft body",
		ScheduledAt: time.Now().UTC(),
		Draft:       true,
		Source:      domain.PostSourceAutomation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddTeamMember(ctx, viewerTeam.ID, domain.AddTeamMemberInput{UserID: userID, Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	// Sanity: the viewer membership is real.
	isViewer, err := s.UserHasAnyTeamRole(ctx, userID, viewerTeam.ID, domain.RoleViewer)
	if err != nil || !isViewer {
		t.Fatalf("viewer membership missing: %v", err)
	}

	h := pushTestHandler(t, s, nil)
	rec := doPush(t, h, bearer, "/v1/me/review-counts")
	if rec.Code != http.StatusOK {
		t.Fatalf("counts: got %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var counts struct {
		Total  int            `json:"total"`
		ByTeam map[string]int `json:"by_team"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &counts); err != nil {
		t.Fatal(err)
	}
	if _, ok := counts.ByTeam[viewerTeam.ID]; ok {
		t.Fatalf("counts must exclude viewer-only membership: %#v", counts.ByTeam)
	}

	rec = pushBearer(t, h, bearer, "PATCH", "/v1/me/team-notification-prefs/"+viewerTeam.ID, []byte(`{"enabled":true}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("viewer must not configure prefs for %s: got %d, want %d", viewerTeam.ID, rec.Code, http.StatusForbidden)
	}

	if counts.Total != 1 || counts.ByTeam[team.ID] != 1 {
		t.Fatalf("counts: total %d by_team %#v, want owner team only with 1", counts.Total, counts.ByTeam)
	}
}