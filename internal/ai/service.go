package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"git.f4mily.net/goloom/internal/domain"
)

// Service executes AI jobs in-process using the team's configured LLM provider.
type Service struct {
	httpClient *http.Client
	observer   Observer
}

func NewService() *Service {
	return &Service{httpClient: &http.Client{Timeout: requestTimeout}, observer: LogObserver{}}
}

// SetObserver overrides the metrics observer used for every LLM call. Passing
// nil restores the default slog-backed observer.
func (s *Service) SetObserver(obs Observer) {
	if obs == nil {
		obs = LogObserver{}
	}
	s.observer = obs
}

// SettingsFromConfig maps the stored team configuration to client settings.
func SettingsFromConfig(cfg domain.AIServiceConfig) Settings {
	team := ""
	if cfg.TeamID != nil {
		team = *cfg.TeamID
	}
	return Settings{
		Provider: cfg.Provider,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
		Team:     team,
	}
}

// ClientFor builds an LLM client for the team configuration.
func (s *Service) ClientFor(cfg domain.AIServiceConfig) (Client, error) {
	return NewClientWithObserver(SettingsFromConfig(cfg), s.httpClient, s.observer)
}

type workerFunc func(ctx context.Context, client Client, job domain.AIJob, aiContext domain.AIContext, p params) (json.RawMessage, error)

var workers = map[domain.AIJobType]workerFunc{
	domain.AIJobTypeVoiceEngine:       runVoiceEngine,
	domain.AIJobTypeCampaignAutopilot: runCampaignAutopilot,
	domain.AIJobTypeProfileAnalysis:   runProfileAnalysis,
	domain.AIJobTypeProfileAssistant:  runProfileAssistant,
	domain.AIJobTypeVibePreview:       runVibePreview,
}

// RunJob executes one AI job and returns its result payload.
func (s *Service) RunJob(ctx context.Context, job domain.AIJob, cfg domain.AIServiceConfig, aiContext domain.AIContext) (json.RawMessage, error) {
	worker, ok := workers[job.Type]
	if !ok {
		return nil, fmt.Errorf("unknown ai job type %q", job.Type)
	}
	client, err := s.ClientFor(cfg)
	if err != nil {
		return nil, err
	}
	p, err := parseJobParams(job.Payload)
	if err != nil {
		return nil, err
	}
	return worker(ctx, client, job, aiContext, p)
}
