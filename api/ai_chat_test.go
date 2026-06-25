package api

import (
	"context"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

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
