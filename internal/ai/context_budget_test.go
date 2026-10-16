package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func knowledgeFixture(n int, size int) []domain.KnowledgeSource {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	out := make([]domain.KnowledgeSource, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, domain.KnowledgeSource{
			Name:      fmt.Sprintf("src%02d", i),
			Content:   strings.Repeat("Ж", size),
			UpdatedAt: base.Add(time.Duration(i) * time.Hour),
		})
	}
	return out
}

func TestKnowledgeSectionRespectsTotalBudget(t *testing.T) {
	prompt := BuildSystemPrompt(domain.AIContext{KnowledgeSources: knowledgeFixture(10, knowledgeSourceChars)})
	if got := strings.Count(prompt, "Ж"); got > knowledgeTotalChars {
		t.Fatalf("knowledge content %d chars exceeds budget %d", got, knowledgeTotalChars)
	}
	if !strings.Contains(prompt, "more knowledge source") {
		t.Fatalf("expected omitted-count line:\n%s", prompt)
	}
}

func TestKnowledgeSectionNewestFirst(t *testing.T) {
	prompt := BuildSystemPrompt(domain.AIContext{KnowledgeSources: knowledgeFixture(10, knowledgeSourceChars)})
	if !strings.Contains(prompt, "[src09]") {
		t.Fatal("newest source must be included")
	}
	if strings.Contains(prompt, "[src00]") {
		t.Fatal("oldest source must be dropped when budget is exhausted")
	}
}

func TestKnowledgeSectionSmallSetUntouched(t *testing.T) {
	prompt := BuildSystemPrompt(domain.AIContext{KnowledgeSources: knowledgeFixture(2, 100)})
	if !strings.Contains(prompt, "[src00]") || !strings.Contains(prompt, "[src01]") {
		t.Fatal("all small sources must be included")
	}
	if strings.Contains(prompt, "more knowledge source") {
		t.Fatal("no omitted line expected")
	}
}

func TestStyleExamplesNewestFirstDeterministic(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var examples []domain.StyleExample
	for i := 0; i < 5; i++ { // stores return oldest first
		examples = append(examples, domain.StyleExample{Content: fmt.Sprintf("example-%d", i), CreatedAt: base.Add(time.Duration(i) * time.Hour)})
	}
	prompt := BuildSystemPrompt(domain.AIContext{StyleExamples: examples})
	for _, want := range []string{"example-4", "example-3", "example-2"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(prompt, "example-1") || strings.Contains(prompt, "example-0") {
		t.Fatal("older examples must be dropped")
	}
}

func TestChatPromptBrandProfileOnlyForMissingDetails(t *testing.T) {
	p := BuildChatSystemPrompt(domain.AIContext{}, nil, "")
	if strings.Contains(p, "Before you write post content, recall") {
		t.Fatal("chat must not unconditionally re-fetch the embedded brand profile")
	}
	if !strings.Contains(p, "get_brand_profile") || !strings.Contains(p, "Brand voice for any post content") {
		t.Fatal("chat prompt must embed the voice and keep a conditional get_brand_profile hint")
	}
}

func TestDefaultsPresentWithoutProfileOrKnowledge(t *testing.T) {
	ctx := domain.AIContext{}
	for name, prompt := range map[string]string{
		"system": BuildSystemPrompt(ctx),
		"chat":   BuildChatSystemPrompt(ctx, nil, ""),
	} {
		for _, want := range []string{defaultToneRule, defaultHookRule, defaultCTARule} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s prompt missing default %q", name, want)
			}
		}
	}
	task, err := BuildGenerationPromptFromParams(ctx, nil, "mastodon")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(task, platformDefaults["mastodon"].rule) {
		t.Error("task prompt missing platform hashtag default")
	}
}

func TestAnthropicRequestUsesCacheControl(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		io.WriteString(w, `{"model":"m","stop_reason":"end_turn","content":[{"type":"text","text":"hi"}]}`)
	}))
	defer server.Close()
	client, err := NewClient(Settings{Provider: ProviderAnthropic, Model: "m", APIKey: "k", BaseURL: server.URL}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	req := Request{System: "sys prompt", Messages: []Message{{Role: RoleUser, Content: "hi"}}, Tools: []Tool{
		{Name: "a", Description: "A", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "b", Description: "B", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}}
	if _, err := client.Complete(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	sys, ok := body["system"].([]any)
	if !ok || len(sys) != 1 {
		t.Fatalf("system must be a block array, got %#v", body["system"])
	}
	blk := sys[0].(map[string]any)
	if blk["type"] != "text" || blk["text"] != "sys prompt" {
		t.Fatalf("bad system block: %#v", blk)
	}
	if cc, _ := blk["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Fatalf("system block lacks cache_control: %#v", blk)
	}
	tools := body["tools"].([]any)
	if _, has := tools[0].(map[string]any)["cache_control"]; has {
		t.Fatal("only the last tool carries cache_control")
	}
	if cc, _ := tools[1].(map[string]any)["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Fatalf("last tool lacks cache_control: %#v", tools[1])
	}

	// No system, no tools: no cache fields at all.
	body = nil
	if _, err := client.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	if _, has := body["system"]; has {
		t.Fatal("empty system must be omitted")
	}
}
