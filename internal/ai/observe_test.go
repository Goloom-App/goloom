package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type recordingObserver struct{ calls []CallMetrics }

func (r *recordingObserver) ObserveCall(_ context.Context, m CallMetrics) {
	r.calls = append(r.calls, m)
}

// A successful OpenAI call must surface provider, model, token usage and a
// non-failing metric to the observer.
func TestObserverRecordsOpenAIUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"model":"gpt-4o","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`)
	}))
	defer server.Close()

	obs := &recordingObserver{}
	client, err := NewClientWithObserver(
		Settings{Provider: ProviderOpenAI, Model: "gpt-4o", APIKey: "k", BaseURL: server.URL, Team: "team-1"},
		server.Client(), obs)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.InputTokens != 11 || resp.Usage.OutputTokens != 7 {
		t.Fatalf("response usage not parsed: %+v", resp.Usage)
	}
	if len(obs.calls) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(obs.calls))
	}
	m := obs.calls[0]
	if m.Team != "team-1" || m.Provider != ProviderOpenAI {
		t.Fatalf("unexpected labels: %+v", m)
	}
	if m.InputTokens != 11 || m.OutputTokens != 7 {
		t.Fatalf("metric tokens wrong: %+v", m)
	}
	if m.Err != nil {
		t.Fatalf("metric should not carry error: %v", m.Err)
	}
}

// Anthropic usage uses input_tokens/output_tokens field names.
func TestObserverRecordsAnthropicUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"model":"claude","stop_reason":"end_turn","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":20,"output_tokens":5}}`)
	}))
	defer server.Close()

	obs := &recordingObserver{}
	client, _ := NewClientWithObserver(
		Settings{Provider: ProviderAnthropic, Model: "claude", APIKey: "k", BaseURL: server.URL},
		server.Client(), obs)
	resp, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.InputTokens != 20 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("anthropic usage not parsed: %+v", resp.Usage)
	}
	if len(obs.calls) != 1 || obs.calls[0].Provider != ProviderAnthropic {
		t.Fatalf("anthropic metric missing/wrong: %+v", obs.calls)
	}
}

// A failed call is still recorded so reliability stays observable.
func TestObserverRecordsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `boom`)
	}))
	defer server.Close()

	obs := &recordingObserver{}
	client, _ := NewClientWithObserver(
		Settings{Provider: ProviderOpenAI, Model: "gpt-4o", APIKey: "k", BaseURL: server.URL},
		server.Client(), obs)
	if _, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err == nil {
		t.Fatal("expected error from 500")
	}
	if len(obs.calls) != 1 || obs.calls[0].Err == nil {
		t.Fatalf("failure metric not recorded: %+v", obs.calls)
	}
}

// A nil observer disables wrapping; raw client behaviour is preserved.
func TestNilObserverReturnsBareClient(t *testing.T) {
	client, err := NewClientWithObserver(
		Settings{Provider: ProviderOpenAI, APIKey: "k"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := client.(observedClient); ok {
		t.Fatal("nil observer must not wrap the client")
	}
}

func TestResolvedProviderDefaultsOpenAI(t *testing.T) {
	if got := (Settings{}).ResolvedProvider(); got != ProviderOpenAI {
		t.Fatalf("empty provider should resolve to openai, got %q", got)
	}
	if got := (Settings{Provider: "Anthropic"}).ResolvedProvider(); got != ProviderAnthropic {
		t.Fatalf("anthropic should resolve case-insensitively, got %q", got)
	}
}
