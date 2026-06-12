package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

// recurringJobPayload mirrors the payload the scheduler submits for recurring
// AI enhancements (scheduler.submitRecurringAIJob).
func recurringJobPayload(templateID, accountID, fallbackContent string, scheduledAt time.Time) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"params":{
		"target_account_ids":[%q],
		"recurring_automation":{
			"template_id":%q,
			"post_kind":"main",
			"output_mode":"scheduled",
			"scheduled_at":%q,
			"template_occurrence_at":%q,
			"draft":false,
			"post_title":"Stammtisch #3",
			"fallback_content":%q,
			"template_counter":3
		}
	}}`, accountID, templateID, scheduledAt.Format(time.RFC3339), scheduledAt.Format(time.RFC3339), fallbackContent))
}

func newRecurringCallbackFixture(t *testing.T) (aiCompletionFixture, domain.SocialAccount, domain.PostTemplate) {
	t.Helper()
	ctx := context.Background()
	f := newAICompletionFixture(t)

	acc, err := f.store.CreateAccount(ctx, f.team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m.example", Username: "rec", AccessToken: "tok",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	enabled := true
	tmpl, err := f.store.CreatePostTemplate(ctx, f.team.ID, domain.AuthenticatedPrincipal{User: f.user}, domain.CreatePostTemplateInput{
		Title:            "Stammtisch",
		Content:          "Stammtisch Nr. {counter} am Montag",
		RecurrenceJSON:   `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}
	return f, acc, tmpl
}

func makeRecurringJob(t *testing.T, f aiCompletionFixture, payload json.RawMessage) domain.AIJob {
	t.Helper()
	job, err := f.store.CreateAIJob(context.Background(), domain.AIJob{
		TeamID:       f.team.ID,
		AuthorUserID: f.user.ID,
		Type:         domain.AIJobTypeVoiceEngine,
		Status:       domain.AIJobStatusPending,
		Payload:      payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestRecurringAutomationCallback(t *testing.T) {
	scheduledAt := time.Date(2027, 2, 1, 9, 0, 0, 0, time.UTC)

	t.Run("UsesAIContentOnSuccess", func(t *testing.T) {
		ctx := context.Background()
		f, acc, tmpl := newRecurringCallbackFixture(t)
		job := makeRecurringJob(t, f, recurringJobPayload(tmpl.ID, acc.ID, "Fallback Nr. {counter}", scheduledAt))

		f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted,
			json.RawMessage(`{"content":"Frischer KI-Text Nr. {counter}"}`), "")

		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(posts) != 1 {
			t.Fatalf("expected 1 post from recurring callback, got %d", len(posts))
		}
		post := posts[0]
		if post.Content != "Frischer KI-Text Nr. {counter}" {
			t.Fatalf("post content = %q, want the AI content", post.Content)
		}
		if post.PostTemplateID == nil || *post.PostTemplateID != tmpl.ID {
			t.Fatalf("post must stay linked to template, got %v", post.PostTemplateID)
		}
		if post.TemplateCounter == nil || *post.TemplateCounter != 3 {
			t.Fatalf("template counter = %v, want 3", post.TemplateCounter)
		}
		if post.Source != domain.PostSourceAutomation {
			t.Fatalf("post source = %q, want automation", post.Source)
		}
		if !post.ScheduledAt.Equal(scheduledAt) {
			t.Fatalf("scheduled at = %s, want %s", post.ScheduledAt, scheduledAt)
		}
	})

	t.Run("FallsBackToTemplateOnFailure", func(t *testing.T) {
		ctx := context.Background()
		f, acc, tmpl := newRecurringCallbackFixture(t)
		job := makeRecurringJob(t, f, recurringJobPayload(tmpl.ID, acc.ID, "Fallback Nr. {counter}", scheduledAt))

		f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusFailed, nil, "llm exploded")

		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(posts) != 1 {
			t.Fatalf("failed AI job must still create the template fallback post, got %d posts", len(posts))
		}
		if posts[0].Content != "Fallback Nr. {counter}" {
			t.Fatalf("fallback content = %q", posts[0].Content)
		}

		updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage != "llm exploded" {
			t.Fatalf("job must keep failure state for the UI, got %s %q", updated.Status, updated.ErrorMessage)
		}
	})

	t.Run("FallsBackToTemplateOnEmptyAIContent", func(t *testing.T) {
		ctx := context.Background()
		f, acc, tmpl := newRecurringCallbackFixture(t)
		job := makeRecurringJob(t, f, recurringJobPayload(tmpl.ID, acc.ID, "Fallback Nr. {counter}", scheduledAt))

		f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"  "}`), "")

		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(posts) != 1 || posts[0].Content != "Fallback Nr. {counter}" {
			t.Fatalf("empty AI content must fall back to template, got %#v", posts)
		}
	})

	t.Run("NoPostWithoutTargets", func(t *testing.T) {
		ctx := context.Background()
		f, _, tmpl := newRecurringCallbackFixture(t)
		payload := json.RawMessage(fmt.Sprintf(`{"params":{
			"recurring_automation":{"template_id":%q,"post_kind":"main","fallback_content":"x","template_counter":1}
		}}`, tmpl.ID))
		job := makeRecurringJob(t, f, payload)

		f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"text"}`), "")

		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(posts) != 0 {
			t.Fatalf("payload without target accounts must not create posts, got %d", len(posts))
		}
	})

	t.Run("UuidlessTemplateKeepsWorkingOnSqlite", func(t *testing.T) {
		// Guards the meta parsing against payloads from older versions where
		// scheduled_at/occurrence may be missing.
		ctx := context.Background()
		f, acc, tmpl := newRecurringCallbackFixture(t)
		payload := json.RawMessage(fmt.Sprintf(`{"params":{
			"target_account_ids":[%q],
			"recurring_automation":{"template_id":%q,"fallback_content":"alt","template_counter":0}
		}}`, acc.ID, tmpl.ID))
		job := makeRecurringJob(t, f, payload)

		f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusFailed, nil, "boom")

		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(posts) != 1 || posts[0].Content != "alt" {
			t.Fatalf("minimal legacy payload must still produce fallback post, got %#v", posts)
		}
		if posts[0].TemplateCounter == nil || *posts[0].TemplateCounter != 1 {
			t.Fatalf("counter must default to 1, got %v", posts[0].TemplateCounter)
		}
	})
}
