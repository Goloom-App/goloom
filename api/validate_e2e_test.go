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

func newValidateE2EStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	enc, err := security.NewEncrypter("api-validate-e2e-secret-32bytes!!")
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

// seedValidateE2E creates a user, team, and two accounts (Bluesky 300, Mastodon 500).
// Returns (bearerToken, teamID, blueskyAccountID, mastodonAccountID).
func seedValidateE2E(t *testing.T, s *sqlitestore.Store) (bearer, teamID, bsID, mastoID string) {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "e2e-val-"+uuid.NewString(), "e2e@val.test", "E2E Validation")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "e2e-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	plain, _, err := s.CreateUserAPIToken(ctx, u.ID, "e2e-val-token", nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	bsAcc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "bluesky", AuthType: domain.AccountAuthTypeAppPassword,
		InstanceURL: "https://bsky.social", Username: "e2e-bs", AccessToken: "pw",
	})
	if err != nil {
		t.Fatal(err)
	}

	mastoAcc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://masto.example", Username: "e2e-masto", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}

	return plain, team.ID, bsAcc.ID, mastoAcc.ID
}

func validateE2EHandler(t *testing.T, s *sqlitestore.Store) http.Handler {
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
	h := api.New(logger, s, authSvc, reg, config.Config{}, nil, catalog, nil, nil)
	return h.Handler(security.NewLimiter(10_000, 10_000), nil)
}

func TestValidateE2E_CrossPost_DifferentLimits(t *testing.T) {
	s := newValidateE2EStore(t)
	token, teamID, bsID, mastoID := seedValidateE2E(t, s)
	h := validateE2EHandler(t, s)

	t.Run("content exceeds Bluesky but fits Mastodon", func(t *testing.T) {
		body := validateBody(t, 418, []string{bsID, mastoID}, nil)
		req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+teamID+"/posts/validate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}

		var resp validationResponseJSON
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		// Overall valid should be false — Bluesky has no override and exceeds its limit
		if resp.Valid {
			t.Error("overall valid = true, want false (Bluesky exceeds 300-char limit, no override)")
		}
		if resp.MaxChars != 300 {
			t.Errorf("maxChars = %d, want 300", resp.MaxChars)
		}
		if resp.ContentLength != 418 {
			t.Errorf("contentLength = %d, want 418", resp.ContentLength)
		}

		// Bluesky should be invalid (418 > 300)
		bs := findDestJSON(resp.Destinations, bsID)
		if bs == nil {
			t.Fatal("Bluesky destination missing")
		}
		if bs.Valid {
			t.Error("Bluesky valid = true, want false (418 > 300)")
		}
		if bs.MaxChars != 300 {
			t.Errorf("Bluesky maxChars = %d, want 300", bs.MaxChars)
		}

		// Mastodon should be valid (418 <= 500)
		masto := findDestJSON(resp.Destinations, mastoID)
		if masto == nil {
			t.Fatal("Mastodon destination missing")
		}
		if !masto.Valid {
			t.Error("Mastodon valid = false, want true (418 <= 500)")
		}
		if masto.MaxChars != 500 {
			t.Errorf("Mastodon maxChars = %d, want 500", masto.MaxChars)
		}
	})

	t.Run("content exceeds both limits", func(t *testing.T) {
		body := validateBody(t, 600, []string{bsID, mastoID}, nil)
		req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+teamID+"/posts/validate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}

		var resp validationResponseJSON
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		// Overall valid should be false — no destination can publish
		if resp.Valid {
			t.Error("overall valid = true, want false (both destinations exceed limits)")
		}
	})

	t.Run("partial override leaves overall invalid", func(t *testing.T) {
		body := validateBody(t, 600, []string{bsID, mastoID}, map[string]string{
			mastoID: string(runeLenE2E(400)),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+teamID+"/posts/validate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}

		var resp validationResponseJSON
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		// Overall valid should be false — Bluesky still has no valid override
		if resp.Valid {
			t.Error("overall valid = true, want false (Bluesky exceeds limit, no override)")
		}

		masto := findDestJSON(resp.Destinations, mastoID)
		if masto == nil {
			t.Fatal("Mastodon destination missing")
		}
		if !masto.Valid {
			t.Error("Mastodon valid = false, want true (override 400 <= 500)")
		}
		if masto.Length != 400 {
			t.Errorf("Mastodon length = %d, want 400 (override)", masto.Length)
		}
	})

	t.Run("all overrides fit — overall valid", func(t *testing.T) {
		body := validateBody(t, 600, []string{bsID, mastoID}, map[string]string{
			bsID:    string(runeLenE2E(200)),
			mastoID: string(runeLenE2E(400)),
		})
		req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+teamID+"/posts/validate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}

		var resp validationResponseJSON
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		if !resp.Valid {
			t.Error("overall valid = false, want true (both destinations have valid overrides)")
		}
		for _, id := range []string{bsID, mastoID} {
			d := findDestJSON(resp.Destinations, id)
			if d == nil {
				t.Fatalf("destination %s not found", id)
			}
			if !d.Valid {
				t.Errorf("destination %s valid = false, want true (override within limit)", id)
			}
		}
	})

	t.Run("content fits both limits", func(t *testing.T) {
		body := validateBody(t, 250, []string{bsID, mastoID}, nil)
		req := httptest.NewRequest(http.MethodPost, "/v1/teams/"+teamID+"/posts/validate", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
		}

		var resp validationResponseJSON
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}

		if !resp.Valid {
			t.Error("overall valid = false, want true (all destinations valid)")
		}
		for _, d := range resp.Destinations {
			if !d.Valid {
				t.Errorf("destination %s (length=%d, max=%d) should be valid", d.AccountID, d.Length, d.MaxChars)
			}
		}
	})
}

// validationResponseJSON mirrors the backend's validationResponse for JSON decoding.
type validationResponseJSON struct {
	MaxChars      int                   `json:"max_chars"`
	ContentLength int                   `json:"content_length"`
	Valid         bool                  `json:"valid"`
	Destinations  []destinationInfoJSON `json:"destinations"`
}

type destinationInfoJSON struct {
	AccountID string `json:"account_id"`
	Provider  string `json:"provider"`
	MaxChars  int    `json:"max_chars"`
	Length    int    `json:"length"`
	Valid     bool   `json:"valid"`
}

func findDestJSON(dests []destinationInfoJSON, accountID string) *destinationInfoJSON {
	for i := range dests {
		if dests[i].AccountID == accountID {
			return &dests[i]
		}
	}
	return nil
}

// validateBody builds a JSON body for the validate endpoint.
func validateBody(t *testing.T, contentLen int, targetAccounts []string, accountContentOverride map[string]string) []byte {
	t.Helper()
	payload := map[string]any{
		"content":         string(runeLenE2E(contentLen)),
		"scheduled_at":    time.Now().UTC().Format(time.RFC3339),
		"target_accounts": targetAccounts,
	}
	if len(accountContentOverride) > 0 {
		payload["account_content_override"] = accountContentOverride
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// runeLenE2E returns a string of n 'a' runes.
func runeLenE2E(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
