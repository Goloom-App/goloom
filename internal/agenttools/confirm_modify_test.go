package agenttools

import (
	"context"
	"encoding/json"
	"testing"
)

// chatExec returns the chat executor of the named tool.
func chatExec(t *testing.T, f fixture, name string) func(context.Context, json.RawMessage) (string, json.RawMessage, error) {
	t.Helper()
	tools := ChatTools(f.deps, ChatBinding{TeamID: f.team.ID, Principal: f.principal(t, `["write"]`)})
	for i := range tools {
		if tools[i].Name == name {
			return tools[i].Execute
		}
	}
	t.Fatalf("%s not exposed to chat", name)
	return nil
}

func isModifyProposal(payload json.RawMessage) bool {
	var w struct {
		Confirmation ConfirmationRequest `json:"confirmation"`
	}
	return json.Unmarshal(payload, &w) == nil && w.Confirmation.Tool == "modify_post"
}

// TestChatModifyPostConfirmationGate: modify_post is only autonomous for pure
// content edits of drafts; scheduled posts and schedule/target changes are
// proposed and must not touch the store.
func TestChatModifyPostConfirmationGate(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	draft, err := coreDraftPost(bg(), f.deps, inv, DraftPostInput{
		TeamID: f.team.ID, Title: "D", Content: "orig", TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	sched, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "S", Content: "orig", ScheduledAt: soon(), TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	modify := chatExec(t, f, "modify_post")

	cases := []struct {
		name     string
		args     string
		propose  bool
		postID   string
		wantBody string
	}{
		{"draft content edit runs", `{"post_id":"` + draft.PostID + `","content":"new"}`, false, draft.PostID, "new"},
		{"draft reschedule proposed", `{"post_id":"` + draft.PostID + `","scheduled_at":"` + soon() + `"}`, true, draft.PostID, "new"},
		{"draft retarget proposed", `{"post_id":"` + draft.PostID + `","target_accounts":["` + f.account.ID + `"]}`, true, draft.PostID, "new"},
		{"scheduled content edit proposed", `{"post_id":"` + sched.PostID + `","content":"evil"}`, true, sched.PostID, "orig"},
		{"unknown post fails closed", `{"post_id":"missing","content":"x"}`, true, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, payload, err := modify(context.Background(), json.RawMessage(tc.args))
			if tc.propose {
				if err != nil || !isModifyProposal(payload) {
					t.Fatalf("expected confirmation proposal, got payload=%s err=%v", payload, err)
				}
			} else if err != nil || isModifyProposal(payload) {
				t.Fatalf("expected autonomous run, got payload=%s err=%v", payload, err)
			}
			if tc.postID == "" {
				return
			}
			p, err := f.store.GetScheduledPost(context.Background(), f.team.ID, tc.postID)
			if err != nil {
				t.Fatal(err)
			}
			if p.Content != tc.wantBody {
				t.Fatalf("content = %q, want %q", p.Content, tc.wantBody)
			}
		})
	}
}

// TestRunConfirmedModifyPost: after the user confirms, modify_post executes even
// for a scheduled post.
func TestRunConfirmedModifyPost(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	sched, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "S", Content: "orig", ScheduledAt: soon(), TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	bind := ChatBinding{TeamID: f.team.ID, Principal: f.principal(t, `["write"]`)}
	if _, err := RunConfirmed(bg(), f.deps, bind, "modify_post", json.RawMessage(`{"post_id":"`+sched.PostID+`","content":"ok"}`)); err != nil {
		t.Fatalf("RunConfirmed modify_post: %v", err)
	}
	p, _ := f.store.GetScheduledPost(bg(), f.team.ID, sched.PostID)
	if p.Content != "ok" {
		t.Fatalf("content = %q", p.Content)
	}
}
