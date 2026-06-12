package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func testContext() domain.AIContext {
	return domain.AIContext{
		Team: domain.Team{Name: "Testteam"},
		Profile: &domain.TeamProfile{
			StyleMetadata: domain.StyleMetadata{
				PreferredLanguage: "de",
				MaxHashtags:       2,
				BannedWords:       []string{"synergy"},
				Identity:          &domain.BrandIdentity{Persona: "A friendly maker."},
			},
		},
		Accounts: []domain.AIAccountSummary{
			{ID: "acc-1", Provider: "mastodon", Username: "main", MaxChars: 500},
			{ID: "acc-2", Provider: "bluesky", Username: "short", MaxChars: 300},
		},
	}
}

func TestBuildSystemPrompt(t *testing.T) {
	prompt := BuildSystemPrompt(testContext())
	for _, want := range []string{"Testteam", "A friendly maker.", "synergy", "Hashtag budget: up to 2"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, prompt)
		}
	}
	// Default anti-AI tells are merged in when the team has few banned words.
	if !strings.Contains(prompt, "game-changer") {
		t.Fatalf("expected core avoid words in prompt")
	}
}

func TestBuildGenerationPromptFromParams(t *testing.T) {
	rawParams := json.RawMessage(`{"prompt_hint":"announce the meetup","rss_article_title":"Big News","rss_article_link":"https://example.org/item"}`)
	prompt, err := BuildGenerationPromptFromParams(testContext(), rawParams, "mastodon")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Task", "RSS ITEM", "Big News", "Character limit: 500"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("generation prompt missing %q:\n%s", want, prompt)
		}
	}
}

type scriptedClient struct {
	responses []Response
	requests  []Request
}

func (c *scriptedClient) Complete(_ context.Context, req Request) (Response, error) {
	c.requests = append(c.requests, req)
	resp := c.responses[0]
	if len(c.responses) > 1 {
		c.responses = c.responses[1:]
	}
	return resp, nil
}

func TestRunVoiceEngine(t *testing.T) {
	longText := strings.Repeat("Wir bauen heute etwas Großartiges zusammen! ", 11) // ~484 chars
	longText = strings.TrimSpace(longText)
	resultJSON, _ := json.Marshal(map[string]any{
		"title":   "Meetup Ankündigung",
		"content": longText,
		"account_content_override": map[string]string{
			"acc-2": "Kurzversion für Bluesky mit allem Wichtigen.",
		},
		"hashtags": []string{"#meetup", "community", "#meetup"},
	})
	client := &scriptedClient{responses: []Response{{Content: string(resultJSON)}}}

	job := domain.AIJob{
		Type:    domain.AIJobTypeVoiceEngine,
		Payload: json.RawMessage(`{"params":{"target_account_ids":["acc-1","acc-2"],"prompt_hint":"meetup"}}`),
	}
	p, err := parseJobParams(job.Payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := runVoiceEngine(context.Background(), client, job, testContext(), p)
	if err != nil {
		t.Fatal(err)
	}

	var result voiceEngineResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.PrimaryAccountID != "acc-1" {
		t.Fatalf("primary account = %q", result.PrimaryAccountID)
	}
	if result.AccountContentOverride["acc-2"] == "" {
		t.Fatalf("expected override for lower-limit account")
	}
	if len(result.Hashtags) != 2 {
		t.Fatalf("hashtags = %v, want 2 deduped tags", result.Hashtags)
	}
	if len([]rune(result.Content)) > 500 {
		t.Fatalf("content exceeds primary limit: %d", len([]rune(result.Content)))
	}
}

func TestRunChatToolLoop(t *testing.T) {
	client := &scriptedClient{responses: []Response{
		{ToolCalls: []ToolCall{{ID: "call-1", Name: "create_draft", Arguments: json.RawMessage(`{"content":"Hallo Welt"}`)}}},
		{Content: "Ich habe den Entwurf erstellt."},
	}}

	executed := false
	tools := []ChatTool{{
		Tool: Tool{Name: "create_draft", Description: "create", InputSchema: json.RawMessage(`{"type":"object"}`)},
		Execute: func(_ context.Context, args json.RawMessage) (string, json.RawMessage, error) {
			executed = true
			return "Draft created with id post-1.", json.RawMessage(`{"id":"post-1"}`), nil
		},
	}}

	var events []ChatEvent
	err := RunChat(context.Background(), client, "system", []Message{{Role: RoleUser, Content: "Schreib einen Post"}}, tools, func(event ChatEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !executed {
		t.Fatal("tool was not executed")
	}

	var kinds []string
	for _, event := range events {
		kinds = append(kinds, event.Type)
	}
	want := []string{"tool_call", "tool_result", "message"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("event sequence = %v, want %v", kinds, want)
	}

	// Second request must contain the assistant tool call and the tool result.
	second := client.requests[1]
	foundToolResult := false
	for _, message := range second.Messages {
		if message.Role == RoleTool && message.ToolCallID == "call-1" {
			foundToolResult = true
		}
	}
	if !foundToolResult {
		t.Fatalf("tool result missing from follow-up request: %+v", second.Messages)
	}
}

func TestNextCampaignSlotSkipsOccupiedWeekday(t *testing.T) {
	// Fixed "now": Monday 2026-06-08 10:00 UTC.
	restore := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC) }
	defer func() { nowFunc = restore }()

	tuesday := 2
	context := testContext()
	// The next Tuesday (2026-06-09) is already taken by a scheduled post.
	context.UpcomingPosts = []domain.ScheduledPost{
		{ScheduledAt: time.Date(2026, 6, 9, 9, 0, 0, 0, time.UTC)},
	}

	slot := NextCampaignSlot(context, &domain.CampaignFormat{Weekday: &tuesday})
	if slot == nil {
		t.Fatal("expected a slot")
	}
	if got := slot.UTC().Format("2006-01-02"); got != "2026-06-16" {
		t.Fatalf("slot date = %s, want next free Tuesday 2026-06-16", got)
	}
	if slot.Weekday() != time.Tuesday {
		t.Fatalf("slot weekday = %s, want Tuesday", slot.Weekday())
	}
}

func TestExtractJSONObjectWithFences(t *testing.T) {
	payload, err := extractJSONObject("```json\n{\"content\": \"hi\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if payloadString(payload, "content") != "hi" {
		t.Fatalf("unexpected payload: %v", payload)
	}

	payload, err = extractJSONObject("Here you go: {\"summary\": \"ok\"} hope that helps")
	if err != nil {
		t.Fatal(err)
	}
	if payloadString(payload, "summary") != "ok" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}
