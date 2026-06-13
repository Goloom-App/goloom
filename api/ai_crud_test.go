package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	internalsse "git.f4mily.net/goloom/internal/sse"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func newAICRUDStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	enc, err := security.NewEncrypter("ai-crud-test-secret-32byteslong!!")
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

func newAICRUDHandler(t *testing.T, s *sqlitestore.Store) http.Handler {
	return newAICRUDHandlerWithManagerAndHub(t, s, nil, nil)
}

func newAICRUDHandlerWithManager(t *testing.T, s *sqlitestore.Store, jobManager *aijobs.Manager) http.Handler {
	return newAICRUDHandlerWithManagerAndHub(t, s, jobManager, nil)
}

func newAICRUDHandlerWithManagerAndHub(t *testing.T, s *sqlitestore.Store, jobManager *aijobs.Manager, hub *internalsse.Hub) http.Handler {
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
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatalf("i18n.Load: %v", err)
	}
	h := api.New(nil, s, authSvc, reg, config.Config{}, nil, catalog, jobManager, hub)
	return h.Handler(security.NewLimiter(10_000, 10_000), nil)
}

type aiCRUDFixture struct {
	store  *sqlitestore.Store
	h      http.Handler
	bearer string
	teamID string
}

func seedAICRUDTeam(t *testing.T, aiEnabled bool) aiCRUDFixture {
	t.Helper()
	ctx := context.Background()
	s := newAICRUDStore(t)
	h := newAICRUDHandler(t, s)

	u, err := s.UpsertOIDCUser(ctx, "ai-crud-"+uuid.NewString(), "aicrud@example.test", "AI CRUD")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ai-team-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if aiEnabled {
		enabled := true
		if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{
			Name:        team.Name,
			IsAIEnabled: &enabled,
		}); err != nil {
			t.Fatal(err)
		}
	}
	plain, _, err := s.CreateUserAPIToken(ctx, u.ID, "test-token", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return aiCRUDFixture{store: s, h: h, bearer: plain, teamID: team.ID}
}

func doRequest(t *testing.T, h http.Handler, method, path, bearer string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAICRUD(t *testing.T) {
	t.Run("TeamProfileUpsertAndGet", func(t *testing.T) {
		f := seedAICRUDTeam(t, true)

		profileInput := map[string]any{
			"style_metadata": map[string]any{
				"tone":            "professional",
				"writing_style":   "concise",
				"target_audience": "developers",
				"brand_voice":     "authoritative",
			},
			"auto_publish_enabled": false,
		}

		rec := doRequest(t, f.h, http.MethodPut, "/v1/teams/"+f.teamID+"/profile", f.bearer, profileInput)
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT profile: got %d, want 200; body: %s", rec.Code, rec.Body.String())
		}

		var created domain.TeamProfile
		if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
			t.Fatalf("decode PUT response: %v", err)
		}
		if created.TeamID != f.teamID {
			t.Errorf("team_id mismatch: got %q, want %q", created.TeamID, f.teamID)
		}

		rec2 := doRequest(t, f.h, http.MethodGet, "/v1/teams/"+f.teamID+"/profile", f.bearer, nil)
		if rec2.Code != http.StatusOK {
			t.Fatalf("GET profile: got %d, want 200; body: %s", rec2.Code, rec2.Body.String())
		}
		var fetched domain.TeamProfile
		if err := json.NewDecoder(rec2.Body).Decode(&fetched); err != nil {
			t.Fatalf("decode GET response: %v", err)
		}
		if fetched.ID != created.ID {
			t.Errorf("profile ID mismatch: got %q, want %q", fetched.ID, created.ID)
		}

		profileInput2 := map[string]any{
			"style_metadata": map[string]any{
				"tone":            "casual",
				"writing_style":   "verbose",
				"target_audience": "everyone",
				"brand_voice":     "friendly",
			},
			"auto_publish_enabled": true,
		}
		rec3 := doRequest(t, f.h, http.MethodPut, "/v1/teams/"+f.teamID+"/profile", f.bearer, profileInput2)
		if rec3.Code != http.StatusOK {
			t.Fatalf("PUT profile (update): got %d; body: %s", rec3.Code, rec3.Body.String())
		}
		var updated domain.TeamProfile
		if err := json.NewDecoder(rec3.Body).Decode(&updated); err != nil {
			t.Fatalf("decode update response: %v", err)
		}
		if !updated.AutoPublishEnabled {
			t.Errorf("expected auto_publish_enabled=true after update")
		}
	})

	t.Run("FeatureFlagBlocks", func(t *testing.T) {
		f := seedAICRUDTeam(t, false)

		rec := doRequest(t, f.h, http.MethodGet, "/v1/teams/"+f.teamID+"/profile", f.bearer, nil)
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 when AI disabled, got %d; body: %s", rec.Code, rec.Body.String())
		}

		rec2 := doRequest(t, f.h, http.MethodGet, "/v1/teams/"+f.teamID+"/campaign-formats", f.bearer, nil)
		if rec2.Code != http.StatusForbidden {
			t.Errorf("campaign-formats: expected 403 when AI disabled, got %d", rec2.Code)
		}

		rec3 := doRequest(t, f.h, http.MethodGet, "/v1/teams/"+f.teamID+"/style-examples", f.bearer, nil)
		if rec3.Code != http.StatusForbidden {
			t.Errorf("style-examples: expected 403 when AI disabled, got %d", rec3.Code)
		}
	})

	t.Run("CampaignFormatCRUD", func(t *testing.T) {
		f := seedAICRUDTeam(t, true)

		createInput := map[string]any{
			"name":              "Weekly Roundup",
			"structure":         map[string]any{"sections": []string{"intro", "body", "cta"}},
			"required_hashtags": []string{"#weekly", "#update"},
			"is_active":         true,
		}

		rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.teamID+"/campaign-formats", f.bearer, createInput)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST campaign-formats: got %d; body: %s", rec.Code, rec.Body.String())
		}
		var created domain.CampaignFormat
		if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
			t.Fatalf("decode create response: %v", err)
		}
		if created.Name != "Weekly Roundup" {
			t.Errorf("name mismatch: got %q", created.Name)
		}

		rec2 := doRequest(t, f.h, http.MethodGet, "/v1/teams/"+f.teamID+"/campaign-formats", f.bearer, nil)
		if rec2.Code != http.StatusOK {
			t.Fatalf("GET campaign-formats: got %d; body: %s", rec2.Code, rec2.Body.String())
		}
		var listResp struct {
			Items []domain.CampaignFormat `json:"items"`
		}
		if err := json.NewDecoder(rec2.Body).Decode(&listResp); err != nil {
			t.Fatalf("decode list response: %v", err)
		}
		if len(listResp.Items) != 1 {
			t.Fatalf("expected 1 format, got %d", len(listResp.Items))
		}

		rec3 := doRequest(t, f.h, http.MethodGet, "/v1/teams/"+f.teamID+"/campaign-formats/"+created.ID, f.bearer, nil)
		if rec3.Code != http.StatusOK {
			t.Fatalf("GET campaign-format by ID: got %d; body: %s", rec3.Code, rec3.Body.String())
		}
		var fetched domain.CampaignFormat
		if err := json.NewDecoder(rec3.Body).Decode(&fetched); err != nil {
			t.Fatalf("decode get response: %v", err)
		}
		if fetched.ID != created.ID {
			t.Errorf("ID mismatch on get: got %q, want %q", fetched.ID, created.ID)
		}

		patchInput := map[string]any{
			"name":              "Monthly Digest",
			"structure":         map[string]any{"sections": []string{"summary"}},
			"required_hashtags": []string{"#monthly"},
			"is_active":         false,
		}
		rec4 := doRequest(t, f.h, http.MethodPatch, "/v1/teams/"+f.teamID+"/campaign-formats/"+created.ID, f.bearer, patchInput)
		if rec4.Code != http.StatusOK {
			t.Fatalf("PATCH campaign-format: got %d; body: %s", rec4.Code, rec4.Body.String())
		}
		var patched domain.CampaignFormat
		if err := json.NewDecoder(rec4.Body).Decode(&patched); err != nil {
			t.Fatalf("decode patch response: %v", err)
		}
		if patched.Name != "Monthly Digest" {
			t.Errorf("name after patch: got %q, want Monthly Digest", patched.Name)
		}

		rec5 := doRequest(t, f.h, http.MethodDelete, "/v1/teams/"+f.teamID+"/campaign-formats/"+created.ID, f.bearer, nil)
		if rec5.Code != http.StatusNoContent {
			t.Fatalf("DELETE campaign-format: got %d; body: %s", rec5.Code, rec5.Body.String())
		}

		rec6 := doRequest(t, f.h, http.MethodGet, "/v1/teams/"+f.teamID+"/campaign-formats", f.bearer, nil)
		if rec6.Code != http.StatusOK {
			t.Fatalf("GET after delete: got %d", rec6.Code)
		}
		var listAfter struct {
			Items []domain.CampaignFormat `json:"items"`
		}
		if err := json.NewDecoder(rec6.Body).Decode(&listAfter); err != nil {
			t.Fatalf("decode list after delete: %v", err)
		}
		if len(listAfter.Items) != 0 {
			t.Errorf("expected 0 formats after delete, got %d", len(listAfter.Items))
		}
	})
}
