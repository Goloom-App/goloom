package agenttools

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestChatPostMutationsStayWithinActiveTeam(t *testing.T) {
	for _, tool := range []string{"modify_post", "delete_post"} {
		t.Run(tool, func(t *testing.T) {
			f := newFixture(t)
			foreign := f.foreignAccount(t) // Same user owns both teams.
			inv := f.inv(t, `["write","delete"]`)
			post, err := coreDraftPost(bg(), f.deps, inv, DraftPostInput{
				TeamID: foreign.TeamID, Title: "private title", Content: "private body",
				TargetAccounts: []string{foreign.ID},
			})
			if err != nil {
				t.Fatal(err)
			}
			bind := ChatBinding{TeamID: f.team.ID, Principal: inv.Principal}
			// A model-supplied team_id must never override the active chat team.
			args, _ := json.Marshal(map[string]string{"post_id": post.PostID, "team_id": foreign.TeamID, "content": "changed"})
			for _, chatTool := range ChatTools(f.deps, bind) {
				if chatTool.Name != tool {
					continue
				}
				_, payload, err := chatTool.Execute(bg(), args)
				if err != nil {
					t.Fatal(err)
				}
				var proposal struct {
					Confirmation ConfirmationRequest `json:"confirmation"`
				}
				if err := json.Unmarshal(payload, &proposal); err != nil || proposal.Confirmation.Tool != tool {
					t.Errorf("foreign post must fail closed with confirmation, got %s, %v", payload, err)
				}
				if strings.Contains(string(payload), "private") {
					t.Errorf("foreign post content leaked: %s", payload)
				}
			}
			if tool == "modify_post" {
				boundArgs := injectField(args, "team_id", f.team.ID)
				needs, reason := confirmModifyPost(bg(), f.deps, boundArgs)
				missingArgs := json.RawMessage(`{"team_id":"` + f.team.ID + `","post_id":"missing","content":"changed"}`)
				missingNeeds, missingReason := confirmModifyPost(bg(), f.deps, missingArgs)
				if !needs || needs != missingNeeds || reason != missingReason {
					t.Errorf("foreign post confirmation must match missing post: (%v, %q) != (%v, %q)", needs, reason, missingNeeds, missingReason)
				}
			}
			_, missingErr := RunConfirmed(bg(), f.deps, bind, tool, json.RawMessage(`{"post_id":"missing","content":"changed"}`))
			if missingErr == nil {
				t.Fatal("missing post must return not found")
			}
			result, err := RunConfirmed(bg(), f.deps, bind, tool, args)
			if !errors.Is(err, missingErr) {
				t.Errorf("confirmed foreign mutation: want not found, got %v, payload=%s", err, result.Payload)
			}
			unchanged, err := f.store.GetScheduledPost(bg(), foreign.TeamID, post.PostID)
			if err != nil || unchanged.Content != "private body" || unchanged.Title != "private title" {
				t.Errorf("foreign post changed: %+v, err=%v", unchanged, err)
			}
		})
	}
}

func TestChatConfirmedPostMutationsWithinActiveTeam(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write","delete"]`)
	post, err := coreDraftPost(bg(), f.deps, inv, DraftPostInput{
		TeamID: f.team.ID, Title: "draft", Content: "original", TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	bind := ChatBinding{TeamID: f.team.ID, Principal: inv.Principal}
	args := json.RawMessage(`{"post_id":"` + post.PostID + `","content":"changed"}`)
	if _, err := RunConfirmed(bg(), f.deps, bind, "modify_post", args); err != nil {
		t.Fatal(err)
	}
	updated, err := f.store.GetScheduledPost(bg(), f.team.ID, post.PostID)
	if err != nil || updated.Content != "changed" {
		t.Fatalf("same-team modify failed: %+v, %v", updated, err)
	}
	if _, err := RunConfirmed(bg(), f.deps, bind, "delete_post", args); err != nil {
		t.Fatal(err)
	}
	posts, err := f.store.ListTeamPosts(bg(), f.team.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("same-team delete failed: posts=%v, err=%v", posts, err)
	}
}

func TestFocusedForeignPostDoesNotGrantTeamAccess(t *testing.T) {
	f := newFixture(t)
	foreign := f.foreignAccount(t) // User owns both teams; active chat team still bounds access.
	inv := f.inv(t, `["read","write","delete"]`)
	post, err := coreDraftPost(bg(), f.deps, inv, DraftPostInput{
		TeamID: foreign.TeamID, Title: "private title", Content: "private body", TargetAccounts: []string{foreign.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, _ := json.Marshal(map[string]any{"section": "contentCalendar", "focus": map[string]string{"type": "post", "id": post.PostID}})
	bind := ChatBinding{TeamID: f.team.ID, Principal: inv.Principal, ViewContext: view}
	args, _ := json.Marshal(map[string]string{"post_id": post.PostID, "team_id": foreign.TeamID, "query": post.PostID, "content": "changed"})
	seen := map[string]bool{}
	for _, tool := range ChatTools(f.deps, bind) {
		if tool.Name != "get_current_view" && tool.Name != "get_posts" && tool.Name != "search_posts" {
			continue
		}
		seen[tool.Name] = true
		summary, payload, err := tool.Execute(bg(), args)
		if err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		if strings.Contains(summary+string(payload), "private") {
			t.Fatalf("%s leaked foreign content: %s %s", tool.Name, summary, payload)
		}
		if tool.Name == "get_current_view" && !strings.Contains(summary+string(payload), post.PostID) {
			t.Fatal("focused identity missing from view")
		}
		if tool.Name != "get_current_view" && strings.Contains(summary+string(payload), post.PostID) {
			t.Fatalf("%s read foreign focused post", tool.Name)
		}
	}
	for _, name := range []string{"get_current_view", "get_posts", "search_posts"} {
		if !seen[name] {
			t.Fatalf("missing tool %s", name)
		}
	}
	for _, name := range []string{"modify_post", "delete_post"} {
		_, missingErr := RunConfirmed(bg(), f.deps, bind, name, json.RawMessage(`{"post_id":"missing","content":"changed"}`))
		_, err := RunConfirmed(bg(), f.deps, bind, name, args)
		if missingErr == nil || !errors.Is(err, missingErr) {
			t.Fatalf("%s focused foreign post must match missing post: %v != %v", name, err, missingErr)
		}
	}
	unchanged, err := f.store.GetScheduledPost(bg(), foreign.TeamID, post.PostID)
	if err != nil || unchanged.Content != "private body" {
		t.Fatalf("foreign post changed: %+v, %v", unchanged, err)
	}
}
