// Package ai implements the native AI engine: LLM clients (OpenAI/Anthropic),
// brand-voice prompt building, job workers (drafting, campaigns, profile
// analysis), and the chat orchestration loop with tool calling.
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"

	DefaultOpenAIModel    = "gpt-4o"
	DefaultAnthropicModel = "claude-opus-4-8"

	defaultMaxTokens = 2048
	// maxTokenBudget caps the growth of escalateBudget. Reasoning models can burn
	// the initial budget on hidden tokens and truncate, so callers retry with more
	// room — but not without bound.
	maxTokenBudget = 8192
	// truncationsBeforeRaise is how many times a model must truncate before its
	// learned *starting* budget is nudged up. One truncation is treated as a
	// fluke ("einmal ist keinmal"); only a recurring pattern adapts the default.
	truncationsBeforeRaise = 3
	// tokenBudgetStep raises the learned starting budget gently, one step at a
	// time, rather than doubling it on a pattern.
	tokenBudgetStep = 1024
	requestTimeout  = 120 * time.Second
)

var ErrNotConfigured = errors.New("llm provider not configured")

// ErrResponseTruncated indicates the model stopped because it hit the token
// limit rather than finishing its answer. The partial body is almost always
// invalid JSON, so callers should treat this as a distinct failure instead of a
// generic parse error.
var ErrResponseTruncated = errors.New("llm response truncated at token limit")

// escalateBudget doubles a token budget for a truncation retry, capped at
// maxTokenBudget.
func escalateBudget(current int) int {
	next := current * 2
	if next > maxTokenBudget {
		return maxTokenBudget
	}
	return next
}

// budgetMemory remembers, per model, the starting token budget that has been
// working. A model that *repeatedly* truncates (e.g. a reasoning model burning
// the budget on hidden tokens) gets a slightly larger starting budget so the
// system stops paying for a truncated attempt plus an escalate-retry on every
// job. A single truncation is ignored as a fluke, and the starting budget only
// creeps up one step at a time. Process-local: it re-learns after a restart.
type budgetMemory struct {
	mu     sync.Mutex
	start  map[string]int
	truncs map[string]int
}

func newBudgetMemory() *budgetMemory {
	return &budgetMemory{start: map[string]int{}, truncs: map[string]int{}}
}

// starting returns the budget to begin with for model.
func (b *budgetMemory) starting(model string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if v := b.start[model]; v > defaultMaxTokens {
		return v
	}
	return defaultMaxTokens
}

// learnTruncation records one truncation for model. Only once a model has
// truncated truncationsBeforeRaise times does its starting budget creep up by a
// single tokenBudgetStep (capped at maxTokenBudget); the counter then resets so
// the next raise again needs a fresh run of truncations.
func (b *budgetMemory) learnTruncation(model string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.truncs[model]++
	if b.truncs[model] < truncationsBeforeRaise {
		return
	}
	b.truncs[model] = 0
	current := b.start[model]
	if current < defaultMaxTokens {
		current = defaultMaxTokens
	}
	if next := current + tokenBudgetStep; next <= maxTokenBudget {
		b.start[model] = next
	} else {
		b.start[model] = maxTokenBudget
	}
}

// modelBudgets is the process-wide learned budget shared across jobs.
var modelBudgets = newBudgetMemory()

// Settings carries the per-team LLM credentials resolved from the store.
type Settings struct {
	Provider string
	Model    string
	APIKey   string
	BaseURL  string
}

func (s Settings) ResolvedModel() string {
	if m := strings.TrimSpace(s.Model); m != "" {
		return m
	}
	if strings.EqualFold(strings.TrimSpace(s.Provider), ProviderAnthropic) {
		return DefaultAnthropicModel
	}
	return DefaultOpenAIModel
}

// Message is a provider-neutral chat message.
type Message struct {
	Role       string // "user", "assistant" or "tool"
	Content    string
	ToolCalls  []ToolCall // assistant messages only
	ToolCallID string     // tool result messages only
	ToolName   string     // tool result messages only
}

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Tool describes a callable function exposed to the model.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

type Request struct {
	System      string
	Messages    []Message
	Tools       []Tool
	Temperature *float64
	MaxTokens   int
	// JSON asks the provider to emit a single valid JSON object. OpenAI enforces
	// this via response_format; Anthropic, which has no equivalent, is steered by
	// prefilling the assistant turn with an opening brace.
	JSON bool
}

type Response struct {
	Content   string
	ToolCalls []ToolCall
	Model     string
}

// Client is a minimal provider-neutral LLM client.
type Client interface {
	Complete(ctx context.Context, req Request) (Response, error)
	// Model reports the resolved model name, used to key the learned token
	// budget so a model that often truncates is started with more room.
	Model() string
}

// NewClient builds a client for the configured provider.
func NewClient(settings Settings, httpClient *http.Client) (Client, error) {
	if strings.TrimSpace(settings.APIKey) == "" {
		return nil, fmt.Errorf("%w: missing api key", ErrNotConfigured)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	switch strings.ToLower(strings.TrimSpace(settings.Provider)) {
	case ProviderAnthropic:
		return &anthropicClient{settings: settings, http: httpClient}, nil
	case ProviderOpenAI, "":
		return &openAIClient{settings: settings, http: httpClient}, nil
	default:
		return nil, fmt.Errorf("%w: unknown provider %q", ErrNotConfigured, settings.Provider)
	}
}

// Generate is a convenience wrapper for single-prompt completions.
func Generate(ctx context.Context, client Client, system, prompt string, temperature float64, maxTokens int) (string, error) {
	return generate(ctx, client, system, prompt, temperature, maxTokens, false)
}

// GenerateJSON is like Generate but asks the provider to return a single valid
// JSON object (see Request.JSON). Use it for every prompt whose reply is parsed
// as JSON.
func GenerateJSON(ctx context.Context, client Client, system, prompt string, temperature float64, maxTokens int) (string, error) {
	return generate(ctx, client, system, prompt, temperature, maxTokens, true)
}

func generate(ctx context.Context, client Client, system, prompt string, temperature float64, maxTokens int, asJSON bool) (string, error) {
	resp, err := client.Complete(ctx, Request{
		System:      system,
		Messages:    []Message{{Role: RoleUser, Content: prompt}},
		Temperature: &temperature,
		MaxTokens:   maxTokens,
		JSON:        asJSON,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// apiError carries the HTTP status and body of a failed LLM API call so callers
// can inspect it (e.g. to retry with a different parameter). Its Error string is
// unchanged from the previous inline format.
type apiError struct {
	status int
	body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("llm api error: status %d: %s", e.status, e.body)
}

func decodeBody(resp *http.Response, into any) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var buf [2048]byte
		n, _ := resp.Body.Read(buf[:])
		return &apiError{status: resp.StatusCode, body: strings.TrimSpace(string(buf[:n]))}
	}
	return json.NewDecoder(resp.Body).Decode(into)
}
