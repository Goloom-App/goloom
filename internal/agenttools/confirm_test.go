package agenttools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestChatConfirmToolProposesInsteadOfExecuting is the confirmation guard: a
// Confirm tool routed through the chat adapter must return a confirmation
// payload and must NOT touch the store.
func TestChatConfirmToolProposesInsteadOfExecuting(t *testing.T) {
	f := newFixture(t)
	tools := ChatTools(f.deps, ChatBinding{TeamID: f.team.ID, Principal: f.principal(t, `["write"]`)})

	var schedule *toolRunner
	for i := range tools {
		if tools[i].Name == "schedule_post" {
			schedule = &toolRunner{exec: tools[i].Execute}
		}
	}
	if schedule == nil {
		t.Fatal("schedule_post must be exposed to chat")
	}

	args := json.RawMessage(`{"title":"T","content":"hi","scheduled_at":"` + soon() + `","target_accounts":["` + f.account.ID + `"]}`)
	_, payload, err := schedule.exec(context.Background(), args)
	if err != nil {
		t.Fatalf("proposing must not error: %v", err)
	}

	var wrapper struct {
		Confirmation ConfirmationRequest `json:"confirmation"`
	}
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		t.Fatalf("payload must be a confirmation request: %v", err)
	}
	if wrapper.Confirmation.Tool != "schedule_post" {
		t.Fatalf("confirmation tool = %q", wrapper.Confirmation.Tool)
	}
	// team_id must be stripped from the proposed args (the endpoint re-injects it).
	if strings.Contains(string(wrapper.Confirmation.Args), "team_id") {
		t.Fatalf("proposed args must not carry team_id: %s", wrapper.Confirmation.Args)
	}

	// Nothing may have been persisted.
	posts, _ := f.store.ListTeamPosts(context.Background(), f.team.ID)
	if len(posts) != 0 {
		t.Fatalf("a proposed schedule must not create a post, got %d", len(posts))
	}
}

// TestRunConfirmedExecutes runs the proposed action after the user's confirm.
func TestRunConfirmedExecutes(t *testing.T) {
	f := newFixture(t)
	bind := ChatBinding{TeamID: f.team.ID, Principal: f.principal(t, `["write"]`)}
	args := json.RawMessage(`{"title":"T","content":"hi","scheduled_at":"` + soon() + `","target_accounts":["` + f.account.ID + `"]}`)

	res, err := RunConfirmed(context.Background(), f.deps, bind, "schedule_post", args)
	if err != nil {
		t.Fatalf("RunConfirmed: %v", err)
	}
	if !strings.Contains(res.Summary, "post_id") {
		t.Fatalf("expected a created post in the result, got %s", res.Summary)
	}
	posts, _ := f.store.ListTeamPosts(context.Background(), f.team.ID)
	if len(posts) != 1 {
		t.Fatalf("confirmed schedule must create exactly one post, got %d", len(posts))
	}
}

// TestRunConfirmedRejectsNonConfirmTool prevents the confirm endpoint from being
// used to run autonomous (non-gated) tools.
func TestRunConfirmedRejectsNonConfirmTool(t *testing.T) {
	f := newFixture(t)
	bind := ChatBinding{TeamID: f.team.ID, Principal: f.principal(t, `["write"]`)}
	if _, err := RunConfirmed(context.Background(), f.deps, bind, "draft_post", json.RawMessage(`{}`)); err == nil {
		t.Fatal("draft_post is autonomous and must not be runnable via the confirm path")
	}
	if _, err := RunConfirmed(context.Background(), f.deps, bind, "nope", json.RawMessage(`{}`)); err == nil {
		t.Fatal("unknown tool must error")
	}
}

// TestDraftPostAutonomousReturnsPreviewPayload checks the autonomous draft path
// the chat preview card relies on: it runs immediately and returns the content.
func TestDraftPostAutonomousReturnsPreviewPayload(t *testing.T) {
	f := newFixture(t)
	tools := ChatTools(f.deps, ChatBinding{TeamID: f.team.ID, Principal: f.principal(t, `["write"]`)})
	for i := range tools {
		if tools[i].Name != "draft_post" {
			continue
		}
		args := json.RawMessage(`{"title":"Hi","content":"draft body","target_accounts":["` + f.account.ID + `"]}`)
		_, payload, err := tools[i].Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("draft_post via chat: %v", err)
		}
		var out DraftPostOutput
		if err := json.Unmarshal(payload, &out); err != nil {
			t.Fatal(err)
		}
		if out.PostID == "" || out.Content != "draft body" {
			t.Fatalf("draft payload must carry id and content, got %+v", out)
		}
		posts, _ := f.store.ListTeamPosts(context.Background(), f.team.ID)
		if len(posts) != 1 {
			t.Fatalf("draft_post must persist immediately, got %d posts", len(posts))
		}
		return
	}
	t.Fatal("draft_post not exposed to chat")
}
