package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"git.f4mily.net/goloom/internal/store"
)

func TestAutomationCallbackTeamBoundary(t *testing.T) {
	for _, kind := range []string{"rss", "recurring"} {
		for _, scenario := range []string{"valid", "valid-draft", "valid-publish-now", "fallback", "empty-result", "foreign-target", "foreign-target-fallback", "unknown-target", "foreign-source", "foreign-source-fallback", "foreign-override", "missing-target", "missing-fallback", "insert-failure", "invalid-result", "over-limit-ai", "over-limit-version", "over-limit-ai-and-template", "foreign-target-over-limit-ai"} {
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
				if scenario == "unknown-target" {
					targets = []string{"unknown-account-id"}
				}
				if scenario == "missing-target" {
					targets = nil
				}
				fallback := "fallback text"
				if scenario == "over-limit-ai-and-template" {
					fallback = strings.Repeat("x", 501)
				}
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
				if strings.Contains(scenario, "over-limit-ai") {
					result, _ = json.Marshal(map[string]any{"content": strings.Repeat("x", 501)})
				}
				if scenario == "over-limit-version" {
					result, _ = json.Marshal(map[string]any{"content": "generated", "account_content_override": map[string]string{account.ID: strings.Repeat("x", 501)}})
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
				usedFallback := scenario == "empty-result" || scenario == "invalid-result" || scenario == "over-limit-ai" || scenario == "over-limit-version"
				valid := strings.HasPrefix(scenario, "valid") || scenario == "fallback" || usedFallback || scenario == "foreign-override"
				if !valid || usedFallback {
					wantMessage := "Automation failed: post could not be created"
					if usedFallback {
						wantMessage = "Automation failed: used template fallback"
					}
					if updated.ErrorMessage != wantMessage {
						t.Errorf("unsafe job error: got %q want %q", updated.ErrorMessage, wantMessage)
					}
					for _, secret := range []string{f.team.ID, other.ID, account.ID, foreign.ID, templateID, feed.ID, "unknown-account-id", "injected post insert failure"} {
						if strings.Contains(updated.ErrorMessage, secret) {
							t.Errorf("job error exposes internal detail %q: %s", secret, updated.ErrorMessage)
						}
					}
					if scenario == "insert-failure" && !strings.Contains(logs.String(), "injected post insert failure") {
						t.Errorf("driver details missing from log: %s", logs.String())
					}
				}
				if valid {
					if len(posts) != 1 {
						t.Fatalf("legitimate automation: posts=%d logs=%s", len(posts), logs.String())
					}
					if scenario == "foreign-override" {
						versions, err := f.store.ListPostVersionsForTeamPost(ctx, f.team.ID, posts[0].ID)
						if err != nil {
							t.Fatal(err)
						}
						if posts[0].Content != "generated" || len(versions) != 0 {
							t.Errorf("hallucinated override retained: %+v", posts[0])
						}
						if updated.Status != domain.AIJobStatusCompleted || !strings.Contains(logs.String(), foreign.ID) || !strings.Contains(logs.String(), "WARN") {
							t.Errorf("dropped override must log warning without failing job: status=%s logs=%s", updated.Status, logs.String())
						}
					}
					if scenario == "valid" && posts[0].Status != domain.PostStatusPending {
						t.Errorf("scheduled automation without user authority became %s", posts[0].Status)
					}
					if scenario == "valid-draft" && posts[0].Status != domain.PostStatusDraft {
						t.Errorf("draft automation became %s", posts[0].Status)
					}
					if scenario == "valid-publish-now" && (posts[0].Status != domain.PostStatusPending || time.Since(posts[0].ScheduledAt) > time.Minute) {
						t.Errorf("publish-now automation changed: %+v", posts[0])
					}
					if (scenario == "fallback" || usedFallback) && posts[0].Content != fallback {
						t.Errorf("fallback content=%q", posts[0].Content)
					}
					if scenario == "fallback" || usedFallback {
						data, _ := json.Marshal(posts[0])
						var visible map[string]any
						_ = json.Unmarshal(data, &visible)
						reason, _ := visible["ai_fallback_reason"].(string)
						if visible["ai_job_id"] != job.ID {
							t.Errorf("fallback job reference=%v want %s", visible["ai_job_id"], job.ID)
						}
						wantReason := "validation_failed"
						switch scenario {
						case "fallback":
							wantReason = "provider_failure"
						case "empty-result":
							wantReason = "empty_content"
						case "invalid-result":
							wantReason = "invalid_output"
						}
						if reason != wantReason {
							t.Errorf("fallback reason=%q want %q", reason, wantReason)
						}
						fields := []string{"WARN", "job_id=" + job.ID, "post_id=" + posts[0].ID, "reason=" + wantReason}
						if kind == "rss" {
							fields = append(fields, "feed_id="+feed.ID, "item_key=item")
						}
						for _, field := range fields {
							if !strings.Contains(logs.String(), field) {
								t.Errorf("fallback log lacks %s: %s", field, logs.String())
							}
						}
					}
					if scenario == "fallback" && (updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage != message) {
						t.Errorf("fallback lost provider failure: %+v", updated)
					}
					if usedFallback && (updated.Status != domain.AIJobStatusFailed || !strings.Contains(updated.ErrorMessage, "used template fallback") || !strings.Contains(logs.String(), "used template fallback")) {
						t.Errorf("fallback unobservable: status=%s error=%q", updated.Status, updated.ErrorMessage)
					}
					if kind == "rss" {
						after, err := f.store.GetRSSFeedConfigByID(ctx, feedTeam, feed.ID)
						if err != nil || after.CounterNext != feed.CounterNext+1 {
							t.Errorf("RSS counter: before=%d after=%d err=%v", feed.CounterNext, after.CounterNext, err)
						}
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
					var streamed struct {
						ErrorMessage string `json:"error_message"`
					}
					if err := json.Unmarshal([]byte(event.Data), &streamed); err != nil {
						t.Fatal(err)
					}
					if streamed.ErrorMessage != updated.ErrorMessage {
						t.Errorf("SSE error=%q want sanitized stored error=%q", streamed.ErrorMessage, updated.ErrorMessage)
					}
					if (!valid || scenario == "fallback" || usedFallback) && !bytes.Contains(data, []byte("failed")) {
						t.Errorf("SSE missing failure: %s", data)
					}
				case <-time.After(time.Second):
					t.Fatal("missing completion SSE")
				}
			})
		}
	}
}

type automationWriteFailureStore struct {
	store.Store
	operation string
	inserts   int
}

func (s *automationWriteFailureStore) CreateScheduledPost(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
	s.inserts++
	if s.operation == "insert" {
		return domain.ScheduledPost{}, fmt.Errorf("injected automation insert failure")
	}
	return s.Store.CreateScheduledPost(ctx, teamID, principal, input)
}

func (s *automationWriteFailureStore) UpdateRSSImportedItemPostID(ctx context.Context, teamID, feedID, itemKey, postID string) error {
	if s.operation == "link" {
		return fmt.Errorf("injected automation link failure")
	}
	return s.Store.UpdateRSSImportedItemPostID(ctx, teamID, feedID, itemKey, postID)
}

func (s *automationWriteFailureStore) IncrementRSSFeedCounter(ctx context.Context, teamID, feedID string) error {
	if s.operation == "counter" {
		return fmt.Errorf("injected automation counter failure")
	}
	return s.Store.IncrementRSSFeedCounter(ctx, teamID, feedID)
}

func TestAutomationCallbackDoesNotRetryStoreFailure(t *testing.T) {
	for _, operation := range []string{"insert", "link", "counter"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			f, account, _ := newRecurringCallbackFixture(t)
			feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "test", TargetAccountIDs: []string{account.ID}})
			if err != nil {
				t.Fatal(err)
			}
			if err := f.store.RecordRSSImportedItem(ctx, feed.ID, "item", ""); err != nil {
				t.Fatal(err)
			}
			payload, _ := json.Marshal(map[string]any{"params": map[string]any{
				"target_account_ids": []string{account.ID},
				"rss_automation":     map[string]any{"feed_id": feed.ID, "item_key": "item", "output_mode": "scheduled", "fallback_content": "template fallback"},
			}})
			job := makeRecurringJob(t, f, payload)
			s := &automationWriteFailureStore{Store: f.store, operation: operation}
			instance := api.New(nil, s, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, nil)
			for range 2 {
				instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"generated"}`), "")
			}
			posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
			if err != nil {
				t.Fatal(err)
			}
			// Post, RSS link/counter and their audit are one transaction.
			want := 0
			if s.inserts != 1 || len(posts) != want {
				t.Fatalf("store failure retried: attempts=%d posts=%d want=%d", s.inserts, len(posts), want)
			}
			if len(posts) > 0 && posts[0].Content != "generated" {
				t.Errorf("store failure used fallback: %+v", posts[0])
			}
		})
	}
}

func TestUserJobRejectsPersistedAutomationMeta(t *testing.T) {
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
				hub := sse.NewHub()
				events, unsubscribe := hub.Subscribe(f.team.ID, "")
				defer unsubscribe()
				instance := api.New(slog.New(slog.NewTextHandler(&logs, nil)), f.store, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, hub)
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
				if len(posts) != 0 {
					t.Errorf("legacy automation job created %d posts, want 0", len(posts))
				}
				after, err := f.store.GetRSSFeedConfigByID(ctx, f.team.ID, feed.ID)
				if err != nil || after.CounterNext != feed.CounterNext {
					t.Errorf("counter changed: before=%d after=%d err=%v", feed.CounterNext, after.CounterNext, err)
				}
				updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
				if err != nil {
					t.Fatal(err)
				}
				if updated.Status != domain.AIJobStatusFailed || updated.ErrorMessage != "Automation job from before upgrade; please re-run" {
					t.Errorf("legacy automation job status=%s error=%q", updated.Status, updated.ErrorMessage)
				}
				select {
				case event := <-events:
					data, err := json.Marshal(event)
					if err != nil || !bytes.Contains(data, []byte("failed")) || !bytes.Contains(data, []byte(updated.ErrorMessage)) {
						t.Errorf("SSE event lacks failure message: %s (err=%v)", data, err)
					}
				default:
					t.Error("missing SSE failure event")
				}
				if !strings.Contains(logs.String(), "WARN") || !strings.Contains(logs.String(), "job_id="+job.ID) || !strings.Contains(logs.String(), "team_id="+f.team.ID) {
					t.Errorf("missing provenance warning: %s", logs.String())
				}
			})
		}
	}
}
