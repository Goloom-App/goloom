package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

// When JSON output is requested, the OpenAI client must ask the API for a
// guaranteed-valid JSON object via response_format, and must not send it when
// JSON is not requested.
func TestOpenAIRequestsJSONResponseFormat(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		io.WriteString(w, `{"model":"gpt-4o","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"ok\":true}"}}]}`)
	}))
	defer server.Close()

	client, err := NewClient(Settings{Provider: ProviderOpenAI, Model: "gpt-4o", APIKey: "k", BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.Complete(context.Background(), Request{JSON: true, Messages: []Message{{Role: RoleUser, Content: "give me JSON"}}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bodies[0], `"response_format"`) || !strings.Contains(bodies[0], `"json_object"`) {
		t.Fatalf("JSON request should set response_format json_object, got: %s", bodies[0])
	}

	if _, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "plain"}}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(bodies[1], `"response_format"`) {
		t.Fatalf("non-JSON request must not set response_format, got: %s", bodies[1])
	}
}

// A response cut off at the token limit (finish_reason "length") yields a
// truncated, unparseable body. The client must surface this as a distinct,
// recognisable error instead of returning the broken content.
func TestOpenAIReportsTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"model":"gpt-4o","choices":[{"finish_reason":"length","message":{"role":"assistant","content":"{\"content\": \"half a sen"}}]}`)
	}))
	defer server.Close()

	client, _ := NewClient(Settings{Provider: ProviderOpenAI, Model: "gpt-4o", APIKey: "k", BaseURL: server.URL}, server.Client())
	_, err := client.Complete(context.Background(), Request{JSON: true, Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrResponseTruncated) {
		t.Fatalf("expected ErrResponseTruncated, got %v", err)
	}
}

// Anthropic has no json_object mode, so the client forces JSON by prefilling the
// assistant turn with "{" and must stitch that brace back onto the reply.
func TestAnthropicPrefillsJSON(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		io.WriteString(w, `{"model":"claude","stop_reason":"end_turn","content":[{"type":"text","text":"\"ok\":true}"}]}`)
	}))
	defer server.Close()

	client, _ := NewClient(Settings{Provider: ProviderAnthropic, Model: "claude", APIKey: "k", BaseURL: server.URL}, server.Client())
	resp, err := client.Complete(context.Background(), Request{JSON: true, Messages: []Message{{Role: RoleUser, Content: "give me JSON"}}})
	if err != nil {
		t.Fatal(err)
	}
	// Last message in the request must be an assistant prefill containing "{".
	if !strings.Contains(bodies[0], `"role":"assistant"`) || !strings.Contains(bodies[0], "{") {
		t.Fatalf("expected assistant prefill, got: %s", bodies[0])
	}
	if resp.Content != `{"ok":true}` {
		t.Fatalf("prefill brace not stitched back, got %q", resp.Content)
	}
}

func TestAnthropicReportsTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"model":"claude","stop_reason":"max_tokens","content":[{"type":"text","text":"\"content\": \"half"}]}`)
	}))
	defer server.Close()

	client, _ := NewClient(Settings{Provider: ProviderAnthropic, Model: "claude", APIKey: "k", BaseURL: server.URL}, server.Client())
	_, err := client.Complete(context.Background(), Request{JSON: true, Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if !errors.Is(err, ErrResponseTruncated) {
		t.Fatalf("expected ErrResponseTruncated, got %v", err)
	}
}

// The tolerant extractor must isolate a JSON object even when it is wrapped in
// prose that itself contains braces, and when the object has nested braces.
func TestExtractJSONObjectBalancedBraces(t *testing.T) {
	in := `Sure! Here is the result {"content": "use {curly} braces", "meta": {"n": 1}}. Let me know if that helps!`
	payload, err := extractJSONObject(in)
	if err != nil {
		t.Fatal(err)
	}
	if payloadString(payload, "content") != "use {curly} braces" {
		t.Fatalf("unexpected content: %v", payload["content"])
	}
	if meta := payloadObject(payload, "meta"); meta["n"] != float64(1) {
		t.Fatalf("nested object lost: %v", payload["meta"])
	}
}

// A single malformed completion must no longer fail the whole campaign job: the
// generator retries and succeeds on the next valid response.
func TestCampaignRetriesOnInvalidJSON(t *testing.T) {
	good, _ := json.Marshal(map[string]any{"content": "Heute bauen wir etwas Schönes.", "hashtags": []string{}})
	client := &scriptedClient{responses: []Response{
		{Content: "Sorry, I cannot produce JSON right now."},
		{Content: string(good)},
	}}

	aiContext := testContext()
	aiContext.CampaignFormats = []domain.CampaignFormat{{
		ID:       "fmt-1",
		Name:     "Weekly",
		IsActive: true,
	}}
	p := params{"campaign_format_id": "fmt-1", "platform": "mastodon"}

	raw, err := runCampaignAutopilot(context.Background(), client, domain.AIJob{Type: domain.AIJobTypeCampaignAutopilot}, aiContext, p)
	if err != nil {
		t.Fatalf("campaign should recover from one bad response: %v", err)
	}
	var result campaignResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Content == "" {
		t.Fatalf("expected content, got empty result")
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected a retry (2 requests), got %d", len(client.requests))
	}
	// The retried request must carry the JSON flag so the provider enforces JSON.
	if !client.requests[1].JSON {
		t.Fatalf("retry request should request JSON output")
	}
}
