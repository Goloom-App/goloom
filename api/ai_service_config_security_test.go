package api_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

type aiConfigSecurityFixture struct {
	h      http.Handler
	store  *sqlitestore.Store
	teamID string
	team   string
	path   string
	owner  string
	editor string
}

// failingAIConfigStore simulates a database error while reading the config.
type failingAIConfigStore struct{ *sqlitestore.Store }

func (failingAIConfigStore) GetAIServiceConfig(context.Context, string) (domain.AIServiceConfig, error) {
	return domain.AIServiceConfig{}, errors.New("database unavailable")
}

func newAIConfigSecurityFixture(t *testing.T, cfg config.Config) aiConfigSecurityFixture {
	return newAIConfigSecurityFixtureWithStore(t, cfg, func(s *sqlitestore.Store) store.Store { return s })
}

func newAIConfigSecurityFixtureWithStore(t *testing.T, cfg config.Config, wrap func(*sqlitestore.Store) store.Store) aiConfigSecurityFixture {
	t.Helper()
	ctx := context.Background()
	s := newAICRUDStore(t)
	authSvc, err := auth.New(ctx, cfg, s)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}))
	h := api.New(nil, wrap(s), authSvc, reg, cfg, nil, catalog, nil, nil).Handler(security.NewLimiter(10_000, 10_000), nil)

	owner, err := s.UpsertOIDCUser(ctx, "owner-"+uuid.NewString(), "owner@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "ai-sec-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	editor, err := s.UpsertOIDCUser(ctx, "editor-"+uuid.NewString(), "editor@example.test", "Editor")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddTeamMember(ctx, team.ID, domain.AddTeamMemberInput{UserID: editor.ID, Role: domain.RoleEditor}); err != nil {
		t.Fatal(err)
	}
	ownerToken, _, err := s.CreateUserAPIToken(ctx, owner.ID, "owner", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	editorToken, _, err := s.CreateUserAPIToken(ctx, editor.ID, "editor", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return aiConfigSecurityFixture{h: h, store: s, teamID: team.ID, team: team.Name, path: "/v1/teams/" + team.ID + "/ai-service-config", owner: ownerToken, editor: editorToken}
}

func (f aiConfigSecurityFixture) put(t *testing.T, bearer string, body map[string]any) (int, string) {
	t.Helper()
	rec := doRequest(t, f.h, http.MethodPut, f.path, bearer, body)
	return rec.Code, rec.Header().Get("X-Error-Code")
}

func TestAIServiceConfigPutIsOwnerOnly(t *testing.T) {
	f := newAIConfigSecurityFixture(t, config.Config{})

	if code, _ := f.put(t, f.editor, map[string]any{"provider": "openai", "api_key": "sk-editor"}); code != http.StatusForbidden {
		t.Fatalf("editor PUT: status=%d, want 403", code)
	}
	if code, _ := f.put(t, f.owner, map[string]any{"provider": "openai", "api_key": "sk-owner"}); code != http.StatusOK {
		t.Fatalf("owner PUT: status=%d, want 200", code)
	}
	// Editors keep read access (key is never returned).
	if rec := doRequest(t, f.h, http.MethodGet, f.path, f.editor, nil); rec.Code != http.StatusOK {
		t.Fatalf("editor GET: status=%d, want 200", rec.Code)
	}
}

func TestAIServiceConfigEndpointChangeRequiresKey(t *testing.T) {
	f := newAIConfigSecurityFixture(t, config.Config{})
	if code, _ := f.put(t, f.owner, map[string]any{"provider": "openai", "model": "gpt-4o", "api_key": "sk-1"}); code != http.StatusOK {
		t.Fatalf("initial PUT: status=%d", code)
	}

	steps := []struct {
		name     string
		body     map[string]any
		wantCode int
		wantErr  string
	}{
		{"model change keeps key", map[string]any{"provider": "openai", "model": "gpt-5"}, http.StatusOK, ""},
		{"base_url change without key", map[string]any{"provider": "openai", "base_url": "https://8.8.8.8/v1"}, http.StatusBadRequest, "ai_api_key_required"},
		{"provider change without key", map[string]any{"provider": "anthropic"}, http.StatusBadRequest, "ai_api_key_required"},
		{"base_url change with key", map[string]any{"provider": "openai", "base_url": "https://8.8.8.8/v1", "api_key": "sk-2"}, http.StatusOK, ""},
		{"same base_url (trailing slash) keeps key", map[string]any{"provider": "openai", "base_url": "https://8.8.8.8/v1/", "model": "gpt-4o"}, http.StatusOK, ""},
		{"reset to default endpoint without key", map[string]any{"provider": "openai", "base_url": ""}, http.StatusBadRequest, "ai_api_key_required"},
	}
	for _, st := range steps {
		code, errCode := f.put(t, f.owner, st.body)
		if code != st.wantCode || errCode != st.wantErr {
			t.Fatalf("%s: status=%d code=%q, want %d %q", st.name, code, errCode, st.wantCode, st.wantErr)
		}
	}
}

func TestAIServiceConfigRejectsNonPublicBaseURL(t *testing.T) {
	f := newAIConfigSecurityFixture(t, config.Config{})
	for _, baseURL := range []string{
		"http://127.0.0.1:11434/v1",
		"http://localhost:8080",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.5/v1",
		"http://[::1]/v1",
		"ftp://8.8.8.8/",
		"https://user:pw@8.8.8.8/v1",
		"not a url",
	} {
		code, errCode := f.put(t, f.owner, map[string]any{"provider": "openai", "base_url": baseURL, "api_key": "sk-1"})
		if code != http.StatusBadRequest || errCode != "invalid_ai_base_url" {
			t.Errorf("base_url %q: status=%d code=%q, want 400 invalid_ai_base_url", baseURL, code, errCode)
		}
	}
}

func TestAIServiceConfigAllowsAdminAllowlistedPrivateHost(t *testing.T) {
	f := newAIConfigSecurityFixture(t, config.Config{AIAllowedPrivateHosts: []string{"127.0.0.1"}})
	code, errCode := f.put(t, f.owner, map[string]any{"provider": "openai", "base_url": "http://127.0.0.1:11434/v1", "api_key": "ollama"})
	if code != http.StatusOK {
		t.Fatalf("allowlisted host: status=%d code=%q", code, errCode)
	}
	code, errCode = f.put(t, f.owner, map[string]any{"provider": "openai", "base_url": "http://10.0.0.5/v1", "api_key": "ollama"})
	if code != http.StatusBadRequest || errCode != "invalid_ai_base_url" {
		t.Fatalf("non-allowlisted private host: status=%d code=%q", code, errCode)
	}
}

func TestAIServiceConfigStoreErrorDoesNotKeepKeyForNewEndpoint(t *testing.T) {
	f := newAIConfigSecurityFixtureWithStore(t, config.Config{}, func(s *sqlitestore.Store) store.Store { return failingAIConfigStore{s} })
	if _, err := f.store.UpsertAIServiceConfig(context.Background(), f.teamID, domain.AIServiceConfig{Provider: "openai", APIKey: "sk-1"}); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.put(t, f.owner, map[string]any{"provider": "openai", "base_url": "https://8.8.8.8/v1"}); code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500 when the stored config cannot be read", code)
	}
	cfg, err := f.store.GetAIServiceConfig(context.Background(), f.teamID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "" {
		t.Fatalf("base_url changed to %q despite failed key check", cfg.BaseURL)
	}
}

func TestAIChatUsesAllowlistedPrivateHost(t *testing.T) {
	hit := make(chan struct{}, 1)
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case hit <- struct{}{}:
		default:
		}
		io.WriteString(w, `{"model":"gpt-4o","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"hi"}}]}`)
	}))
	defer llm.Close()

	f := newAIConfigSecurityFixture(t, config.Config{AIAllowedPrivateHosts: []string{"127.0.0.1"}})
	ctx := context.Background()
	enabled := true
	if _, err := f.store.UpdateTeam(ctx, f.teamID, domain.UpdateTeamInput{Name: f.team, IsAIEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.UpsertAIServiceConfig(ctx, f.teamID, domain.AIServiceConfig{Provider: "openai", Model: "gpt-4o", BaseURL: llm.URL, APIKey: "sk-1"}); err != nil {
		t.Fatal(err)
	}

	doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.teamID+"/ai/chat", f.owner, map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hello"}},
	})
	select {
	case <-hit:
	default:
		t.Fatal("chat must reach the admin-allowlisted LLM host")
	}
}
