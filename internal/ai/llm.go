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
	"time"
)

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"

	DefaultOpenAIModel    = "gpt-4o"
	DefaultAnthropicModel = "claude-opus-4-8"

	defaultMaxTokens = 1500
	requestTimeout   = 120 * time.Second
)

var ErrNotConfigured = errors.New("llm provider not configured")

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
}

type Response struct {
	Content   string
	ToolCalls []ToolCall
	Model     string
}

// Client is a minimal provider-neutral LLM client.
type Client interface {
	Complete(ctx context.Context, req Request) (Response, error)
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
	resp, err := client.Complete(ctx, Request{
		System:      system,
		Messages:    []Message{{Role: RoleUser, Content: prompt}},
		Temperature: &temperature,
		MaxTokens:   maxTokens,
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
