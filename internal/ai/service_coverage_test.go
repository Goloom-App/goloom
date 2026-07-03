package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

// ---- NewService ----

func TestNewService_ReturnsNonNil(t *testing.T) {
	svc := NewService()
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

// ---- SetObserver ----

func TestSetObserver_NilRestoresDefault(t *testing.T) {
	svc := NewService()
	// Setting nil must not panic and must restore the default LogObserver.
	svc.SetObserver(nil)
	if svc.observer == nil {
		t.Fatal("observer must not be nil after SetObserver(nil)")
	}
	if _, ok := svc.observer.(LogObserver); !ok {
		t.Fatalf("expected LogObserver after nil reset, got %T", svc.observer)
	}
}

func TestSetObserver_CustomObserverIsStored(t *testing.T) {
	svc := NewService()
	obs := &recordingObserver{}
	svc.SetObserver(obs)
	if svc.observer != obs {
		t.Fatal("custom observer was not stored")
	}
}

// ---- SettingsFromConfig ----

func TestSettingsFromConfig_WithTeamID(t *testing.T) {
	teamID := "team-abc"
	cfg := domain.AIServiceConfig{
		Provider: ProviderOpenAI,
		Model:    "gpt-4o",
		APIKey:   "sk-test",
		BaseURL:  "https://api.example.com",
		TeamID:   &teamID,
	}
	s := SettingsFromConfig(cfg)
	if s.Team != teamID {
		t.Fatalf("Team = %q, want %q", s.Team, teamID)
	}
	if s.Provider != ProviderOpenAI {
		t.Fatalf("Provider = %q, want openai", s.Provider)
	}
	if s.Model != "gpt-4o" {
		t.Fatalf("Model = %q, want gpt-4o", s.Model)
	}
	if s.APIKey != "sk-test" {
		t.Fatalf("APIKey = %q, want sk-test", s.APIKey)
	}
}

func TestSettingsFromConfig_NilTeamIDYieldsEmptyTeam(t *testing.T) {
	cfg := domain.AIServiceConfig{
		Provider: ProviderAnthropic,
		Model:    "claude-opus",
		APIKey:   "ak-test",
		TeamID:   nil,
	}
	s := SettingsFromConfig(cfg)
	if s.Team != "" {
		t.Fatalf("Team = %q, want empty string for nil TeamID", s.Team)
	}
	if s.Provider != ProviderAnthropic {
		t.Fatalf("Provider = %q, want anthropic", s.Provider)
	}
}

// ---- ClientFor ----

func TestClientFor_MissingAPIKeyReturnsErrNotConfigured(t *testing.T) {
	svc := NewService()
	cfg := domain.AIServiceConfig{Provider: ProviderOpenAI, Model: "gpt-4o", APIKey: ""}
	_, err := svc.ClientFor(cfg)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestClientFor_ValidConfigReturnsClient(t *testing.T) {
	svc := NewService()
	cfg := domain.AIServiceConfig{Provider: ProviderOpenAI, Model: "gpt-4o", APIKey: "sk-test"}
	client, err := svc.ClientFor(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

// ---- RunJob ----

func TestRunJob_UnknownJobTypeReturnsError(t *testing.T) {
	svc := NewService()
	job := domain.AIJob{Type: "unknown_type_xyz"}
	cfg := domain.AIServiceConfig{APIKey: "sk-test", Provider: ProviderOpenAI}
	_, err := svc.RunJob(context.Background(), job, cfg, testContext())
	if err == nil || !strings.Contains(err.Error(), "unknown ai job type") {
		t.Fatalf("expected 'unknown ai job type' error, got %v", err)
	}
}

func TestRunJob_MissingAPIKeyReturnsErrBeforeWorker(t *testing.T) {
	svc := NewService()
	job := domain.AIJob{Type: domain.AIJobTypeVoiceEngine}
	cfg := domain.AIServiceConfig{Provider: ProviderOpenAI, APIKey: ""}
	_, err := svc.RunJob(context.Background(), job, cfg, testContext())
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured for missing API key, got %v", err)
	}
}

func TestRunJob_AllExpectedJobTypesAreRegistered(t *testing.T) {
	expected := []domain.AIJobType{
		domain.AIJobTypeVoiceEngine,
		domain.AIJobTypeCampaignAutopilot,
		domain.AIJobTypeProfileAnalysis,
		domain.AIJobTypeProfileAssistant,
		domain.AIJobTypeVibePreview,
	}
	for _, jt := range expected {
		if _, ok := workers[jt]; !ok {
			t.Errorf("workers map missing expected job type %q", jt)
		}
	}
}

func TestRunJob_ProfileAnalysisDispatches(t *testing.T) {
	// Use an httptest server to simulate the LLM so we can verify end-to-end
	// dispatch through Service.RunJob -> runProfileAnalysis -> LLM.
	analysisResponse := `{"tonality":"direct","formatting_rules":[],"banned_words":[],"preferred_language":"en","max_hashtags":2}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"model":"gpt-4o","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":`+
			`"`+strings.ReplaceAll(analysisResponse, `"`, `\"`)+`"}}]}`)
	}))
	defer server.Close()

	svc := NewService()
	svc.httpClient = server.Client()

	cfg := domain.AIServiceConfig{
		Provider: ProviderOpenAI,
		Model:    "gpt-4o",
		APIKey:   "sk-test",
		BaseURL:  server.URL,
	}
	aiContext := testContext()
	aiContext.RecentPosts = []domain.ScheduledPost{
		{ID: "p1", Content: "Our first feature is live and ready for the world to see!"},
	}
	job := domain.AIJob{Type: domain.AIJobTypeProfileAnalysis}

	raw, err := svc.RunJob(context.Background(), job, cfg, aiContext)
	if err != nil {
		t.Fatalf("RunJob: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("RunJob returned empty result")
	}
}
