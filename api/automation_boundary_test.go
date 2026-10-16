package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/sse"
)

func TestAutomationCallbackTeamBoundary(t *testing.T) {
	for _, kind := range []string{"rss", "recurring"} {
		for _, scenario := range []string{"valid", "valid-draft", "valid-publish-now", "fallback", "empty-result", "foreign-target", "foreign-target-fallback", "foreign-source", "foreign-source-fallback", "foreign-override", "missing-target", "missing-fallback", "insert-failure", "invalid-result"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				ctx := context.Background()
				f, account, template := newRecurringCallbackFixture(t)
				other, err := f.store.CreateTeam(ctx, f.user.ID, domain.CreateTeamInput{Name: "other"})
				if err != nil {
					t.Fatal(err)
				}
				foreign := callbackAccount(t, f, other.ID)
				feedTeam := f.team.ID
				templateID := template.ID
				if strings.HasPrefix(scenario, "foreign-source") {
					feedTeam = other.ID
					foreignTemplate, err := f.store.CreatePostTemplate(ctx, other.ID, domain.AuthenticatedPrincipal{User: f.user}, domain.CreatePostTemplateInput{Title: "foreign", Content: "foreign", RecurrenceJSON: template.RecurrenceJSON, TargetAccountIDs: []string{foreign.ID}})
					if err != nil {
						t.Fatal(err)
					}
					templateID = foreignTemplate.ID
				}
				feed, err := f.store.CreateRSSFeedConfig(ctx, feedTeam, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "test", TargetAccountIDs: []string{account.ID}})
				if err != nil {
					t.Fatal(err)
				}
				if err := f.store.RecordRSSImportedItem(ctx, feed.ID, "item", ""); err != nil {
					t.Fatal(err)
				}
				targets := []string{account.ID}
				if strings.HasPrefix(scenario, "foreign-target") {
					targets = []string{foreign.ID}
				}
				if scenario == "missing-target" {
					targets = nil
				}
				fallback := "fallback text"
				if scenario == "missing-fallback" {
					fallback = ""
				}
				meta := map[string]any{"feed_id": feed.ID, "item_key": "item", "template_id": templateID, "post_kind": "main", "output_mode": "scheduled", "scheduled_at": "2027-02-01T09:00:00Z", "fallback_content": fallback, "template_counter": 3}
				if scenario == "valid-draft" {
					meta["output_mode"] = "draft"
				}
				if scenario == "valid-publish-now" {
					meta["output_mode"] = "publish_now"
				}
				payload, _ := json.Marshal(map[string]any{"params": map[string]any{"target_account_ids": targets, kind + "_automation": meta}})
				job := makeRecurringJob(t, f, payload)
				var logs bytes.Buffer
				hub := sse.NewHub()
				events, unsubscribe := hub.Subscribe(f.team.ID, "")
				defer unsubscribe()
				instance := api.New(slog.New(slog.NewTextHandler(&logs, nil)), f.store, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, hub)
				if scenario == "insert-failure" {
					instance = api.New(slog.New(slog.NewTextHandler(&logs, nil)), failingAutoPostStore{f.store}, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, hub)
				}
				status, message := domain.AIJobStatusCompleted, ""
				result := json.RawMessage(`{"content":"generated"}`)
				if strings.Contains(scenario, "fallback") || scenario == "fallback" {
					status, message, result = domain.AIJobStatusFailed, "provider failed", nil
				}
				if scenario == "empty-result" {
					result = json.RawMessage(`{"content":" "}`)
				}
				if scenario == "invalid-result" {
					result = json.RawMessage(`{"content":42}`)
				}
				if scenario == "foreign-override" {
					result, _ = json.Marshal(map[string]any{"content": "generated", "account_content_override": map[string]string{foreign.ID: "foreign override"}})
				}
				instance.CompleteAIJob(ctx, job.ID, status, result, message)
				posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
				if err != nil {
					t.Fatal(err)
				}
				updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
				if err != nil {
					t.Fatal(err)
				}
				valid := strings.HasPrefix(scenario, "valid") || scenario == "fallback" || scenario == "empty-result" || scenario == "invalid-result"
				if valid {
					if len(posts) != 1 {
						t.Fatalf("legitimate automation: posts=%d logs=%s", len(posts), logs.String())
					}
					if scenario == "valid-draft" && posts[0].Status != domain.PostStatusDraft {
						t.Errorf("draft automation became %s", posts[0].Status)
					}
					if scenario == "valid-publish-now" && (posts[0].Status != domain.PostStatusPending || time.Since(posts[0].ScheduledAt) > time.Minute) {
						t.Errorf("publish-now automation changed: %+v", posts[0])
					}
					if (scenario == "fallback" || scenario == "empty-result" || scenario == "invalid-result") && posts[0].Content != fallback {
						t.Errorf("fallback content=%q", posts[0].Content)
					}
					if scenario == "fallback" && (updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage != message) {
						t.Errorf("fallback lost provider failure: %+v", updated)
					}
					if (scenario == "empty-result" || scenario == "invalid-result") && (updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage == "") {
						t.Errorf("fallback unobservable: status=%s error=%q", updated.Status, updated.ErrorMessage)
					}
				} else {
					if len(posts) != 0 {
						t.Errorf("invalid automation created %d posts", len(posts))
					}
					if updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage == "" || updated.ErrorMessage == message {
						t.Errorf("missing callback error: status=%s error=%q", updated.Status, updated.ErrorMessage)
					}
					if !strings.Contains(logs.String(), "ERROR") {
						t.Errorf("missing error log: %s", logs.String())
					}
				}
				if !valid {
					after, err := f.store.GetRSSFeedConfigByID(ctx, feedTeam, feed.ID)
					if err != nil || after.CounterNext != feed.CounterNext {
						t.Errorf("foreign/invalid automation changed counter: before=%d after=%d err=%v", feed.CounterNext, after.CounterNext, err)
					}
				}
				select {
				case event := <-events:
					data, _ := json.Marshal(event)
					if (!valid || !strings.HasPrefix(scenario, "valid")) && !bytes.Contains(data, []byte("failed")) {
						t.Errorf("SSE missing failure: %s", data)
					}
				case <-time.After(time.Second):
					t.Fatal("missing completion SSE")
				}
			})
		}
	}
}

func TestUserJobIgnoresPersistedAutomationMeta(t *testing.T) {
	for _, kind := range []string{"rss", "recurring"} {
		for _, outcome := range []string{"completed", "failed", "recovery", "auto-publish"} {
			t.Run(kind+"/"+outcome, func(t *testing.T) {
				ctx := context.Background()
				f, account, template := newRecurringCallbackFixture(t)
				feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "test", TargetAccountIDs: []string{account.ID}})
				if err != nil {
					t.Fatal(err)
				}
				if err := f.store.RecordRSSImportedItem(ctx, feed.ID, "item", ""); err != nil {
					t.Fatal(err)
				}
				payload, _ := json.Marshal(map[string]any{"origin": "automation", "params": map[string]any{
					"origin": "automation", "target_account_ids": []string{account.ID}, kind + "_automation": map[string]any{
						"feed_id": feed.ID, "item_key": "item", "template_id": template.ID, "post_kind": "main", "output_mode": "scheduled", "scheduled_at": "2027-02-01T09:00:00Z", "fallback_content": "forged fallback", "template_counter": 3,
					},
				}})
				// No server origin: persisted before upgrade or submitted by an ordinary client.
				job, err := f.store.CreateAIJob(ctx, domain.AIJob{TeamID: f.team.ID, AuthorUserID: f.user.ID, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
				if err != nil {
					t.Fatal(err)
				}
				var logs bytes.Buffer
				instance := api.New(slog.New(slog.NewTextHandler(&logs, nil)), f.store, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, nil)
				if outcome == "auto-publish" {
					if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: true}); err != nil {
						t.Fatal(err)
					}
				}
				if outcome == "recovery" {
					manager := aijobs.NewManager(f.store, nil)
					manager.SetCompleter(instance)
					manager.SetStaleAfter(-time.Second)
					if count, err := manager.RecoverInterrupted(ctx); err != nil || count != 1 {
						t.Fatalf("recovery: count=%d err=%v", count, err)
					}
				} else if outcome == "failed" {
					instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusFailed, nil, "provider failed")
				} else {
					instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"generated"}`), "")
				}
				posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
				if err != nil {
					t.Fatal(err)
				}
				want := 0
				if outcome == "auto-publish" {
					want = 1
				}
				if len(posts) != want {
					t.Fatalf("user job created %d posts, want %d", len(posts), want)
				}
				for _, post := range posts {
					if post.Source == domain.PostSourceAutomation || post.PostTemplateID != nil || post.RSSFeedID != nil || post.Content != "generated" {
						t.Errorf("user job used automation metadata: %+v", post)
					}
				}
				after, err := f.store.GetRSSFeedConfigByID(ctx, f.team.ID, feed.ID)
				if err != nil || after.CounterNext != feed.CounterNext {
					t.Errorf("counter changed: before=%d after=%d err=%v", feed.CounterNext, after.CounterNext, err)
				}
				updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
				if err != nil {
					t.Fatal(err)
				}
				wantStatus := domain.AIJobStatusCompleted
				if outcome == "failed" || outcome == "recovery" {
					wantStatus = domain.AIJobStatusFailed
				}
				if updated.Status != wantStatus {
					t.Errorf("normal job status=%s want=%s", updated.Status, wantStatus)
				}
				if !strings.Contains(logs.String(), "WARN") || !strings.Contains(logs.String(), "ignoring automation metadata") {
					t.Errorf("missing provenance warning: %s", logs.String())
				}
			})
		}
	}
}
