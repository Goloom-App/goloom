package api

import (
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

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
