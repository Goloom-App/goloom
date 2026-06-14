package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

// seedRevokeAccount creates a team plus a Mastodon provider instance pointing at
// instanceURL and a connected OAuth account linked to it. Returns the account.
func seedRevokeAccount(t *testing.T, s *sqlitestore.Store, instanceURL string, authType domain.AccountAuthType) domain.SocialAccount {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "rev-usr-"+uuid.NewString(), "rev@test.test", "Revoke Tester")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "rev-team-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	inst, err := s.CreateProviderInstance(ctx, u.ID, domain.PreparedProviderInstance{
		Provider:     "mastodon",
		Name:         "test instance",
		InstanceURL:  instanceURL,
		ClientID:     "cid",
		ClientSecret: "secret",
		Scopes:       []string{"read", "write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider:           "mastodon",
		AuthType:           authType,
		ProviderInstanceID: inst.ID,
		InstanceURL:        instanceURL,
		Username:           "masto-test",
		AccessToken:        "access-tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

func TestRevokeProviderToken_revokesOAuthAccount(t *testing.T) {
	s := newValidationMemoryStore(t)
	api := newTestAPI(t, s)

	var revoked atomic.Int32
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/revoke" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = r.ParseForm()
		gotToken = r.PostForm.Get("token")
		revoked.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	acc := seedRevokeAccount(t, s, srv.URL, domain.AccountAuthTypeOAuthToken)
	api.revokeProviderToken(context.Background(), acc)

	if revoked.Load() != 1 {
		t.Fatalf("expected exactly one revoke call, got %d", revoked.Load())
	}
	if gotToken != "access-tok" {
		t.Fatalf("revoke token = %q, want access-tok", gotToken)
	}
}

func TestRevokeProviderToken_skipsAppPasswordAccount(t *testing.T) {
	s := newValidationMemoryStore(t)
	api := newTestAPI(t, s)

	var revoked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		revoked.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	acc := seedRevokeAccount(t, s, srv.URL, domain.AccountAuthTypeAppPassword)
	api.revokeProviderToken(context.Background(), acc)

	if revoked.Load() != 0 {
		t.Fatalf("expected no revoke call for app-password account, got %d", revoked.Load())
	}
}
