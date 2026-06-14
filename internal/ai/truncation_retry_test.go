package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

// budgetClient mimics a reasoning model whose hidden tokens overflow a small
// budget: it truncates until the request grants at least `threshold` tokens.
type budgetClient struct {
	model     string
	threshold int
	success   Response
	budgets   []int
}

func (c *budgetClient) Model() string {
	if c.model == "" {
		return "budget-test"
	}
	return c.model
}

func (c *budgetClient) Complete(_ context.Context, req Request) (Response, error) {
	c.budgets = append(c.budgets, req.MaxTokens)
	if req.MaxTokens < c.threshold {
		return Response{}, ErrResponseTruncated
	}
	return c.success, nil
}

func voiceJob() (domain.AIJob, params) {
	job := domain.AIJob{
		Type:    domain.AIJobTypeVoiceEngine,
		Payload: json.RawMessage(`{"params":{"target_account_ids":["acc-1","acc-2"],"prompt_hint":"meetup"}}`),
	}
	p, _ := parseJobParams(job.Payload)
	return job, p
}

func voiceSuccess() Response {
	longText := strings.TrimSpace(strings.Repeat("Wir bauen heute etwas Großartiges zusammen! ", 11))
	resultJSON, _ := json.Marshal(map[string]any{
		"title":                    "Meetup",
		"content":                  longText,
		"account_content_override": map[string]string{"acc-2": "Kurzversion für Bluesky."},
		"hashtags":                 []string{"#meetup"},
	})
	return Response{Content: string(resultJSON)}
}

func TestVoiceEngineEscalatesTokenBudgetOnTruncation(t *testing.T) {
	client := &budgetClient{model: "esc-voice", threshold: defaultMaxTokens + 1, success: voiceSuccess()}
	job, p := voiceJob()
	if _, err := runVoiceEngine(context.Background(), client, job, testContext(), p); err != nil {
		t.Fatalf("expected budget escalation to recover, got: %v", err)
	}
	if len(client.budgets) < 2 {
		t.Fatalf("expected a retry after truncation, got budgets %v", client.budgets)
	}
	if client.budgets[0] != defaultMaxTokens {
		t.Fatalf("first attempt budget = %d, want %d", client.budgets[0], defaultMaxTokens)
	}
	if client.budgets[1] <= client.budgets[0] {
		t.Fatalf("budget should grow after truncation, got %v", client.budgets)
	}
}

func TestCampaignEscalatesTokenBudgetOnTruncation(t *testing.T) {
	good, _ := json.Marshal(map[string]any{"content": "Heute bauen wir etwas Schönes.", "hashtags": []string{}})
	client := &budgetClient{model: "esc-campaign", threshold: defaultMaxTokens + 1, success: Response{Content: string(good)}}

	aiContext := testContext()
	aiContext.CampaignFormats = []domain.CampaignFormat{{ID: "fmt-1", Name: "Weekly", IsActive: true}}
	p := params{"campaign_format_id": "fmt-1", "platform": "mastodon"}

	if _, err := runCampaignAutopilot(context.Background(), client, domain.AIJob{Type: domain.AIJobTypeCampaignAutopilot}, aiContext, p); err != nil {
		t.Fatalf("expected budget escalation to recover, got: %v", err)
	}
	if len(client.budgets) < 2 || client.budgets[1] <= client.budgets[0] {
		t.Fatalf("campaign should retry with a larger budget, got %v", client.budgets)
	}
}

// A single truncation must not change the starting budget; only a recurring
// pattern (truncationsBeforeRaise) nudges a later job's start up — by one gentle
// step, not by doubling.
func TestVoiceEngineRemembersBudgetPerModel(t *testing.T) {
	const model = "learner-1"
	job, p := voiceJob()
	run := func() *budgetClient {
		c := &budgetClient{model: model, threshold: defaultMaxTokens + 1, success: voiceSuccess()}
		if _, err := runVoiceEngine(context.Background(), c, job, testContext(), p); err != nil {
			t.Fatalf("run should recover: %v", err)
		}
		return c
	}
	// Every job up to and including the one that crosses the threshold still
	// starts at the default — a single fluke must not move it.
	for i := 0; i < truncationsBeforeRaise; i++ {
		if c := run(); c.budgets[0] != defaultMaxTokens {
			t.Fatalf("run %d should still start at default, got %v", i, c.budgets)
		}
	}
	// Only the *next* job sees the raised starting budget, grown by one step.
	after := run()
	if after.budgets[0] != defaultMaxTokens+tokenBudgetStep {
		t.Fatalf("learned start = %d, want a single step up to %d", after.budgets[0], defaultMaxTokens+tokenBudgetStep)
	}
}

func TestBudgetMemoryRaisesOnlyOnRepeatedTruncation(t *testing.T) {
	m := newBudgetMemory()
	if got := m.starting("a"); got != defaultMaxTokens {
		t.Fatalf("fresh model start = %d, want %d", got, defaultMaxTokens)
	}
	// One truncation is a fluke — no change.
	m.learnTruncation("a")
	if got := m.starting("a"); got != defaultMaxTokens {
		t.Fatalf("one truncation must not raise the budget, got %d", got)
	}
	// Reaching the threshold raises it by exactly one step.
	for i := 1; i < truncationsBeforeRaise; i++ {
		m.learnTruncation("a")
	}
	if got := m.starting("a"); got != defaultMaxTokens+tokenBudgetStep {
		t.Fatalf("after %d truncations start = %d, want %d", truncationsBeforeRaise, got, defaultMaxTokens+tokenBudgetStep)
	}
	// A second full run of truncations adds another step (no doubling).
	for i := 0; i < truncationsBeforeRaise; i++ {
		m.learnTruncation("a")
	}
	if got := m.starting("a"); got != defaultMaxTokens+2*tokenBudgetStep {
		t.Fatalf("second cycle start = %d, want %d", got, defaultMaxTokens+2*tokenBudgetStep)
	}
	// It never climbs past the cap.
	for i := 0; i < truncationsBeforeRaise*20; i++ {
		m.learnTruncation("a")
	}
	if got := m.starting("a"); got != maxTokenBudget {
		t.Fatalf("budget should cap at %d, got %d", maxTokenBudget, got)
	}
	// Unrelated model untouched.
	if got := m.starting("b"); got != defaultMaxTokens {
		t.Fatalf("unrelated model should be untouched, got %d", got)
	}
}
