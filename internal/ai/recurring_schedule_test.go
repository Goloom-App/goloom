package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

// The posting/publication time is a scheduling-only field; it must never be
// rendered into the generation prompt as content, because the model mistakes it
// for the event time and writes it into the post. (testContext is German.)
func TestRecurringMainPromptOmitsPostingTime(t *testing.T) {
	raw := json.RawMessage(`{"recurring_post_kind":"main","source_content":"Stammtisch heute Abend im Vereinsheim.","post_scheduled_at":"2026-06-19T17:00:00Z"}`)
	prompt, err := BuildGenerationPromptFromParams(testContext(), raw, "mastodon")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"Veröffentlichung dieses Posts", "This post publishes", "17:00"} {
		if strings.Contains(prompt, bad) {
			t.Fatalf("prompt must not inject the posting time (found %q):\n%s", bad, prompt)
		}
	}
	if !strings.Contains(prompt, "automatisch gesetzt") {
		t.Fatalf("prompt should instruct the model not to mention the posting time:\n%s", prompt)
	}
}

// For announcements the *event* date stays (it is real, referenceable content),
// but the separate posting time must not leak in.
func TestRecurringAnnouncementKeepsEventDateNotPostingTime(t *testing.T) {
	raw := json.RawMessage(`{"recurring_post_kind":"announcement","source_content":"Save the date!","post_scheduled_at":"2026-06-17T09:00:00Z","main_event_at":"2026-06-19T17:00:00Z"}`)
	prompt, err := BuildGenerationPromptFromParams(testContext(), raw, "mastodon")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "09:00") {
		t.Fatalf("announcement must not inject the posting time (09:00):\n%s", prompt)
	}
	if !strings.Contains(prompt, "Event-Datum") {
		t.Fatalf("announcement should still reference the event date:\n%s", prompt)
	}
}
