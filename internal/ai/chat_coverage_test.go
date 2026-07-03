package ai

import (
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

// ---- BuildChatSystemPrompt ----

func TestBuildChatSystemPrompt_ContainsTeamName(t *testing.T) {
	ctx := testContext()
	prompt := BuildChatSystemPrompt(ctx, nil, "")
	if !strings.Contains(prompt, "Testteam") {
		t.Fatalf("chat system prompt missing team name:\n%s", prompt)
	}
}

func TestBuildChatSystemPrompt_ContainsConnectedAccounts(t *testing.T) {
	ctx := testContext()
	prompt := BuildChatSystemPrompt(ctx, nil, "")
	// Both accounts from testContext must appear.
	for _, want := range []string{"acc-1", "acc-2", "mastodon", "bluesky"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("chat system prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildChatSystemPrompt_WithViewSummary(t *testing.T) {
	ctx := testContext()
	prompt := BuildChatSystemPrompt(ctx, nil, "composer: editing post about meetup")
	if !strings.Contains(prompt, "Current view") {
		t.Fatalf("chat system prompt missing Current view section")
	}
	if !strings.Contains(prompt, "composer: editing post about meetup") {
		t.Fatalf("chat system prompt missing view summary text")
	}
}

func TestBuildChatSystemPrompt_EmptyViewSummaryOmitsSection(t *testing.T) {
	ctx := testContext()
	prompt := BuildChatSystemPrompt(ctx, nil, "")
	if strings.Contains(prompt, "Current view") {
		t.Fatalf("chat system prompt should not include Current view when summary is empty")
	}
}

func TestBuildChatSystemPrompt_WithMentionContext(t *testing.T) {
	ctx := testContext()
	sections := []string{"Campaign: WeeklyUpdate (id=camp-1)"}
	prompt := BuildChatSystemPrompt(ctx, sections, "")
	if !strings.Contains(prompt, "WeeklyUpdate") {
		t.Fatalf("chat system prompt missing mention context")
	}
}

func TestBuildChatSystemPrompt_WithTopHashtags(t *testing.T) {
	ctx := testContext()
	ctx.TopHashtags = []domain.HashtagPerformance{
		{Tag: "golang", Display: "golang", Uses: 10, AvgEngagement: 4.5},
	}
	prompt := BuildChatSystemPrompt(ctx, nil, "")
	if !strings.Contains(prompt, "golang") {
		t.Fatalf("chat system prompt missing top hashtag")
	}
}

func TestBuildChatSystemPrompt_WithCampaignFormats(t *testing.T) {
	ctx := testContext()
	ctx.CampaignFormats = []domain.CampaignFormat{
		{ID: "fmt-1", Name: "Freitagspost", IsActive: true},
	}
	prompt := BuildChatSystemPrompt(ctx, nil, "")
	if !strings.Contains(prompt, "Freitagspost") {
		t.Fatalf("chat system prompt missing campaign format name")
	}
}

func TestBuildChatSystemPrompt_SkipsBlankMentionContext(t *testing.T) {
	ctx := testContext()
	prompt1 := BuildChatSystemPrompt(ctx, nil, "")
	prompt2 := BuildChatSystemPrompt(ctx, []string{"", "  "}, "")
	if prompt1 != prompt2 {
		t.Fatal("blank mention context sections should produce the same prompt")
	}
}

// ---- ChatMessagesFromDomain ----

func TestChatMessagesFromDomain_RoleMappingUserAndAssistant(t *testing.T) {
	msgs := []domain.AIChatMessage{
		{Role: domain.AIChatMessageRoleUser, Content: "Hello"},
		{Role: domain.AIChatMessageRoleAssistant, Content: "Hi there"},
	}
	out := ChatMessagesFromDomain(msgs)
	if len(out) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out))
	}
	if out[0].Role != RoleUser {
		t.Fatalf("first message role = %q, want user", out[0].Role)
	}
	if out[1].Role != RoleAssistant {
		t.Fatalf("second message role = %q, want assistant", out[1].Role)
	}
}

func TestChatMessagesFromDomain_MentionsFoldedIntoUserText(t *testing.T) {
	msgs := []domain.AIChatMessage{
		{
			Role:    domain.AIChatMessageRoleUser,
			Content: "Tell me about this",
			Mentions: []domain.AIChatMention{
				{Type: domain.AIChatMentionTypeCampaign, ID: "camp-1", Name: "WeeklyUpdate"},
			},
		},
	}
	out := ChatMessagesFromDomain(msgs)
	if len(out) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out))
	}
	if !strings.Contains(out[0].Content, "camp-1") {
		t.Fatalf("mention id not folded into content: %q", out[0].Content)
	}
	if !strings.Contains(out[0].Content, "WeeklyUpdate") {
		t.Fatalf("mention name not folded into content: %q", out[0].Content)
	}
}

func TestChatMessagesFromDomain_AssistantMentionsNotFolded(t *testing.T) {
	// Mentions should only be folded for user messages, not assistant messages.
	msgs := []domain.AIChatMessage{
		{
			Role:    domain.AIChatMessageRoleAssistant,
			Content: "I see",
			Mentions: []domain.AIChatMention{
				{Type: domain.AIChatMentionTypeAccount, ID: "acc-1", Name: "main"},
			},
		},
	}
	out := ChatMessagesFromDomain(msgs)
	if strings.Contains(out[0].Content, "Referenced entities") {
		t.Fatal("assistant message should not have mentions folded in")
	}
	if out[0].Content != "I see" {
		t.Fatalf("assistant message content modified: %q", out[0].Content)
	}
}

func TestChatMessagesFromDomain_EmptySlice(t *testing.T) {
	out := ChatMessagesFromDomain(nil)
	if len(out) != 0 {
		t.Fatalf("expected empty slice, got %d messages", len(out))
	}
}

func TestChatMessagesFromDomain_MultipleMentionsSeparatedByComma(t *testing.T) {
	msgs := []domain.AIChatMessage{
		{
			Role:    domain.AIChatMessageRoleUser,
			Content: "Compare these",
			Mentions: []domain.AIChatMention{
				{Type: domain.AIChatMentionTypeCampaign, ID: "c1", Name: "Alpha"},
				{Type: domain.AIChatMentionTypeCampaign, ID: "c2", Name: "Beta"},
			},
		},
	}
	out := ChatMessagesFromDomain(msgs)
	if !strings.Contains(out[0].Content, "c1") || !strings.Contains(out[0].Content, "c2") {
		t.Fatalf("both mention ids missing: %q", out[0].Content)
	}
}
