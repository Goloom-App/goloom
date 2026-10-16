package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/ai"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
)

type recordingObserver struct {
	mu    sync.Mutex
	calls []ai.CallMetrics
}

func (r *recordingObserver) ObserveCall(_ context.Context, m ai.CallMetrics) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, m)
}

func (r *recordingObserver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// The chat endpoint must build its LLM client through the injected ai.Service,
// so every chat call reaches the same observer as job calls.
func TestAIChatUsesInjectedServiceObserver(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-test","choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`))
	}))
	defer llm.Close()

	fx := seedAICRUDTeam(t, true)
	ctx := context.Background()
	if _, err := fx.store.UpsertAIServiceConfig(ctx, fx.teamID, domain.AIServiceConfig{
		Provider: ai.ProviderOpenAI, Model: "gpt-test", BaseURL: llm.URL, APIKey: "sk-test",
	}); err != nil {
		t.Fatal(err)
	}

	obs := &recordingObserver{}
	svc := ai.NewService()
	svc.SetObserver(obs)

	authSvc, err := auth.New(ctx, config.Config{}, fx.store)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	reg := provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}))
	a := api.New(nil, fx.store, authSvc, reg, config.Config{}, nil, catalog, nil, nil)
	a.SetAIService(svc)
	h := a.Handler(security.NewLimiter(10_000, 10_000), nil)

	rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+fx.teamID+"/ai/chat", fx.bearer, map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hello"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"hi"`) {
		t.Fatalf("expected assistant message in stream, got %s", rec.Body.String())
	}
	if obs.count() == 0 {
		t.Fatal("chat LLM call did not reach the injected service observer")
	}
}
