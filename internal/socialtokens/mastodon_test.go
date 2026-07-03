package socialtokens_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/socialtokens"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

// newTestStore creates a fresh in-memory SQLite store for testing.
func newTestStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	enc, err := security.NewEncrypter("socialtoken-test-secret-32bytes!!")
	if err != nil {
		t.Fatal(err)
	}
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	s, err := sqlitestore.New(context.Background(), dsn, enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedMastodonAccountWithRefresh creates a user, team, provider instance, and a
// social account with a refresh token. The account's AccessTokenExpiresAt is set
// to the given value.
func seedMastodonAccountWithRefresh(t *testing.T, s *sqlitestore.Store, instanceURL string, expiresAt *time.Time) domain.SocialAccount {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "st-usr-"+uuid.NewString(), "st@test.test", "ST Tester")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "st-team-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	inst, err := s.CreateProviderInstance(ctx, u.ID, domain.PreparedProviderInstance{
		Provider:     "mastodon",
		Name:         "test instance",
		InstanceURL:  instanceURL,
		ClientID:     "cid",
		ClientSecret: "cs",
		Scopes:       []string{"read", "write"},
	})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider:             "mastodon",
		AuthType:             domain.AccountAuthTypeOAuthToken,
		ProviderInstanceID:   inst.ID,
		InstanceURL:          instanceURL,
		Username:             "st-masto",
		AccessToken:          "old-access",
		RefreshToken:         "old-refresh",
		AccessTokenExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

// --- Early return: non-mastodon provider ---

func TestEnsureMastodonFresh_NonMastodonProvider(t *testing.T) {
	t.Parallel()
	account := domain.SocialAccount{Provider: "bluesky", ID: "acc1"}
	// Store is nil — must not be accessed for non-mastodon provider.
	got, err := socialtokens.EnsureMastodonFresh(context.Background(), nil, nil, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "acc1" {
		t.Errorf("returned wrong account: %+v", got)
	}
}

// --- Early return: no ProviderInstanceID ---

func TestEnsureMastodonFresh_NoProviderInstance(t *testing.T) {
	t.Parallel()
	exp := time.Now().Add(-time.Hour)
	account := domain.SocialAccount{
		Provider:             "mastodon",
		ProviderInstanceID:  "",
		AccessTokenExpiresAt: &exp,
	}
	got, err := socialtokens.EnsureMastodonFresh(context.Background(), nil, nil, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Provider != "mastodon" {
		t.Errorf("unexpected account returned: %+v", got)
	}
}

// --- Early return: nil AccessTokenExpiresAt ---

func TestEnsureMastodonFresh_NilExpiry(t *testing.T) {
	t.Parallel()
	account := domain.SocialAccount{
		Provider:            "mastodon",
		ProviderInstanceID: "inst-1",
		AccessTokenExpiresAt: nil,
	}
	got, err := socialtokens.EnsureMastodonFresh(context.Background(), nil, nil, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ProviderInstanceID != "inst-1" {
		t.Errorf("unexpected account: %+v", got)
	}
}

// --- Early return: token still fresh ---

func TestEnsureMastodonFresh_TokenFresh(t *testing.T) {
	t.Parallel()
	// Expires in 10 minutes → more than 2-minute threshold → no refresh.
	exp := time.Now().Add(10 * time.Minute)
	account := domain.SocialAccount{
		Provider:             "mastodon",
		ProviderInstanceID:  "inst-1",
		AccessTokenExpiresAt: &exp,
	}
	got, err := socialtokens.EnsureMastodonFresh(context.Background(), nil, nil, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ProviderInstanceID != "inst-1" {
		t.Errorf("unexpected account: %+v", got)
	}
}

// --- Early return: empty refresh token (no ciphertext stored) ---

func TestEnsureMastodonFresh_EmptyRefreshToken(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	// Create a minimal user+team+instance+account with no refresh token.
	u, _ := s.UpsertOIDCUser(ctx, "er-"+uuid.NewString(), "er@test.test", "ER")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "er-team"})
	inst, _ := s.CreateProviderInstance(ctx, u.ID, domain.PreparedProviderInstance{
		Provider: "mastodon", Name: "i", InstanceURL: "https://example.com",
		ClientID: "c", ClientSecret: "s",
	})
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider:           "mastodon",
		AuthType:           domain.AccountAuthTypeOAuthToken,
		ProviderInstanceID: inst.ID,
		InstanceURL:        "https://example.com",
		Username:           "masto",
		AccessToken:        "tok",
		// RefreshToken intentionally empty → DecryptRefreshToken returns "".
	})
	if err != nil {
		t.Fatal(err)
	}
	// Inject an expiry in the past so the function doesn't short-circuit earlier.
	exp := time.Now().Add(-time.Hour)
	acc.AccessTokenExpiresAt = &exp

	reg := provider.NewRegistry() // empty — shouldn't be reached
	got, err := socialtokens.EnsureMastodonFresh(ctx, s, reg, acc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != acc.ID {
		t.Errorf("returned wrong account: %+v", got)
	}
}

// --- Early return: provider not in registry ---

func TestEnsureMastodonFresh_ProviderNotInRegistry(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Seed an account with a refresh token so we get past the empty-refresh check.
	exp := time.Now().Add(-time.Minute)
	acc := seedMastodonAccountWithRefresh(t, s, "https://mastodon.example.com", &exp)

	// Empty registry → providers.Get("mastodon") returns false.
	reg := provider.NewRegistry()
	got, err := socialtokens.EnsureMastodonFresh(context.Background(), s, reg, acc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != acc.ID {
		t.Errorf("returned wrong account: %+v", got)
	}
}

// --- Early return: provider does not implement OAuthTokenRefresher ---

// nonRefresherProvider satisfies provider.SocialMediaProvider but NOT OAuthTokenRefresher.
type nonRefresherProvider struct{}

func (n *nonRefresherProvider) Name() string { return "mastodon" }
func (n *nonRefresherProvider) Capabilities(_ context.Context, _ domain.SocialAccount) (provider.Capabilities, error) {
	return provider.Capabilities{}, nil
}
func (n *nonRefresherProvider) PrepareProviderInstance(_ context.Context, _ domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error) {
	return domain.PreparedProviderInstance{}, nil
}
func (n *nonRefresherProvider) ConnectAccount(_ context.Context, _ domain.CreateAccountInput, _ *domain.ProviderInstance) (domain.ConnectedAccount, error) {
	return domain.ConnectedAccount{}, nil
}
func (n *nonRefresherProvider) UploadMedia(_ context.Context, _ domain.SocialAccount, _ provider.PublishAuth, _ io.Reader, _, _, _ string) (string, error) {
	return "", nil
}
func (n *nonRefresherProvider) Publish(_ context.Context, _ domain.SocialAccount, _ provider.PublishAuth, _ provider.PublishRequest) (provider.PublishResult, error) {
	return provider.PublishResult{}, nil
}
func (n *nonRefresherProvider) GetMetrics(_ context.Context, _ domain.SocialAccount, _ provider.PublishAuth, _ string) ([]provider.EngagementMetric, error) {
	return nil, nil
}
func (n *nonRefresherProvider) GetAccountMetrics(_ context.Context, _ domain.SocialAccount, _ provider.PublishAuth) ([]provider.AccountMetric, error) {
	return nil, nil
}

func TestEnsureMastodonFresh_ProviderNoRefresher(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	exp := time.Now().Add(-time.Minute)
	acc := seedMastodonAccountWithRefresh(t, s, "https://mastodon.example.com", &exp)

	// Registry has mastodon but without OAuthTokenRefresher.
	reg := provider.NewRegistry(&nonRefresherProvider{})
	got, err := socialtokens.EnsureMastodonFresh(context.Background(), s, reg, acc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != acc.ID {
		t.Errorf("returned wrong account: %+v", got)
	}
}

// --- Happy path: token refresh succeeds ---

func TestEnsureMastodonFresh_HappyPath(t *testing.T) {
	t.Parallel()

	// Set up a fake OAuth token endpoint.
	tokenResp := map[string]interface{}{
		"access_token":  "new-access",
		"refresh_token": "new-refresh",
		"token_type":    "Bearer",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" {
			http.Error(w, "unexpected grant_type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResp)
	}))
	defer srv.Close()

	s := newTestStore(t)
	exp := time.Now().Add(-time.Minute) // expired — needs refresh
	acc := seedMastodonAccountWithRefresh(t, s, srv.URL, &exp)

	reg := provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}))
	got, err := socialtokens.EnsureMastodonFresh(context.Background(), s, reg, acc)
	if err != nil {
		t.Fatalf("EnsureMastodonFresh: %v", err)
	}
	// After refresh the account is re-fetched from the store; ID must be the same.
	if got.ID != acc.ID {
		t.Errorf("got account ID %q, want %q", got.ID, acc.ID)
	}
}
