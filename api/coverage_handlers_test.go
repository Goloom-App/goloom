package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

// newPrivateLANHandler creates an API handler with AllowPrivateProviderInstanceURLs
// so tests can use 127.0.0.1 / loopback URLs without DNS SSRF guards blocking them.
func newPrivateLANHandler(t *testing.T, s *sqlitestore.Store) http.Handler {
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
	cfg := config.Config{AllowPrivateProviderInstanceURLs: true}
	h := api.New(logger, s, authSvc, reg, cfg, nil, catalog, nil, nil)
	return h.Handler(security.NewLimiter(10_000, 10_000), nil)
}

// seedProviderInstance creates a provider instance directly in the store and
// returns it; bypasses HTTP so no DNS/SSRF check runs.
func seedProviderInstance(t *testing.T, s *sqlitestore.Store, userID string) domain.ProviderInstance {
	t.Helper()
	ctx := context.Background()
	inst, err := s.CreateProviderInstance(ctx, userID, domain.PreparedProviderInstance{
		Provider:              "mastodon",
		Name:                  "Test Instance",
		InstanceURL:           "https://mastodon.example.test",
		ClientID:              "client-id-123",
		ClientSecret:          "client-secret-456",
		Scopes:                []string{"read", "write"},
		AuthorizationEndpoint: "https://mastodon.example.test/oauth/authorize",
		TokenEndpoint:         "https://mastodon.example.test/oauth/token",
	})
	if err != nil {
		t.Fatalf("seedProviderInstance: %v", err)
	}
	return inst
}

// seedScheduledPost creates a future-scheduled post directly in the store.
func seedScheduledPost(t *testing.T, s *sqlitestore.Store, teamID, accountID string, user domain.User) domain.ScheduledPost {
	t.Helper()
	ctx := context.Background()
	principal := domain.AuthenticatedPrincipal{User: user}
	post, err := s.CreateScheduledPost(ctx, teamID, principal, domain.CreatePostInput{
		Title:          "coverage-test-post",
		Content:        "coverage test post content",
		ScheduledAt:    time.Now().UTC().Add(24 * time.Hour),
		TargetAccounts: []string{accountID},
	})
	if err != nil {
		t.Fatalf("seedScheduledPost: %v", err)
	}
	return post
}

// seedLogEntry inserts a log entry directly into the store and returns it.
func seedLogEntry(t *testing.T, s *sqlitestore.Store) domain.LogEntry {
	t.Helper()
	ctx := context.Background()
	e := domain.LogEntry{
		ID:        uuid.NewString(),
		Level:     "info",
		Message:   "coverage test log entry",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.InsertLogEntry(ctx, e); err != nil {
		t.Fatalf("seedLogEntry: %v", err)
	}
	return e
}

// doWithBearer sends a request with the given bearer token and returns the recorder.
func doWithBearer(t *testing.T, h http.Handler, method, path, bearer string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Authorization", "Bearer "+bearer)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ── Accounts ──────────────────────────────────────────────────────────────────

func TestListAccounts(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/accounts", nil)
	requireStatus(t, rec, http.StatusOK)
	list := decodeJSON[map[string][]domain.SocialAccount](t, rec)
	if len(list["items"]) < 1 {
		t.Fatalf("expected at least one account (the fixture account), got %v", list)
	}
}

func TestCreateAccount_MissingProvider(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/accounts", map[string]any{
		"instance_url": "https://mastodon.social",
		"access_token": "tok",
	})
	requireStatus(t, rec, http.StatusBadRequest)
}

func TestCreateAccount_UnsupportedProvider(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/accounts", map[string]any{
		"provider":     "unknown-provider-xyz",
		"access_token": "tok",
	})
	requireStatus(t, rec, http.StatusBadRequest)
}

func TestCreateAccount_InvalidJSON(t *testing.T) {
	f := newEndpointFixture(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+f.team.ID+"/accounts",
		bytes.NewReader([]byte("{not valid json")))
	req.Header.Set("Authorization", "Bearer "+f.bearer)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	requireStatus(t, rec, http.StatusBadRequest)
}

func TestCreateAccount_Friendica(t *testing.T) {
	// Friendica/GenericStatusProvider.ConnectAccount does not make HTTP calls.
	// AllowPrivateProviderInstanceURLs bypasses the SSRF URL validation.
	s := newMemorySQLite(t)
	h := newPrivateLANHandler(t, s)
	ctx := context.Background()

	u, err := s.UpsertOIDCUser(ctx, "ca-"+uuid.NewString(), "ca@example.test", "CA")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ca-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "ca", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+team.ID+"/accounts", bearer, map[string]any{
		"provider":     "friendica",
		"instance_url": "http://127.0.0.1/",
		"username":     "friuser",
		"access_token": "tok",
	})
	requireStatus(t, rec, http.StatusCreated)
	acc := decodeJSON[domain.SocialAccount](t, rec)
	if acc.ID == "" {
		t.Fatal("expected non-empty ID in created account")
	}
	if acc.Provider != "friendica" {
		t.Errorf("provider = %q, want friendica", acc.Provider)
	}
}

func TestCreateAccount_MastodonViaTestServer(t *testing.T) {
	// Start a local server simulating Mastodon's verify_credentials endpoint.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/accounts/verify_credentials" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"12345","username":"mastotest","acct":"mastotest"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	s := newMemorySQLite(t)
	h := newPrivateLANHandler(t, s)
	ctx := context.Background()

	u, err := s.UpsertOIDCUser(ctx, "mca-"+uuid.NewString(), "mca@example.test", "MCA")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "mca-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "mca", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+team.ID+"/accounts", bearer, map[string]any{
		"provider":     "mastodon",
		"instance_url": server.URL,
		"access_token": "test-token",
	})
	requireStatus(t, rec, http.StatusCreated)
	acc := decodeJSON[domain.SocialAccount](t, rec)
	if acc.Username != "mastotest" {
		t.Errorf("username = %q, want mastotest", acc.Username)
	}
}

func TestUpdateAccount(t *testing.T) {
	f := newEndpointFixture(t)
	newName := "Updated Account Name"
	rec := f.do(t, http.MethodPatch, "/v1/teams/"+f.team.ID+"/accounts/"+f.account.ID,
		map[string]any{"name": newName})
	requireStatus(t, rec, http.StatusOK)
	acc := decodeJSON[domain.SocialAccount](t, rec)
	if acc.Name != newName {
		t.Errorf("name = %q, want %q", acc.Name, newName)
	}
}

func TestUpdateAccount_NotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPatch, "/v1/teams/"+f.team.ID+"/accounts/"+uuid.NewString(),
		map[string]any{"name": "x"})
	requireStatus(t, rec, http.StatusNotFound)
}

func TestUpdateAccount_WrongTeam(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	// Create a second team and try to address f.team's account via the new team path.
	otherTeam, err := f.store.CreateTeam(ctx, f.user.ID, domain.CreateTeamInput{Name: "other-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	rec := f.do(t, http.MethodPatch, "/v1/teams/"+otherTeam.ID+"/accounts/"+f.account.ID,
		map[string]any{"name": "x"})
	requireStatus(t, rec, http.StatusNotFound)
}

func TestDeleteAccount(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	acc, err := f.store.CreateAccount(ctx, f.team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m2.example", Username: "del-me", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := f.do(t, http.MethodDelete, "/v1/teams/"+f.team.ID+"/accounts/"+acc.ID, nil)
	requireStatus(t, rec, http.StatusNoContent)
}

func TestDeleteAccount_NotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodDelete, "/v1/teams/"+f.team.ID+"/accounts/"+uuid.NewString(), nil)
	requireStatus(t, rec, http.StatusNotFound)
}

// ── Team Members ──────────────────────────────────────────────────────────────

func TestAddTeamMember(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	newUser, err := f.store.UpsertOIDCUser(ctx, "mem-"+uuid.NewString(), "mem@example.test", "Member")
	if err != nil {
		t.Fatal(err)
	}

	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/members", map[string]any{
		"user_id": newUser.ID,
		"role":    "editor",
	})
	requireStatus(t, rec, http.StatusCreated)
}

func TestAddTeamMember_MissingUserID(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/members", map[string]any{
		"role": "editor",
	})
	requireStatus(t, rec, http.StatusBadRequest)
}

func TestAddTeamMember_InvalidRole(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	other, _ := f.store.UpsertOIDCUser(ctx, "mr-"+uuid.NewString(), "mr@example.test", "MR")
	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/members", map[string]any{
		"user_id": other.ID,
		"role":    "superuser", // invalid role
	})
	requireStatus(t, rec, http.StatusBadRequest)
}

func TestRemoveTeamMember(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	newUser, err := f.store.UpsertOIDCUser(ctx, "rm-"+uuid.NewString(), "rm@example.test", "RM")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AddTeamMember(ctx, f.team.ID, domain.AddTeamMemberInput{
		UserID: newUser.ID, Role: domain.RoleEditor,
	}); err != nil {
		t.Fatal(err)
	}

	rec := f.do(t, http.MethodDelete, "/v1/teams/"+f.team.ID+"/members/"+newUser.ID, nil)
	requireStatus(t, rec, http.StatusNoContent)
}

// ── Provider Instances ────────────────────────────────────────────────────────

func TestListProviderInstances(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodGet, "/v1/provider-instances", nil)
	requireStatus(t, rec, http.StatusOK)
	list := decodeJSON[map[string]any](t, rec)
	if _, ok := list["items"]; !ok {
		t.Fatalf("missing items key: %v", list)
	}
}

func TestListProviderInstances_WithSeededData(t *testing.T) {
	f := newEndpointFixture(t)
	seedProviderInstance(t, f.store, f.user.ID)
	rec := f.do(t, http.MethodGet, "/v1/provider-instances?provider=mastodon", nil)
	requireStatus(t, rec, http.StatusOK)
}

func TestListProviderInstances_UnsupportedProviderFilter(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodGet, "/v1/provider-instances?provider=totally-unknown", nil)
	requireStatus(t, rec, http.StatusBadRequest)
}

func TestGetProviderInstance(t *testing.T) {
	f := newEndpointFixture(t)
	inst := seedProviderInstance(t, f.store, f.user.ID)

	rec := f.do(t, http.MethodGet, "/v1/provider-instances/"+inst.ID, nil)
	requireStatus(t, rec, http.StatusOK)
	got := decodeJSON[domain.ProviderInstance](t, rec)
	if got.ID != inst.ID {
		t.Errorf("ID = %q, want %q", got.ID, inst.ID)
	}
	if got.Provider != "mastodon" {
		t.Errorf("provider = %q, want mastodon", got.Provider)
	}
}

func TestGetProviderInstance_NotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodGet, "/v1/provider-instances/"+uuid.NewString(), nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestCreateProviderInstance_MissingProvider(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/admin/provider-instances", map[string]any{
		"name":         "test",
		"instance_url": "https://mastodon.social",
	})
	requireStatus(t, rec, http.StatusBadRequest)
}

func TestCreateProviderInstance_UnsupportedProvider(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/admin/provider-instances", map[string]any{
		"provider":     "totally-unknown-xyz",
		"name":         "test",
		"instance_url": "https://example.test",
	})
	requireStatus(t, rec, http.StatusBadRequest)
}

func TestCreateProviderInstance_HappyPath(t *testing.T) {
	// AllowPrivateProviderInstanceURLs + fully-specified OAuth fields avoids
	// any outbound mastodon app registration / discovery calls.
	s := newMemorySQLite(t)
	h := newPrivateLANHandler(t, s)
	ctx := context.Background()

	u, err := s.UpsertOIDCUser(ctx, "cpi-"+uuid.NewString(), "cpi@example.test", "CPI")
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "cpi", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, h, http.MethodPost, "/v1/admin/provider-instances", bearer, map[string]any{
		"provider":               "mastodon",
		"name":                   "My Mastodon",
		"instance_url":           "http://127.0.0.1/",
		"client_id":              "cid-123",
		"client_secret":          "csec-456",
		"scopes":                 []string{"read", "write"},
		"authorization_endpoint": "http://127.0.0.1/oauth/authorize",
		"token_endpoint":         "http://127.0.0.1/oauth/token",
	})
	requireStatus(t, rec, http.StatusCreated)
	created := decodeJSON[domain.ProviderInstance](t, rec)
	if created.ID == "" {
		t.Fatal("expected non-empty ID in created provider instance")
	}
}

func TestUpdateProviderInstance_HappyPath(t *testing.T) {
	s := newMemorySQLite(t)
	h := newPrivateLANHandler(t, s)
	ctx := context.Background()

	u, err := s.UpsertOIDCUser(ctx, "upi-"+uuid.NewString(), "upi@example.test", "UPI")
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "upi", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := s.CreateProviderInstance(ctx, u.ID, domain.PreparedProviderInstance{
		Provider:              "mastodon",
		Name:                  "Before",
		InstanceURL:           "http://127.0.0.1/",
		ClientID:              "cid",
		ClientSecret:          "csec",
		Scopes:                []string{"read"},
		AuthorizationEndpoint: "http://127.0.0.1/oauth/authorize",
		TokenEndpoint:         "http://127.0.0.1/oauth/token",
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := doRequest(t, h, http.MethodPut, "/v1/admin/provider-instances/"+inst.ID, bearer, map[string]any{
		"name": "After",
		// All other fields omitted → carried over from existing record.
	})
	requireStatus(t, rec, http.StatusOK)
	updated := decodeJSON[domain.ProviderInstance](t, rec)
	if updated.Name != "After" {
		t.Errorf("name = %q, want After", updated.Name)
	}
}

func TestUpdateProviderInstance_NotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPut, "/v1/admin/provider-instances/"+uuid.NewString(),
		map[string]any{"name": "x"})
	requireStatus(t, rec, http.StatusNotFound)
}

func TestDeleteProviderInstance(t *testing.T) {
	f := newEndpointFixture(t)
	inst := seedProviderInstance(t, f.store, f.user.ID)
	rec := f.do(t, http.MethodDelete, "/v1/admin/provider-instances/"+inst.ID, nil)
	requireStatus(t, rec, http.StatusNoContent)
}

func TestDeleteProviderInstance_NotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodDelete, "/v1/admin/provider-instances/"+uuid.NewString(), nil)
	requireStatus(t, rec, http.StatusNotFound)
}

// ── Posts ─────────────────────────────────────────────────────────────────────

func TestListPosts(t *testing.T) {
	f := newEndpointFixture(t)
	seedScheduledPost(t, f.store, f.team.ID, f.account.ID, f.user)

	rec := f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/posts", nil)
	requireStatus(t, rec, http.StatusOK)
	list := decodeJSON[map[string][]domain.ScheduledPost](t, rec)
	if len(list["items"]) < 1 {
		t.Fatal("expected at least one post in list")
	}
}

func TestGetPost(t *testing.T) {
	f := newEndpointFixture(t)
	post := seedScheduledPost(t, f.store, f.team.ID, f.account.ID, f.user)

	rec := f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/posts/"+post.ID, nil)
	requireStatus(t, rec, http.StatusOK)
	got := decodeJSON[domain.ScheduledPost](t, rec)
	if got.ID != post.ID {
		t.Errorf("ID = %q, want %q", got.ID, post.ID)
	}
	// PublishedLinks field is populated via attachPublishedLinks, even if empty.
}

func TestGetPost_NotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/posts/"+uuid.NewString(), nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestGetPost_ForbiddenForOtherTeam(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	// victim creates a team and a post.
	victim, _ := f.store.UpsertOIDCUser(ctx, "gpv-"+uuid.NewString(), "gpv@example.test", "GPV")
	victimTeam, _ := f.store.CreateTeam(ctx, victim.ID, domain.CreateTeamInput{Name: "gpv-" + uuid.NewString()})
	victimAcc, _ := f.store.CreateAccount(ctx, victimTeam.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://victim.example", Username: "victim", AccessToken: "tok",
	})
	victimPost := seedScheduledPost(t, f.store, victimTeam.ID, victimAcc.ID, victim)

	// attacker is a non-admin user without membership in victimTeam.
	attacker, _ := f.store.UpsertOIDCUser(ctx, "gpa-"+uuid.NewString(), "gpa@example.test", "GPA")
	attackerBearer, _, _ := f.store.CreateUserAPIToken(ctx, attacker.ID, "atk", nil, "", nil, "")

	// attacker tries to read victim's post → 403.
	rec := doWithBearer(t, f.handler, http.MethodGet,
		"/v1/teams/"+victimTeam.ID+"/posts/"+victimPost.ID, attackerBearer, nil)
	requireStatus(t, rec, http.StatusForbidden)
}

func TestDeletePost_HappyPath(t *testing.T) {
	f := newEndpointFixture(t)
	post := seedScheduledPost(t, f.store, f.team.ID, f.account.ID, f.user)
	rec := f.do(t, http.MethodDelete, "/v1/teams/"+f.team.ID+"/posts/"+post.ID, nil)
	requireStatus(t, rec, http.StatusNoContent)
}

func TestDeletePost_NotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodDelete, "/v1/teams/"+f.team.ID+"/posts/"+uuid.NewString(), nil)
	requireStatus(t, rec, http.StatusNotFound)
}

// postDeleteScope maps status to the required token scope: draft→delete:draft, else→delete:schedule.
func TestDeletePost_ScopeEnforcement(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	post := seedScheduledPost(t, f.store, f.team.ID, f.account.ID, f.user)

	// A token with only delete:draft cannot delete a scheduled post.
	draftOnly, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "draftonly", nil, `["delete:draft"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	rec := doWithBearer(t, f.handler, http.MethodDelete, "/v1/teams/"+f.team.ID+"/posts/"+post.ID, draftOnly, nil)
	requireStatus(t, rec, http.StatusForbidden)

	// A token with delete:schedule can delete the same post.
	scheduleDelete, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "scheddelete", nil, `["delete:schedule"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	rec = doWithBearer(t, f.handler, http.MethodDelete, "/v1/teams/"+f.team.ID+"/posts/"+post.ID, scheduleDelete, nil)
	requireStatus(t, rec, http.StatusNoContent)
}

func TestDeletePost_ForbiddenForOtherTeam(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	// victim creates a team and a post.
	victim, _ := f.store.UpsertOIDCUser(ctx, "dpv-"+uuid.NewString(), "dpv@example.test", "DPV")
	victimTeam, _ := f.store.CreateTeam(ctx, victim.ID, domain.CreateTeamInput{Name: "dpv-" + uuid.NewString()})
	victimAcc, _ := f.store.CreateAccount(ctx, victimTeam.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://dpv.example", Username: "dpv", AccessToken: "tok",
	})
	victimPost := seedScheduledPost(t, f.store, victimTeam.ID, victimAcc.ID, victim)

	// attacker is a non-admin user without membership in victimTeam.
	attacker, _ := f.store.UpsertOIDCUser(ctx, "dpa-"+uuid.NewString(), "dpa@example.test", "DPA")
	attackerBearer, _, _ := f.store.CreateUserAPIToken(ctx, attacker.ID, "dpatk", nil, "", nil, "")

	// attacker tries to delete victim's post → 403.
	rec := doWithBearer(t, f.handler, http.MethodDelete,
		"/v1/teams/"+victimTeam.ID+"/posts/"+victimPost.ID, attackerBearer, nil)
	requireStatus(t, rec, http.StatusForbidden)
}

func TestCancelPost_HappyPath(t *testing.T) {
	f := newEndpointFixture(t)
	post := seedScheduledPost(t, f.store, f.team.ID, f.account.ID, f.user)
	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/posts/"+post.ID+"/cancel", nil)
	requireStatus(t, rec, http.StatusNoContent)
}

func TestCancelPost_NotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/posts/"+uuid.NewString()+"/cancel", nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestCancelPost_ScopeEnforcement(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	post := seedScheduledPost(t, f.store, f.team.ID, f.account.ID, f.user)

	// A read-only token may not cancel a scheduled post (needs write:schedule).
	readOnly, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "ro", nil, `["read"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	rec := doWithBearer(t, f.handler, http.MethodPost, "/v1/teams/"+f.team.ID+"/posts/"+post.ID+"/cancel", readOnly, nil)
	requireStatus(t, rec, http.StatusForbidden)
}

// ── Update User ───────────────────────────────────────────────────────────────

func TestUpdateUser_PromoteToAdmin(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	target, err := f.store.UpsertOIDCUser(ctx, "upd-"+uuid.NewString(), "upd@example.test", "Upd")
	if err != nil {
		t.Fatal(err)
	}
	if target.IsAdmin {
		t.Fatal("new user should not be admin before promotion")
	}

	rec := f.do(t, http.MethodPatch, "/v1/admin/users/"+target.ID, map[string]any{"is_admin": true})
	requireStatus(t, rec, http.StatusOK)
	updated := decodeJSON[domain.User](t, rec)
	if !updated.IsAdmin {
		t.Error("expected is_admin=true after update")
	}
}

func TestUpdateUser_Demote(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	// Create an admin user.
	target, err := f.store.UpsertOIDCUser(ctx, "dem-"+uuid.NewString(), "dem@example.test", "Dem")
	if err != nil {
		t.Fatal(err)
	}
	target, err = f.store.SetUserAdmin(ctx, target.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !target.IsAdmin {
		t.Fatal("setup: target must be admin")
	}

	rec := f.do(t, http.MethodPatch, "/v1/admin/users/"+target.ID, map[string]any{"is_admin": false})
	requireStatus(t, rec, http.StatusOK)
	updated := decodeJSON[domain.User](t, rec)
	if updated.IsAdmin {
		t.Error("expected is_admin=false after demotion")
	}
}

// ── Accept Team Invitation ────────────────────────────────────────────────────

func TestAcceptTeamInvitation_HappyPath(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	// Create an invitee user.
	invitee, err := f.store.UpsertOIDCUser(ctx, "inv-"+uuid.NewString(), "inv-accept@example.test", "Invitee")
	if err != nil {
		t.Fatal(err)
	}
	inviteeBearer, _, err := f.store.CreateUserAPIToken(ctx, invitee.ID, "inv-tok", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// Team owner creates an invitation for the invitee's email address.
	_, token, err := f.store.CreateTeamInvitation(ctx, f.team.ID, f.user.ID, domain.CreateTeamInvitationInput{
		Email: "inv-accept@example.test",
		Role:  domain.RoleEditor,
	})
	if err != nil {
		t.Fatalf("CreateTeamInvitation: %v", err)
	}

	// Invitee accepts using their own bearer token.
	body, _ := json.Marshal(map[string]any{"token": token})
	req := httptest.NewRequest(http.MethodPost, "/v1/invitations/accept", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+inviteeBearer)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.handler.ServeHTTP(rec, req)
	requireStatus(t, rec, http.StatusOK)
}

func TestAcceptTeamInvitation_MissingToken(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/invitations/accept", map[string]any{"token": ""})
	requireStatus(t, rec, http.StatusBadRequest)
}

func TestAcceptTeamInvitation_InvalidToken(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/invitations/accept", map[string]any{"token": "bogus-token-xyz"})
	requireStatus(t, rec, http.StatusBadRequest)
}

// ── Migrate Account ───────────────────────────────────────────────────────────

func TestMigrateAccount_HappyPath(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	targetTeam, err := f.store.CreateTeam(ctx, f.user.ID, domain.CreateTeamInput{Name: "target-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}

	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/accounts/"+f.account.ID+"/migrate",
		map[string]any{"target_team_id": targetTeam.ID})
	requireStatus(t, rec, http.StatusOK)
	acc := decodeJSON[domain.SocialAccount](t, rec)
	if acc.TeamID != targetTeam.ID {
		t.Errorf("TeamID = %q after migrate, want %q", acc.TeamID, targetTeam.ID)
	}
}

func TestMigrateAccount_MissingTargetTeamID(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/accounts/"+f.account.ID+"/migrate",
		map[string]any{"target_team_id": ""})
	requireStatus(t, rec, http.StatusBadRequest)
}

// ── Admin Logs ────────────────────────────────────────────────────────────────

func TestAdminLogsArchiveUnarchiveDelete(t *testing.T) {
	f := newEndpointFixture(t)
	entry := seedLogEntry(t, f.store)

	// Archive
	rec := f.do(t, http.MethodPost, "/v1/admin/logs/"+entry.ID+"/archive", nil)
	requireStatus(t, rec, http.StatusNoContent)

	// Unarchive
	rec = f.do(t, http.MethodPost, "/v1/admin/logs/"+entry.ID+"/unarchive", nil)
	requireStatus(t, rec, http.StatusNoContent)

	// Delete
	rec = f.do(t, http.MethodDelete, "/v1/admin/logs/"+entry.ID, nil)
	requireStatus(t, rec, http.StatusNoContent)
}

// ── Admin Publish Failures ────────────────────────────────────────────────────

func TestAdminPublishFailures_List(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodGet, "/v1/admin/publish-failures", nil)
	requireStatus(t, rec, http.StatusOK)
	list := decodeJSON[map[string]any](t, rec)
	if _, ok := list["items"]; !ok {
		t.Fatalf("missing items key: %v", list)
	}
}

func TestAdminPublishFailures_AcknowledgeNotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/admin/publish-failures/"+uuid.NewString()+"/acknowledge", nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestAdminPublishFailures_RetryNotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/admin/publish-failures/"+uuid.NewString()+"/retry", nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestAdminPublishFailures_AcknowledgeAndRetry(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	principal := domain.AuthenticatedPrincipal{User: f.user}

	seedFailedPost := func(title string) string {
		t.Helper()
		post, err := f.store.CreateScheduledPost(ctx, f.team.ID, principal, domain.CreatePostInput{
			Title: title, Content: "c", ScheduledAt: time.Now().UTC().Add(-time.Hour),
			TargetAccounts: []string{f.account.ID},
		})
		if err != nil {
			t.Fatalf("CreateScheduledPost %q: %v", title, err)
		}
		if err := f.store.MarkPostTargetResult(ctx, post.ID, f.account.ID, domain.PostStatusFailed, "", "connection refused", nil, ""); err != nil {
			t.Fatalf("MarkPostTargetResult: %v", err)
		}
		if err := f.store.MarkPostResult(ctx, post.ID, 1, domain.PostStatusFailed, "connection refused", nil); err != nil {
			t.Fatalf("MarkPostResult: %v", err)
		}
		return post.ID
	}

	ackPostID := seedFailedPost("Failed Ack Post")
	retryPostID := seedFailedPost("Failed Retry Post")

	rec := f.do(t, http.MethodPost, "/v1/admin/publish-failures/"+ackPostID+"/acknowledge", nil)
	requireStatus(t, rec, http.StatusOK)

	rec = f.do(t, http.MethodPost, "/v1/admin/publish-failures/"+retryPostID+"/retry", nil)
	requireStatus(t, rec, http.StatusOK)
}

// ── Knowledge Sources ─────────────────────────────────────────────────────────

func TestKnowledgeSources_CRUD(t *testing.T) {
	f := newEndpointFixture(t)
	base := "/v1/teams/" + f.team.ID + "/knowledge-sources"

	// List (starts empty)
	rec := f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)
	emptyList := decodeJSON[map[string]any](t, rec)
	if _, ok := emptyList["items"]; !ok {
		t.Fatalf("missing items key: %v", emptyList)
	}

	// Create
	rec = f.do(t, http.MethodPost, base, map[string]any{
		"type":    "text",
		"name":    "Brand Guide",
		"content": "We are a friendly company.",
	})
	requireStatus(t, rec, http.StatusCreated)
	created := decodeJSON[domain.KnowledgeSource](t, rec)
	if created.ID == "" {
		t.Fatal("expected non-empty ID in created knowledge source")
	}
	if created.Name != "Brand Guide" {
		t.Errorf("name = %q, want Brand Guide", created.Name)
	}

	// List (one item)
	rec = f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)
	withOne := decodeJSON[map[string][]domain.KnowledgeSource](t, rec)
	if len(withOne["items"]) != 1 {
		t.Fatalf("expected 1 knowledge source, got %d", len(withOne["items"]))
	}

	// Update
	rec = f.do(t, http.MethodPatch, base+"/"+created.ID, map[string]any{
		"type":    "text",
		"name":    "Brand Guide v2",
		"content": "We are even friendlier.",
	})
	requireStatus(t, rec, http.StatusOK)
	updated := decodeJSON[domain.KnowledgeSource](t, rec)
	if updated.Name != "Brand Guide v2" {
		t.Errorf("updated name = %q, want Brand Guide v2", updated.Name)
	}

	// Delete
	rec = f.do(t, http.MethodDelete, base+"/"+created.ID, nil)
	requireStatus(t, rec, http.StatusNoContent)

	// List (empty again)
	rec = f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)
	afterDelete := decodeJSON[map[string][]domain.KnowledgeSource](t, rec)
	if len(afterDelete["items"]) != 0 {
		t.Fatalf("expected 0 items after delete, got %d", len(afterDelete["items"]))
	}
}

func TestKnowledgeSources_CreateInvalidType(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/knowledge-sources", map[string]any{
		"type":    "invalid-type",
		"name":    "Bad",
		"content": "content",
	})
	requireStatus(t, rec, http.StatusBadRequest)
}

// ── Media ─────────────────────────────────────────────────────────────────────

func TestTeamMediaList(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/media", nil)
	requireStatus(t, rec, http.StatusOK)
	list := decodeJSON[map[string]any](t, rec)
	if _, ok := list["items"]; !ok {
		t.Fatalf("missing items key: %v", list)
	}
}

func TestTeamMediaDelete_HappyPath(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	item, err := f.store.CreateMediaItem(ctx, domain.MediaItem{
		TeamID: f.team.ID, Sha256: "del-" + uuid.NewString(),
		Filename: "todel.png", MimeType: "image/png", SizeBytes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	rec := f.do(t, http.MethodDelete, "/v1/teams/"+f.team.ID+"/media/"+item.ID, nil)
	requireStatus(t, rec, http.StatusNoContent)
}

func TestTeamMediaDelete_NotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodDelete, "/v1/teams/"+f.team.ID+"/media/"+uuid.NewString(), nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestTeamMediaPreview_UnknownID(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/media/"+uuid.NewString()+"/preview", nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestTeamMediaPreview_KnownIDButMissingFile(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	// Create DB record with a valid hex hash but no actual file on disk.
	// GetMediaFilePath will succeed (valid path segments), then http.ServeFile
	// returns 404 because the file doesn't exist. This covers the success path
	// through GetMediaFilePath and the ServeFile call.
	item, err := f.store.CreateMediaItem(ctx, domain.MediaItem{
		TeamID:    f.team.ID,
		Sha256:    "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899",
		Filename:  "test.png",
		MimeType:  "image/png",
		SizeBytes: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The handler sets headers and calls http.ServeFile; ServeFile returns 404
	// when the file is absent from disk.
	rec := f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/media/"+item.ID+"/preview", nil)
	if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
		t.Errorf("expected 200 or 404, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// ── Post Template Linked Posts ────────────────────────────────────────────────

// ── postDeleteScope: draft path ───────────────────────────────────────────────

// seedDraftPost creates a draft post directly in the store.
func seedDraftPost(t *testing.T, s *sqlitestore.Store, teamID, accountID string, user domain.User) domain.ScheduledPost {
	t.Helper()
	ctx := context.Background()
	principal := domain.AuthenticatedPrincipal{User: user}
	post, err := s.CreateScheduledPost(ctx, teamID, principal, domain.CreatePostInput{
		Title:          "draft-post",
		Content:        "draft content",
		ScheduledAt:    time.Now().UTC().Add(24 * time.Hour),
		TargetAccounts: []string{accountID},
		Draft:          true,
	})
	if err != nil {
		t.Fatalf("seedDraftPost: %v", err)
	}
	return post
}

func TestDeleteDraftPost_RequiresDeleteDraftScope(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	post := seedDraftPost(t, f.store, f.team.ID, f.account.ID, f.user)

	// delete:schedule scope is NOT sufficient for deleting a draft.
	schedOnly, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "schedonly", nil, `["delete:schedule"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	rec := doWithBearer(t, f.handler, http.MethodDelete,
		"/v1/teams/"+f.team.ID+"/posts/"+post.ID, schedOnly, nil)
	requireStatus(t, rec, http.StatusForbidden)

	// delete:draft scope IS sufficient.
	draftToken, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "draftdelete", nil, `["delete:draft"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	rec = doWithBearer(t, f.handler, http.MethodDelete,
		"/v1/teams/"+f.team.ID+"/posts/"+post.ID, draftToken, nil)
	requireStatus(t, rec, http.StatusNoContent)
}

// ── Post Template Linked Posts ────────────────────────────────────────────────

func TestListPostTemplateLinkedPosts(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	enabled := true
	tmpl, err := f.store.CreatePostTemplate(ctx, f.team.ID, domain.AuthenticatedPrincipal{User: f.user},
		domain.CreatePostTemplateInput{
			Title:            "Coverage Template",
			Content:          "Template #{counter}",
			RecurrenceJSON:   `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`,
			TargetAccountIDs: []string{f.account.ID},
			Enabled:          &enabled,
		})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}

	rec := f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/post-templates/"+tmpl.ID+"/linked-posts", nil)
	requireStatus(t, rec, http.StatusOK)
	list := decodeJSON[map[string]any](t, rec)
	if _, ok := list["items"]; !ok {
		t.Fatalf("missing items key: %v", list)
	}
}

// ── Update Post ───────────────────────────────────────────────────────────────

func TestUpdatePost_HappyPath(t *testing.T) {
	f := newEndpointFixture(t)
	post := seedScheduledPost(t, f.store, f.team.ID, f.account.ID, f.user)

	// Patch the title.
	newTitle := "Updated Title via PATCH"
	rec := f.do(t, http.MethodPatch, "/v1/teams/"+f.team.ID+"/posts/"+post.ID,
		map[string]any{"title": newTitle})
	requireStatus(t, rec, http.StatusOK)
	updated := decodeJSON[domain.ScheduledPost](t, rec)
	if updated.Title != newTitle {
		t.Errorf("title = %q, want %q", updated.Title, newTitle)
	}
}

func TestUpdatePost_NotFound(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPatch, "/v1/teams/"+f.team.ID+"/posts/"+uuid.NewString(),
		map[string]any{"title": "x"})
	requireStatus(t, rec, http.StatusNotFound)
}

func TestUpdatePost_ScopeEnforcement(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	post := seedScheduledPost(t, f.store, f.team.ID, f.account.ID, f.user)

	// A read-only token cannot update a post.
	readOnly, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "ro-up", nil, `["read"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	rec := doWithBearer(t, f.handler, http.MethodPatch,
		"/v1/teams/"+f.team.ID+"/posts/"+post.ID, readOnly, map[string]any{"title": "x"})
	requireStatus(t, rec, http.StatusForbidden)
}

// ── Admin Log Filters ─────────────────────────────────────────────────────────

func TestAdminLogs_WithFilters(t *testing.T) {
	f := newEndpointFixture(t)
	seedLogEntry(t, f.store)

	// level filter
	rec := f.do(t, http.MethodGet, "/v1/admin/logs?level=info", nil)
	requireStatus(t, rec, http.StatusOK)

	// archived filter
	rec = f.do(t, http.MethodGet, "/v1/admin/logs?archived=true", nil)
	requireStatus(t, rec, http.StatusOK)

	// search filter
	rec = f.do(t, http.MethodGet, "/v1/admin/logs?search=coverage", nil)
	requireStatus(t, rec, http.StatusOK)

	// limit+offset filters
	rec = f.do(t, http.MethodGet, "/v1/admin/logs?limit=10&offset=0", nil)
	requireStatus(t, rec, http.StatusOK)

	// time filters (before and after)
	now := time.Now().UTC()
	rec = f.do(t, http.MethodGet, "/v1/admin/logs?before="+now.Add(time.Hour).Format(time.RFC3339), nil)
	requireStatus(t, rec, http.StatusOK)

	rec = f.do(t, http.MethodGet, "/v1/admin/logs?after="+now.Add(-time.Hour).Format(time.RFC3339), nil)
	requireStatus(t, rec, http.StatusOK)
}

func TestAdminLogs_PruneWithBeforeFilter(t *testing.T) {
	f := newEndpointFixture(t)
	seedLogEntry(t, f.store)

	before := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	rec := f.do(t, http.MethodPost, "/v1/admin/logs/prune?before="+before,
		map[string]any{"older_than_days": 1})
	requireStatus(t, rec, http.StatusOK)
}

// --- Proactive trigger settings ---

func TestProactiveSettings_GetDefaultsAndUpsert(t *testing.T) {
	f := newEndpointFixture(t)
	base := "/v1/teams/" + f.team.ID + "/proactive-settings"

	// GET before any upsert returns the defaults.
	rec := f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)
	defaults := decodeJSON[domain.ProactiveTriggerSettings](t, rec)
	if defaults.MaxTriggersPerDay != 5 {
		t.Errorf("default max_triggers_per_day = %d, want 5", defaults.MaxTriggersPerDay)
	}
	if defaults.CronSchedule == "" {
		t.Error("default cron_schedule is empty")
	}

	// PUT stores new settings.
	rec = f.do(t, http.MethodPut, base, map[string]any{
		"content_gap_threshold_days": 7,
		"auto_fill_enabled":          true,
		"max_triggers_per_day":       2,
		"cron_schedule":              "30 8 * * *",
	})
	requireStatus(t, rec, http.StatusOK)
	saved := decodeJSON[domain.ProactiveTriggerSettings](t, rec)
	if saved.ContentGapThresholdDays != 7 || !saved.AutoFillEnabled || saved.MaxTriggersPerDay != 2 || saved.CronSchedule != "30 8 * * *" {
		t.Errorf("saved settings = %+v", saved)
	}

	// GET reflects the stored settings.
	rec = f.do(t, http.MethodGet, base, nil)
	requireStatus(t, rec, http.StatusOK)
	got := decodeJSON[domain.ProactiveTriggerSettings](t, rec)
	if got.ContentGapThresholdDays != 7 || got.MaxTriggersPerDay != 2 {
		t.Errorf("get after upsert = %+v", got)
	}
}

func TestProactiveSettings_UpsertInvalidBody(t *testing.T) {
	f := newEndpointFixture(t)
	// A JSON string is not decodable into the settings struct.
	rec := f.do(t, http.MethodPut, "/v1/teams/"+f.team.ID+"/proactive-settings", "not-an-object")
	requireStatus(t, rec, http.StatusBadRequest)
}
