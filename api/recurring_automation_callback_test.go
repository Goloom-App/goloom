package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store"
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

func recurringMedia(t *testing.T, f aiCompletionFixture, teamID, name string) string {
	t.Helper()
	item, err := f.store.CreateMediaItem(context.Background(), domain.MediaItem{
		TeamID: teamID, Sha256: name, Filename: name + ".png", MimeType: "image/png", SizeBytes: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	return item.ID
}

type recurringMediaFailureStore struct{ store.Store }

func (s recurringMediaFailureStore) GetMediaItemByID(context.Context, string, string) (domain.MediaItem, error) {
	return domain.MediaItem{}, fmt.Errorf("injected media lookup failure")
}

func TestRecurringAutomationRejectsUnsafeMedia(t *testing.T) {
	for _, scenario := range []string{"foreign-media", "unknown-media", "foreign-target", "media-lookup-failure"} {
		for _, kind := range []string{domain.TemplatePostRoleMain, domain.TemplatePostRoleAnnouncement} {
			for _, outcome := range []string{"completed", "failed", "empty", "draft"} {
				t.Run(scenario+"/"+kind+"/"+outcome, func(t *testing.T) {
					ctx := context.Background()
					f, account, template := newRecurringCallbackFixture(t)
					other, err := f.store.CreateTeam(ctx, f.user.ID, domain.CreateTeamInput{Name: "other"})
					if err != nil {
						t.Fatal(err)
					}
					owned := recurringMedia(t, f, f.team.ID, "owned")
					media := []string{owned}
					switch scenario {
					case "foreign-media":
						media = append(media, recurringMedia(t, f, other.ID, "foreign"))
					case "unknown-media":
						media = append(media, "missing-media")
					case "foreign-target":
						account = callbackAccount(t, f, other.ID)
					}
					payload, err := json.Marshal(map[string]any{"params": map[string]any{
						"target_account_ids": []string{account.ID},
						"recurring_automation": map[string]any{
							"template_id": template.ID, "post_kind": kind, "draft": outcome == "draft",
							"scheduled_at": "2027-02-01T09:00:00Z", "fallback_content": "fallback",
							"visibility": domain.PostVisibilityPrivate, "media_ids": media,
						},
					}})
					if err != nil {
						t.Fatal(err)
					}
					job := makeRecurringJob(t, f, payload)
					var logs bytes.Buffer
					var callbackStore store.Store = f.store
					if scenario == "media-lookup-failure" {
						callbackStore = recurringMediaFailureStore{f.store}
					}
					instance := api.New(slog.New(slog.NewTextHandler(&logs, nil)), callbackStore, nil,
						provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})),
						config.Config{}, nil, nil, nil, nil)
					status, result := domain.AIJobStatusCompleted, json.RawMessage(`{"content":"generated"}`)
					if outcome == "failed" {
						status, result = domain.AIJobStatusFailed, nil
					} else if outcome == "empty" {
						result = json.RawMessage(`{"content":" "}`)
					}
					instance.CompleteAIJob(ctx, job.ID, status, result, "provider failed")
					for _, teamID := range []string{f.team.ID, other.ID} {
						posts, err := f.store.ListTeamPosts(ctx, teamID)
						if err != nil {
							t.Fatal(err)
						}
						if len(posts) != 0 {
							t.Errorf("unsafe automation created %d posts", len(posts))
						}
					}
					updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
					if err != nil {
						t.Fatal(err)
					}
					if updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage != "Automation failed: post could not be created" {
						t.Errorf("failure not visible to user: %s %q", updated.Status, updated.ErrorMessage)
					}
					if !strings.Contains(logs.String(), "ERROR") {
						t.Errorf("missing failure log: %s", logs.String())
					}
				})
			}
		}
	}
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
		Origin:       domain.AIJobOriginAutomation,
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

func TestRecurringAutomationPreservesPublicationSnapshot(t *testing.T) {
	for _, visibility := range []string{domain.PostVisibilityPrivate, domain.PostVisibilityUnlisted} {
		for _, kind := range []string{domain.TemplatePostRoleMain, domain.TemplatePostRoleAnnouncement} {
			for _, outcome := range []struct {
				name    string
				status  domain.AIJobStatus
				result  json.RawMessage
				content string
			}{
				{"completed", domain.AIJobStatusCompleted, json.RawMessage(`{"content":"AI text","visibility":"public","media_ids":[],"media_exclude_by_account":{}}`), "AI text"},
				{"failed", domain.AIJobStatusFailed, nil, "fallback text"},
				{"empty", domain.AIJobStatusCompleted, json.RawMessage(`{"content":" "}`), "fallback text"},
			} {
				t.Run(visibility+"/"+kind+"/"+outcome.name, func(t *testing.T) {
					ctx := context.Background()
					f, acc, tmpl := newRecurringCallbackFixture(t)
					at := time.Date(2027, 2, 1, 9, 0, 0, 0, time.UTC)
					media := []string{recurringMedia(t, f, f.team.ID, "media-1"), recurringMedia(t, f, f.team.ID, "media-2")}
					exclusions := map[string][]string{acc.ID: {media[1]}}
					// Live template is public with no media. Queued snapshot must win.
					payload, err := json.Marshal(map[string]any{"params": map[string]any{
						"target_account_ids": []string{acc.ID},
						"recurring_automation": map[string]any{
							"template_id": tmpl.ID, "post_kind": kind, "scheduled_at": at.Format(time.RFC3339),
							"fallback_content": "fallback text", "visibility": visibility,
							"media_ids": media, "media_exclude_by_account": exclusions,
						},
					}})
					if err != nil {
						t.Fatal(err)
					}
					job := makeRecurringJob(t, f, payload)
					f.api.CompleteAIJob(ctx, job.ID, outcome.status, outcome.result, "test failure")
					posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
					if err != nil {
						t.Fatal(err)
					}
					if len(posts) != 1 {
						t.Fatalf("posts = %d, want 1", len(posts))
					}
					post := posts[0]
					// Control uses the same publication fields as direct template scheduling.
					direct, err := f.store.CreateScheduledPost(ctx, f.team.ID, domain.AuthenticatedPrincipal{User: f.user}, domain.CreatePostInput{
						Title: "direct control", Content: "direct text", ScheduledAt: at,
						TargetAccounts: []string{acc.ID}, Visibility: visibility,
						MediaIDs: media, MediaExcludeByAccount: exclusions,
					})
					if err != nil {
						t.Fatal(err)
					}
					if post.Visibility != direct.Visibility {
						t.Errorf("visibility = %q, direct = %q", post.Visibility, direct.Visibility)
					}
					if !reflect.DeepEqual(post.MediaIDs, direct.MediaIDs) {
						t.Errorf("media = %v, direct = %v", post.MediaIDs, direct.MediaIDs)
					}
					if !reflect.DeepEqual(post.MediaExcludeByAccount, direct.MediaExcludeByAccount) {
						t.Errorf("exclusions = %v, direct = %v", post.MediaExcludeByAccount, direct.MediaExcludeByAccount)
					}
					if post.Content != outcome.content {
						t.Errorf("content = %q, want %q", post.Content, outcome.content)
					}
				})
			}
		}
	}
}

func TestRecurringAutomationLegacyPublicationProperties(t *testing.T) {
	for _, snapshot := range []string{"legacy", "empty", "null", "empty-visibility"} {
		for _, kind := range []string{domain.TemplatePostRoleMain, domain.TemplatePostRoleAnnouncement} {
			for _, outcome := range []string{"completed", "failed", "empty", "invalid"} {
				t.Run(snapshot+"/"+kind+"/"+outcome, func(t *testing.T) {
					ctx := context.Background()
					f, account, template := newRecurringCallbackFixture(t)
					meta := map[string]any{
						"template_id": template.ID, "post_kind": kind, "fallback_content": "fallback",
						"scheduled_at": "2027-02-01T09:00:00Z", "template_occurrence_at": "2027-02-01T09:00:00Z",
					}
					if snapshot != "legacy" {
						meta["visibility"] = domain.PostVisibilityPublic
						meta["media_ids"] = []string{}
						meta["media_exclude_by_account"] = map[string][]string{}
						if snapshot == "null" {
							meta["media_ids"], meta["media_exclude_by_account"] = nil, nil
						} else if snapshot == "empty-visibility" {
							meta["visibility"] = ""
						}
					}
					payload, err := json.Marshal(map[string]any{"params": map[string]any{
						"target_account_ids": []string{account.ID}, "recurring_automation": meta,
					}})
					if err != nil {
						t.Fatal(err)
					}
					job := makeRecurringJob(t, f, payload)
					// Legacy jobs must use edits made after queuing, from this team's template.
					visibility := domain.PostVisibilityPrivate
					media := []string{recurringMedia(t, f, f.team.ID, "first"), recurringMedia(t, f, f.team.ID, "second")}
					exclusions := map[string][]string{account.ID: {media[1]}}
					template, err = f.store.UpdatePostTemplate(ctx, f.team.ID, template.ID, domain.UpdatePostTemplateInput{
						Visibility: &visibility, MediaIDs: &media, MediaExcludeByAccount: exclusions,
					})
					if err != nil {
						t.Fatal(err)
					}
					status, result, wantContent := domain.AIJobStatusCompleted, json.RawMessage(`{"content":"generated"}`), "generated"
					switch outcome {
					case "failed":
						status, result, wantContent = domain.AIJobStatusFailed, nil, "fallback"
					case "empty":
						result, wantContent = json.RawMessage(`{"content":" "}`), "fallback"
					case "invalid":
						result, wantContent = json.RawMessage(`{"content":42}`), "fallback"
					}
					f.api.CompleteAIJob(ctx, job.ID, status, result, "provider failed")
					posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
					if err != nil {
						t.Fatal(err)
					}
					if len(posts) != 1 {
						t.Fatalf("posts=%d, want 1", len(posts))
					}
					post := posts[0]
					if snapshot == "legacy" {
						if post.Visibility != template.Visibility || !reflect.DeepEqual(post.MediaIDs, template.MediaIDs) || !reflect.DeepEqual(post.MediaExcludeByAccount, template.MediaExcludeByAccount) {
							t.Errorf("legacy publication: visibility=%q media=%v exclusions=%v; want current template %q %v %v", post.Visibility, post.MediaIDs, post.MediaExcludeByAccount, template.Visibility, template.MediaIDs, template.MediaExcludeByAccount)
						}
					} else if post.Visibility != domain.PostVisibilityPublic || len(post.MediaIDs) != 0 || len(post.MediaExcludeByAccount) != 0 {
						t.Errorf("explicit empty snapshot replaced by template: visibility=%q media=%v exclusions=%v", post.Visibility, post.MediaIDs, post.MediaExcludeByAccount)
					}
					if post.Content != wantContent {
						t.Errorf("content=%q, want %q", post.Content, wantContent)
					}
				})
			}
		}
	}
}

func TestRecurringAutomationLegacyRejectsUnsafeTemplateMedia(t *testing.T) {
	for _, scenario := range []string{"foreign", "missing"} {
		for _, status := range []domain.AIJobStatus{domain.AIJobStatusCompleted, domain.AIJobStatusFailed} {
			t.Run(scenario+"/"+string(status), func(t *testing.T) {
				ctx := context.Background()
				f, account, template := newRecurringCallbackFixture(t)
				job := makeRecurringJob(t, f, recurringJobPayload(template.ID, account.ID, "fallback", time.Now().UTC().Add(time.Hour)))
				media := []string{"missing-media"}
				if scenario == "foreign" {
					other, err := f.store.CreateTeam(ctx, f.user.ID, domain.CreateTeamInput{Name: "other"})
					if err != nil {
						t.Fatal(err)
					}
					media = []string{recurringMedia(t, f, other.ID, "foreign")}
				}
				if _, err := f.store.UpdatePostTemplate(ctx, f.team.ID, template.ID, domain.UpdatePostTemplateInput{MediaIDs: &media}); err != nil {
					t.Fatal(err)
				}
				f.api.CompleteAIJob(ctx, job.ID, status, json.RawMessage(`{"content":"generated"}`), "provider failed")
				posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
				if err != nil {
					t.Fatal(err)
				}
				if len(posts) != 0 {
					t.Errorf("legacy job created %d posts with unsafe template media", len(posts))
				}
				updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
				if err != nil {
					t.Fatal(err)
				}
				if updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage != "Automation failed: post could not be created" {
					t.Errorf("legacy media failure not visible: %s %q", updated.Status, updated.ErrorMessage)
				}
			})
		}
	}
}
