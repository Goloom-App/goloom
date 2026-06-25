package agenttools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestReviseComposerPost_ScopedOverrideWithinLimit(t *testing.T) {
	f := newFixture(t)
	bsky := f.blueskyAccount(t) // 300-char limit
	tool := chatTool(t, f, "revise_composer_post")

	args := json.RawMessage(`{"account_content_override":{"` + bsky.ID + `":"` + strings.Repeat("b", 280) + `"}}`)
	_, payload, err := tool(context.Background(), args)
	if err != nil {
		t.Fatalf("scoped override within limit must succeed: %v", err)
	}
	var out ReviseComposerPostOutput
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatal(err)
	}
	if out.Content != "" {
		t.Fatalf("a scoped edit must not set default content, got %q", out.Content)
	}
	if len(out.AccountContentOverride[bsky.ID]) != 280 {
		t.Fatalf("expected the bluesky override returned, got %v", out.AccountContentOverride)
	}
	// Nothing persisted: revise is a proposal applied in the composer.
	posts, _ := f.store.ListTeamPosts(context.Background(), f.team.ID)
	if len(posts) != 0 {
		t.Fatalf("revise_composer_post must not persist anything, got %d posts", len(posts))
	}
}

func TestReviseComposerPost_RejectsOversizedOverride(t *testing.T) {
	f := newFixture(t)
	bsky := f.blueskyAccount(t)
	tool := chatTool(t, f, "revise_composer_post")
	args := json.RawMessage(`{"account_content_override":{"` + bsky.ID + `":"` + strings.Repeat("b", 301) + `"}}`)
	if _, _, err := tool(context.Background(), args); err == nil {
		t.Fatal("override exceeding the bluesky limit must be rejected")
	}
}

func TestReviseComposerPost_RejectsDefaultExceedingAnAccount(t *testing.T) {
	f := newFixture(t)
	f.blueskyAccount(t) // 300; the fixture's mastodon account allows 500
	tool := chatTool(t, f, "revise_composer_post")
	// Fits mastodon (500) but exceeds bluesky (300) with no override.
	args := json.RawMessage(`{"content":"` + strings.Repeat("a", 450) + `"}`)
	if _, _, err := tool(context.Background(), args); err == nil {
		t.Fatal("default text exceeding a targeted account's limit must be rejected")
	}
}

func TestReviseComposerPost_RejectsUnknownAccount(t *testing.T) {
	f := newFixture(t)
	tool := chatTool(t, f, "revise_composer_post")
	args := json.RawMessage(`{"account_content_override":{"nope":"hi"}}`)
	if _, _, err := tool(context.Background(), args); err == nil {
		t.Fatal("override for an unknown account id must be rejected")
	}
}

func TestReviseComposerPost_RequiresSomething(t *testing.T) {
	f := newFixture(t)
	tool := chatTool(t, f, "revise_composer_post")
	if _, _, err := tool(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("an empty revision must be rejected")
	}
}

// chatTool returns the Execute func of a chat-exposed tool by name.
func chatTool(t *testing.T, f fixture, name string) func(context.Context, json.RawMessage) (string, json.RawMessage, error) {
	t.Helper()
	tools := ChatTools(f.deps, ChatBinding{TeamID: f.team.ID, Principal: f.principal(t, `["write"]`)})
	for i := range tools {
		if tools[i].Name == name {
			return tools[i].Execute
		}
	}
	t.Fatalf("tool %q not exposed to chat", name)
	return nil
}
