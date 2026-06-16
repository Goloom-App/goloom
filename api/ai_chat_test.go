package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/ai"
	"git.f4mily.net/goloom/internal/domain"
)

func reviseComposerTool(t *testing.T) ai.ChatTool {
	t.Helper()
	tools := (&API{}).chatTools("team", domain.AuthenticatedPrincipal{}, chatTestContext())
	for _, tool := range tools {
		if tool.Tool.Name == "revise_composer_post" {
			return tool
		}
	}
	t.Fatal("revise_composer_post tool not registered")
	return ai.ChatTool{}
}

func TestReviseComposerPostScopedOverride(t *testing.T) {
	tool := reviseComposerTool(t)
	args := json.RawMessage(`{"account_content_override":{"acc-bluesky":"` + strings.Repeat("b", 280) + `"}}`)
	msg, payload, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("scoped override within limit should succeed: %v", err)
	}
	if msg == "" || payload == nil {
		t.Fatalf("expected a confirmation message and payload, got %q / %v", msg, payload)
	}
	var out struct {
		Content                string            `json:"content"`
		AccountContentOverride map[string]string `json:"account_content_override"`
	}
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if out.Content != "" {
		t.Fatalf("scoped edit must not set a default content, got %q", out.Content)
	}
	if len(out.AccountContentOverride["acc-bluesky"]) != 280 {
		t.Fatalf("expected the bluesky override to be returned, got %v", out.AccountContentOverride)
	}
}

func TestReviseComposerPostRejectsOversizedOverride(t *testing.T) {
	tool := reviseComposerTool(t)
	args := json.RawMessage(`{"account_content_override":{"acc-bluesky":"` + strings.Repeat("b", 301) + `"}}`)
	if _, _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("override exceeding the bluesky limit must be rejected")
	}
}

func TestReviseComposerPostRejectsDefaultExceedingAnAccount(t *testing.T) {
	tool := reviseComposerTool(t)
	// New default text fits mastodon (500) but exceeds bluesky (300) with no override.
	args := json.RawMessage(`{"content":"` + strings.Repeat("a", 450) + `"}`)
	if _, _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatal("default text exceeding a targeted account's limit must be rejected")
	}
}

func TestReviseComposerPostRequiresSomething(t *testing.T) {
	tool := reviseComposerTool(t)
	if _, _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("an empty revision must be rejected")
	}
}

func TestChatMentionContextDescribesAccount(t *testing.T) {
	messages := []domain.AIChatMessage{{
		Role:    domain.AIChatMessageRoleUser,
		Content: "give @team.bsky more pep",
		Mentions: []domain.AIChatMention{
			{Type: domain.AIChatMentionTypeAccount, ID: "acc-bluesky", Name: "team.bsky"},
		},
	}}
	sections := (&API{}).chatMentionContext(context.Background(), "team", chatTestContext(), messages)
	if len(sections) != 1 {
		t.Fatalf("expected one mention section, got %d: %v", len(sections), sections)
	}
	if !strings.Contains(sections[0], "acc-bluesky") || !strings.Contains(sections[0], "bluesky") {
		t.Fatalf("account section should describe the account, got: %v", sections[0])
	}
}

func chatTestContext() domain.AIContext {
	return domain.AIContext{
		Accounts: []domain.AIAccountSummary{
			{ID: "acc-mastodon", Username: "team", Provider: "mastodon", MaxChars: 500},
			{ID: "acc-bluesky", Username: "team.bsky", Provider: "bluesky", MaxChars: 300},
		},
	}
}

func TestChatAccountLimitErrorAcceptsFittingContent(t *testing.T) {
	content := strings.Repeat("a", 300)
	if err := chatAccountLimitError(chatTestContext(), []string{"acc-mastodon", "acc-bluesky"}, content, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatAccountLimitErrorRejectsOversizedContent(t *testing.T) {
	content := strings.Repeat("a", 301)
	err := chatAccountLimitError(chatTestContext(), []string{"acc-mastodon", "acc-bluesky"}, content, nil)
	if err == nil {
		t.Fatal("expected limit error for bluesky account")
	}
	if !strings.Contains(err.Error(), "acc-bluesky") || !strings.Contains(err.Error(), "300") {
		t.Fatalf("error should name the violating account and its limit, got: %v", err)
	}
	if strings.Contains(err.Error(), "acc-mastodon") {
		t.Fatalf("error should not name accounts whose limit fits, got: %v", err)
	}
}

func TestChatAccountLimitErrorUsesOverrides(t *testing.T) {
	content := strings.Repeat("a", 450)
	overrides := map[string]string{"acc-bluesky": strings.Repeat("b", 280)}
	if err := chatAccountLimitError(chatTestContext(), []string{"acc-mastodon", "acc-bluesky"}, content, overrides); err != nil {
		t.Fatalf("override within limit should pass: %v", err)
	}

	overrides["acc-bluesky"] = strings.Repeat("b", 301)
	if err := chatAccountLimitError(chatTestContext(), []string{"acc-bluesky"}, content, overrides); err == nil {
		t.Fatal("oversized override must be rejected")
	}
}

func TestChatUnknownAccountError(t *testing.T) {
	if err := chatUnknownAccountError(chatTestContext(), []string{"acc-mastodon"}); err != nil {
		t.Fatalf("known account rejected: %v", err)
	}
	err := chatUnknownAccountError(chatTestContext(), []string{"acc-mastodon", "acc-nope"})
	if err == nil || !strings.Contains(err.Error(), "acc-nope") {
		t.Fatalf("expected unknown id error naming acc-nope, got: %v", err)
	}
}
