package ai

import (
	"context"
	"log/slog"
	"time"
)

// Usage reports the token counts the provider billed for one completion. A
// provider that omits usage leaves these at zero.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// CallMetrics is the audit record emitted for every LLM call (success or
// failure). It deliberately carries no prompt or completion text — only the
// metadata needed to trace cost, latency and reliability per team
// (catalog §8.3 Audit-Logging, §9.1 Metriken). The prompt content is never
// persisted here.
type CallMetrics struct {
	Team         string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	Latency      time.Duration
	JSON         bool
	ToolUse      bool
	Err          error
}

// Observer receives one CallMetrics per completed LLM call.
type Observer interface {
	ObserveCall(ctx context.Context, m CallMetrics)
}

// LogObserver writes call metrics as a structured slog record. A nil Logger
// falls back to slog.Default, so wiring one is optional.
type LogObserver struct{ Logger *slog.Logger }

func (o LogObserver) ObserveCall(_ context.Context, m CallMetrics) {
	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}
	attrs := []any{
		"team", m.Team,
		"provider", m.Provider,
		"model", m.Model,
		"input_tokens", m.InputTokens,
		"output_tokens", m.OutputTokens,
		"latency_ms", m.Latency.Milliseconds(),
		"json", m.JSON,
		"tool_use", m.ToolUse,
	}
	if m.Err != nil {
		logger.Warn("llm call failed", append(attrs, "error", m.Err.Error())...)
		return
	}
	logger.Info("llm call", attrs...)
}

// observedClient wraps a provider Client and emits one CallMetrics after each
// Complete, timing the call and reading token usage from the response. A failed
// call is still recorded (with Err set) so reliability stays observable, in line
// with the project rule that degraded paths must surface.
type observedClient struct {
	inner    Client
	obs      Observer
	provider string
	team     string
}

func (c observedClient) Model() string { return c.inner.Model() }

func (c observedClient) Complete(ctx context.Context, req Request) (Response, error) {
	start := time.Now()
	resp, err := c.inner.Complete(ctx, req)
	c.obs.ObserveCall(ctx, CallMetrics{
		Team:         c.team,
		Provider:     c.provider,
		Model:        c.inner.Model(),
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		Latency:      time.Since(start),
		JSON:         req.JSON,
		ToolUse:      len(req.Tools) > 0,
		Err:          err,
	})
	return resp, err
}
