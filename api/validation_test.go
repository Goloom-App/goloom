package api

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func newValidationMemoryStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	enc, err := security.NewEncrypter("api-validation-test-secret-32bytes!!")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	s, err := sqlitestore.New(ctx, dsn, enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newTestAPI creates an API instance with in-memory SQLite for validation tests.
func newTestAPI(t *testing.T, s *sqlitestore.Store) *API {
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
	return New(logger, s, authSvc, reg, config.Config{}, nil, catalog, nil, nil)
}

// seedCrossPostAccounts creates a team with two accounts: Bluesky (300 chars) and Mastodon (500 chars).
// Returns (teamID, blueskyAccountID, mastodonAccountID).
func seedCrossPostAccounts(t *testing.T, s *sqlitestore.Store) (teamID string, bsID string, mastoID string) {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "val-usr-"+uuid.NewString(), "val@test.test", "Validation Tester")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "val-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}

	bsAcc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "bluesky", AuthType: domain.AccountAuthTypeAppPassword,
		InstanceURL: "https://bsky.social", Username: "bsky-test", AccessToken: "app-pw",
	})
	if err != nil {
		t.Fatal(err)
	}

	mastoAcc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://mastodon.social", Username: "masto-test", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}

	return team.ID, bsAcc.ID, mastoAcc.ID
}

func TestValidatePost_OverallInvalid_WhenNotAllValid(t *testing.T) {
	// Content fits Mastodon (500) but exceeds Bluesky (300).
	// No per-account override for Bluesky — overall valid should be false.
	s := newValidationMemoryStore(t)
	api := newTestAPI(t, s)
	teamID, bsID, mastoID := seedCrossPostAccounts(t, s)

	ctx := context.Background()
	resp, _, err := api.validatePostInput(ctx, teamID, domain.CreatePostInput{
		Content:        string(runeLen(418)),
		ScheduledAt:    time.Now().UTC(),
		TargetAccounts: []string{bsID, mastoID},
	})
	if err != nil {
		t.Fatalf("validatePostInput: %v", err)
	}

	if resp.Valid {
		t.Errorf("overall valid = true, want false (Bluesky exceeds 300-char limit, no override)")
	}
	if resp.MaxChars != 300 {
		t.Errorf("maxChars = %d, want 300 (minimum across destinations)", resp.MaxChars)
	}
	if resp.ContentLength != 418 {
		t.Errorf("contentLength = %d, want 418", resp.ContentLength)
	}
	if len(resp.Destinations) != 2 {
		t.Fatalf("got %d destinations, want 2", len(resp.Destinations))
	}

	// Bluesky should be invalid (418 > 300)
	bs := findDest(resp.Destinations, bsID)
	if bs == nil {
		t.Fatal("Bluesky destination not found")
	}
	if bs.Valid {
		t.Errorf("Bluesky valid = true, want false (418 > 300)")
	}
	if bs.MaxChars != 300 {
		t.Errorf("Bluesky maxChars = %d, want 300", bs.MaxChars)
	}

	// Mastodon should be valid (418 <= 500)
	masto := findDest(resp.Destinations, mastoID)
	if masto == nil {
		t.Fatal("Mastodon destination not found")
	}
	if !masto.Valid {
		t.Errorf("Mastodon valid = false, want true (418 <= 500)")
	}
	if masto.MaxChars != 500 {
		t.Errorf("Mastodon maxChars = %d, want 500", masto.MaxChars)
	}
}

func TestValidatePost_OverallInvalid_WhenNoValidDestination(t *testing.T) {
	// Content exceeds both Bluesky (300) and Mastodon (500).
	// Overall valid should be false — no destination can publish.
	s := newValidationMemoryStore(t)
	api := newTestAPI(t, s)
	teamID, bsID, mastoID := seedCrossPostAccounts(t, s)

	ctx := context.Background()
	resp, _, err := api.validatePostInput(ctx, teamID, domain.CreatePostInput{
		Content:        string(runeLen(600)),
		ScheduledAt:    time.Now().UTC(),
		TargetAccounts: []string{bsID, mastoID},
	})
	if err != nil {
		t.Fatalf("validatePostInput: %v", err)
	}

	if resp.Valid {
		t.Errorf("overall valid = true, want false (both destinations exceed their limits)")
	}
}

func TestValidatePost_OverallInvalid_WithPartialOverrides(t *testing.T) {
	// Main content is 600 chars (exceeds both limits).
	// Mastodon has a per-account override of 400 chars (valid).
	// Bluesky has NO override — still invalid → overall valid false.
	s := newValidationMemoryStore(t)
	api := newTestAPI(t, s)
	teamID, bsID, mastoID := seedCrossPostAccounts(t, s)

	ctx := context.Background()
	resp, _, err := api.validatePostInput(ctx, teamID, domain.CreatePostInput{
		Content:        string(runeLen(600)),
		ScheduledAt:    time.Now().UTC(),
		TargetAccounts: []string{bsID, mastoID},
		AccountContentOverride: map[string]string{
			mastoID: string(runeLen(400)),
		},
	})
	if err != nil {
		t.Fatalf("validatePostInput: %v", err)
	}

	if resp.Valid {
		t.Errorf("overall valid = true, want false (Bluesky still exceeds limit, no override)")
	}

	// Bluesky (no override, uses main 600 chars) should be invalid
	bs := findDest(resp.Destinations, bsID)
	if bs == nil {
		t.Fatal("Bluesky destination not found")
	}
	if bs.Valid {
		t.Errorf("Bluesky valid = true, want false (600 > 300, no override)")
	}
	if bs.Length != 600 {
		t.Errorf("Bluesky length = %d, want 600 (main content)", bs.Length)
	}

	// Mastodon (override 400 chars) should be valid
	masto := findDest(resp.Destinations, mastoID)
	if masto == nil {
		t.Fatal("Mastodon destination not found")
	}
	if !masto.Valid {
		t.Errorf("Mastodon valid = false, want true (override 400 <= 500)")
	}
	if masto.Length != 400 {
		t.Errorf("Mastodon length = %d, want 400 (override content)", masto.Length)
	}
}

func TestValidatePost_OverallValid_WithAllOverrides(t *testing.T) {
	// Main content is 600 chars (exceeds both limits).
	// Both destinations have per-account overrides within their limits.
	// Overall valid should be true — all destinations are covered.
	s := newValidationMemoryStore(t)
	api := newTestAPI(t, s)
	teamID, bsID, mastoID := seedCrossPostAccounts(t, s)

	ctx := context.Background()
	resp, _, err := api.validatePostInput(ctx, teamID, domain.CreatePostInput{
		Content:        string(runeLen(600)),
		ScheduledAt:    time.Now().UTC(),
		TargetAccounts: []string{bsID, mastoID},
		AccountContentOverride: map[string]string{
			bsID:    string(runeLen(200)), // within Bluesky 300
			mastoID: string(runeLen(400)), // within Mastodon 500
		},
	})
	if err != nil {
		t.Fatalf("validatePostInput: %v", err)
	}

	if !resp.Valid {
		t.Errorf("overall valid = false, want true (both destinations have valid overrides)")
	}

	// Both destinations should be valid
	for _, id := range []string{bsID, mastoID} {
		d := findDest(resp.Destinations, id)
		if d == nil {
			t.Fatalf("destination %s not found", id)
		}
		if !d.Valid {
			t.Errorf("destination %s valid = false, want true (override within limit)", id)
		}
	}
}

func TestValidatePost_AllDestinationsValid(t *testing.T) {
	// Content fits both Bluesky (300) and Mastodon (500).
	// Overall valid should be true, all destinations valid.
	s := newValidationMemoryStore(t)
	api := newTestAPI(t, s)
	teamID, bsID, mastoID := seedCrossPostAccounts(t, s)

	ctx := context.Background()
	resp, _, err := api.validatePostInput(ctx, teamID, domain.CreatePostInput{
		Content:        string(runeLen(250)),
		ScheduledAt:    time.Now().UTC(),
		TargetAccounts: []string{bsID, mastoID},
	})
	if err != nil {
		t.Fatalf("validatePostInput: %v", err)
	}

	if !resp.Valid {
		t.Errorf("overall valid = false, want true (all destinations within limits)")
	}
	for _, d := range resp.Destinations {
		if !d.Valid {
			t.Errorf("destination %s (provider=%s) valid = false, want true (length %d <= max %d)",
				d.AccountID, d.Provider, d.Length, d.MaxChars)
		}
	}
}

func findDest(dests []destinationInfo, accountID string) *destinationInfo {
	for i := range dests {
		if dests[i].AccountID == accountID {
			return &dests[i]
		}
	}
	return nil
}

// runeLen returns a string of n runes (each 'a') for deterministic length testing.
func runeLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
