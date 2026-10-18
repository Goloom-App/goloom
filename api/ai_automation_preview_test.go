package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/ai"
	"git.f4mily.net/goloom/internal/aicontext"
	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/rss"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
	"github.com/google/uuid"
)

type previewFeedErrorStore struct{ store.Store }

func (s previewFeedErrorStore) GetRSSFeedConfigByID(context.Context, string, string) (domain.RSSFeedConfig, error) {
	return domain.RSSFeedConfig{}, errors.New("private database detail")
}

func previewHandler(t *testing.T, f aiTriggerFixture, fetch func(context.Context, string) ([]rss.Item, error)) http.Handler {
	t.Helper()
	authSvc, err := auth.New(context.Background(), config.Config{}, f.store)
	if err != nil {
		t.Fatal(err)
	}
	instance := api.New(nil, f.store, authSvc, provider.NewRegistry(), config.Config{}, nil, nil, f.manager, nil)
	instance.SetRSSPreviewFetcher(fetch)
	return instance.Handler(security.NewLimiter(10000, 10000), nil)
}

func TestRSSPreviewAuthAndStoreFailuresDoNotFetchFeed(t *testing.T) {
	ctx := context.Background()
	f := newAITriggerFixture(t, nil, auth.ScopeRead, auth.ScopeWriteDraft)
	feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{FeedURL: "https://feeds.example.test/rss", Name: "preview"})
	if err != nil {
		t.Fatal(err)
	}
	fetchCalls := 0
	fetch := func(context.Context, string) ([]rss.Item, error) {
		fetchCalls++
		return []rss.Item{{GUID: "example", Title: "Example"}}, nil
	}
	h := previewHandler(t, f, fetch)
	path := "/v1/teams/" + f.team.ID + "/rss-feeds/" + feed.ID + "/preview-items"
	unauthorized := doRequest(t, h, http.MethodGet, path, "", nil)
	if unauthorized.Code != http.StatusUnauthorized || fetchCalls != 0 {
		t.Fatalf("unauthorized preview status=%d fetch_calls=%d", unauthorized.Code, fetchCalls)
	}
	readToken, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "preview-read-only", nil, `["read"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/rss-feeds/"+feed.ID+"/ai-preview", readToken, map[string]any{"item_key": "example"})
	if forbidden.Code != http.StatusForbidden || fetchCalls != 0 {
		t.Fatalf("forbidden preview status=%d fetch_calls=%d", forbidden.Code, fetchCalls)
	}
	authSvc, err := auth.New(ctx, config.Config{}, f.store)
	if err != nil {
		t.Fatal(err)
	}
	instance := api.New(nil, previewFeedErrorStore{Store: f.store}, authSvc, provider.NewRegistry(), config.Config{}, nil, nil, f.manager, nil)
	instance.SetRSSPreviewFetcher(fetch)
	storeFailure := doRequest(t, instance.Handler(security.NewLimiter(10000, 10000), nil), http.MethodGet, path, f.bearer, nil)
	if storeFailure.Code != http.StatusInternalServerError || strings.Contains(storeFailure.Body.String(), "private database detail") || fetchCalls != 0 {
		t.Fatalf("store failure status=%d body=%s fetch_calls=%d", storeFailure.Code, storeFailure.Body.String(), fetchCalls)
	}
}

func TestPreviewCompletionFailureCancellationAndRecoveryStaySideEffectFree(t *testing.T) {
	ctx := context.Background()
	f := newAICompletionFixture(t)
	if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: true}); err != nil {
		t.Fatal(err)
	}
	feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{FeedURL: "https://feeds.example.test/rss", Name: "preview"})
	if err != nil {
		t.Fatal(err)
	}
	makePreview := func(key string, status domain.AIJobStatus) domain.AIJob {
		t.Helper()
		if err := f.store.RecordRSSImportedItem(ctx, feed.ID, key, ""); err != nil {
			t.Fatal(err)
		}
		payload := json.RawMessage(`{"params":{"no_publish":true,"rss_automation":{"feed_id":"` + feed.ID + `","item_key":"` + key + `","fallback_content":"must not create"},"recurring_automation":{"template_id":"forged"}},"preview":{"source":{}}}`)
		job, err := f.store.CreateAIJob(ctx, domain.AIJob{Origin: domain.AIJobOriginPreview, TeamID: f.team.ID, AuthorUserID: f.user.ID, Type: domain.AIJobTypeVoiceEngine, Status: status, Payload: payload})
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	assertNoPostAndPreservedMarker := func(key string) {
		t.Helper()
		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil || len(posts) != 0 {
			t.Fatalf("preview side effects created posts=%d err=%v", len(posts), err)
		}
		imported, err := f.store.RSSItemAlreadyImported(ctx, feed.ID, key)
		if err != nil || !imported {
			t.Fatalf("preview changed source marker imported=%v err=%v", imported, err)
		}
	}

	failure := makePreview("failed-item", domain.AIJobStatusPending)
	f.api.CompleteAIJob(ctx, failure.ID, domain.AIJobStatusFailed, nil, "provider failed")
	f.api.CompleteAIJob(ctx, failure.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"late result"}`), "")
	failed, err := f.store.GetAIJobByID(ctx, f.team.ID, failure.ID)
	if err != nil || failed.Status != domain.AIJobStatusFailed {
		t.Fatalf("preview failure status=%v err=%v", failed.Status, err)
	}
	assertNoPostAndPreservedMarker("failed-item")

	cancelled := makePreview("cancelled-item", domain.AIJobStatusPending)
	bearer, _, err := f.store.CreateUserAPIToken(ctx, f.user.ID, "preview-cancel", nil, `["write:draft"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	h := f.api.Handler(security.NewLimiter(10000, 10000), nil)
	response := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+cancelled.ID+"/cancel", bearer, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("cancel preview: %d %s", response.Code, response.Body.String())
	}
	assertNoPostAndPreservedMarker("cancelled-item")

	interrupted := makePreview("recovered-item", domain.AIJobStatusProcessing)
	manager := aijobs.NewManager(f.store, nil)
	manager.SetCompleter(f.api)
	manager.SetStaleAfter(-time.Second)
	if count, err := manager.RecoverInterrupted(ctx); err != nil || count != 1 {
		t.Fatalf("recover preview count=%d err=%v", count, err)
	}
	recovered, err := f.store.GetAIJobByID(ctx, f.team.ID, interrupted.ID)
	if err != nil || recovered.Status != domain.AIJobStatusFailed || recovered.ErrorMessage != aijobs.InterruptedMessage {
		t.Fatalf("recovered preview=%+v err=%v", recovered, err)
	}
	assertNoPostAndPreservedMarker("recovered-item")
	after, err := f.store.GetRSSFeedConfigByID(ctx, f.team.ID, feed.ID)
	if err != nil || after.CounterNext != feed.CounterNext {
		t.Fatalf("preview changed feed counter=%d err=%v", after.CounterNext, err)
	}
}

func previewAccount(t *testing.T, f aiTriggerFixture) domain.SocialAccount {
	t.Helper()
	account, err := f.store.CreateAccount(context.Background(), f.team.ID, domain.ConnectedAccount{
		Provider: "mastodon", Username: "preview", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://example.org", AccessToken: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func TestRSSAIPreviewRunsWithoutImportOrPostSideEffects(t *testing.T) {
	ctx := context.Background()
	accountResult := json.RawMessage(`{"title":"Generated","content":"Generated preview","hashtags":[],"platform_metadata":{},"account_content_override":{}}`)
	runner := &fakeRunner{result: accountResult}
	f := newAITriggerFixture(t, runner, auth.ScopeRead, auth.ScopeWriteDraft)
	t.Cleanup(f.manager.Wait)
	account := previewAccount(t, f)
	upsertTestLLMConfig(t, f.store, f.team.ID)
	if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: true}); err != nil {
		t.Fatal(err)
	}
	feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{
		FeedURL: "https://feeds.example.test/feed.xml", Name: "Preview feed", IsActive: true, AiEnhanceEnabled: true,
		ContentTemplate: "{title}\n{summary}\n{link}", TitleTemplate: "{title}", CounterNext: 9,
		PromptHint: "Write factual summary", TargetAccountIDs: []string{account.ID}, OutputMode: domain.AutomationOutputPublishNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	published := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	item := rss.Item{GUID: "item-161", Link: "https://news.example.test/story", Title: "Release", Content: "<p>Source details</p>", PublishedAt: published}
	h := previewHandler(t, f, func(_ context.Context, _ string) ([]rss.Item, error) { return []rss.Item{item}, nil })
	selectRec := doRequest(t, h, http.MethodGet, "/v1/teams/"+f.team.ID+"/rss-feeds/"+feed.ID+"/preview-items", f.bearer, nil)
	if selectRec.Code != http.StatusOK || !strings.Contains(selectRec.Body.String(), "item-161") {
		t.Fatalf("preview items: %d %s", selectRec.Code, selectRec.Body.String())
	}
	bad := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/rss-feeds/"+feed.ID+"/ai-preview", f.bearer, map[string]any{"item_key": "item-161", "config": map[string]any{"editorial_goal": "invent"}})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid editorial preview option: %d %s", bad.Code, bad.Body.String())
	}
	created := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/rss-feeds/"+feed.ID+"/ai-preview", f.bearer, map[string]any{"item_key": "item-161", "config": map[string]any{"content_template": "{title} — {summary}", "prompt_hint": "Unsaved hint", "title_hint": "Unsaved title", "target_account_ids": []string{account.ID}, "editorial_goal": "summary", "desired_length": "detailed"}})
	if created.Code != http.StatusAccepted {
		t.Fatalf("preview submit: %d %s", created.Code, created.Body.String())
	}
	var response struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil || response.JobID == "" {
		t.Fatalf("job response: %s, %v", created.Body.String(), err)
	}
	f.manager.Wait()
	job, err := f.store.GetAIJobByID(ctx, f.team.ID, response.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Origin != domain.AIJobOriginPreview || job.Status != domain.AIJobStatusCompleted || !strings.Contains(string(job.Result), "Generated preview") {
		t.Fatalf("preview job: origin=%q status=%q result=%s", job.Origin, job.Status, job.Result)
	}
	var payload struct {
		Params  map[string]any `json:"params"`
		Preview struct {
			Source   map[string]any `json:"source"`
			Template map[string]any `json:"template"`
			Inputs   map[string]any `json:"generation_inputs"`
		} `json:"preview"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Params["no_publish"] != true || payload.Params["rss_automation"] != nil || payload.Preview.Source["title"] != "Release" || payload.Preview.Template["content"] != "Release — Source details" {
		t.Fatalf("preview payload: %+v", payload)
	}
	posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("preview posts=%d err=%v", len(posts), err)
	}
	imported, err := f.store.RSSItemAlreadyImported(ctx, feed.ID, item.GUID)
	if err != nil || imported {
		t.Fatalf("preview marked item imported=%v err=%v", imported, err)
	}
	after, err := f.store.GetRSSFeedConfigByID(ctx, f.team.ID, feed.ID)
	if err != nil || after.CounterNext != feed.CounterNext || after.ContentTemplate != feed.ContentTemplate || after.PromptHint != feed.PromptHint {
		t.Fatalf("preview changed feed counter=%d err=%v", after.CounterNext, err)
	}
	aiCtx, err := aicontext.Build(ctx, f.store, f.team.ID, aicontext.Full())
	if err != nil {
		t.Fatal(err)
	}
	actualRaw, _ := json.Marshal(payload.Params)
	wantParams := map[string]any{"occasion": "Unsaved hint", "post_skeleton": "Release — Source details", "rss_article_title": "Release", "rss_article_content": "Source details", "rss_article_link": "https://news.example.test/story", "title_hint": "Unsaved title", "target_account_ids": []string{account.ID}, "schedule": false, "rss_editorial_goal": "summary", "rss_desired_length": "detailed"}
	wantRaw, _ := json.Marshal(wantParams)
	gotSystem, gotTask, err := ai.PrepareVoicePrompt(aiCtx, actualRaw)
	if err != nil {
		t.Fatal(err)
	}
	wantSystem, wantTask, err := ai.PrepareVoicePrompt(aiCtx, wantRaw)
	if err != nil {
		t.Fatal(err)
	}
	if gotSystem != wantSystem || gotTask != wantTask {
		t.Fatal("preview provider prompt differs from normal RSS generation inputs")
	}
}

func TestRecurringAIPreviewSnapshotsConcreteInputsAndNeverMaterializes(t *testing.T) {
	ctx := context.Background()
	runner := &fakeRunner{result: json.RawMessage(`{"title":"Generated","content":"Recurring preview","hashtags":[],"platform_metadata":{},"account_content_override":{}}`)}
	f := newAITriggerFixture(t, runner, auth.ScopeRead, auth.ScopeWriteDraft)
	t.Cleanup(f.manager.Wait)
	account := previewAccount(t, f)
	upsertTestLLMConfig(t, f.store, f.team.ID)
	if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: true}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	daysBefore := 2
	tmpl, err := f.store.CreatePostTemplate(ctx, f.team.ID, domain.AuthenticatedPrincipal{User: f.user}, domain.CreatePostTemplateInput{
		Title: "Event {counter}", Content: "Event details {counter}", RecurrenceJSON: `{"kind":"weekly","weekdays":[1],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs: []string{account.ID}, AiEnhanceEnabled: &enabled, OutputMode: domain.AutomationOutputScheduled,
		PromptHint: "Keep it direct", TitleHint: "Event title", AnnouncementEnabled: &enabled, AnnouncementTitle: "Soon {counter}", AnnouncementContent: "Reminder {counter}", AnnouncementDaysBefore: &daysBefore,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := previewHandler(t, f, nil)
	occurrence := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	draftRecurrence := `{"kind":"weekly","weekdays":[2],"hour":10,"minute":0,"timezone":"UTC"}`
	created := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/post-templates/"+tmpl.ID+"/ai-preview", f.bearer, map[string]any{"occurrence_at": occurrence.Format(time.RFC3339), "config": map[string]any{"content": "New event {counter}", "prompt_hint": "Unsaved recurring hint", "announcement_content": "New reminder {counter}", "recurrence_json": draftRecurrence, "counter_next": 42, "announcement_counter_next": 7}})
	if created.Code != http.StatusAccepted {
		t.Fatalf("preview submit: %d %s", created.Code, created.Body.String())
	}
	offRule := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/post-templates/"+tmpl.ID+"/ai-preview", f.bearer, map[string]any{"occurrence_at": occurrence.Format(time.RFC3339)})
	if offRule.Code != http.StatusBadRequest {
		t.Fatalf("persisted recurrence should reject draft occurrence without override: %d %s", offRule.Code, offRule.Body.String())
	}
	invalidCounter := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/post-templates/"+tmpl.ID+"/ai-preview", f.bearer, map[string]any{"occurrence_at": occurrence.Format(time.RFC3339), "config": map[string]any{"recurrence_json": draftRecurrence, "counter_next": 0}})
	if invalidCounter.Code != http.StatusBadRequest {
		t.Fatalf("non-positive draft counter status=%d body=%s", invalidCounter.Code, invalidCounter.Body.String())
	}
	var response struct {
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &response)
	f.manager.Wait()
	job, err := f.store.GetAIJobByID(ctx, f.team.ID, response.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Origin != domain.AIJobOriginPreview || job.Status != domain.AIJobStatusCompleted || !strings.Contains(string(job.Result), "Recurring preview") {
		t.Fatalf("preview job: %+v", job)
	}
	var payload struct {
		Params  map[string]any `json:"params"`
		Preview struct {
			Source   map[string]any `json:"source"`
			Template map[string]any `json:"template"`
		} `json:"preview"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Params["no_publish"] != true || payload.Params["recurring_automation"] != nil || payload.Params["main_event_at"] != occurrence.Format(time.RFC3339) || payload.Params["source_content"] != "New event {counter}" || payload.Params["announcement_reference_content"] != "New reminder 7" || payload.Preview.Source["occurrence_at"] != occurrence.Format(time.RFC3339) || payload.Preview.Template["title"] != "Event 42" || payload.Preview.Template["counter_next"] != float64(42) || payload.Preview.Template["announcement_counter_next"] != float64(7) || payload.Preview.Template["recurrence_json"] != draftRecurrence {
		t.Fatalf("preview payload: %+v", payload)
	}
	events, err := f.store.ListAuditEvents(ctx, domain.AuditFilter{TeamID: f.team.ID, Action: "ai_job.submit"})
	if err != nil {
		t.Fatal(err)
	}
	foundPreviewAudit := false
	for _, event := range events {
		if event.TargetID != nil && *event.TargetID == response.JobID {
			foundPreviewAudit = event.Metadata["channel"] == "preview"
		}
	}
	if !foundPreviewAudit {
		t.Fatalf("preview submission audit missing channel=preview: %+v", events)
	}
	posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("preview posts=%d err=%v", len(posts), err)
	}
	after, err := f.store.GetPostTemplate(ctx, f.team.ID, tmpl.ID)
	if err != nil || after.CounterNext != tmpl.CounterNext || after.AnnouncementCounterNext != tmpl.AnnouncementCounterNext || after.Content != tmpl.Content || after.PromptHint != tmpl.PromptHint {
		t.Fatalf("preview changed counters: %+v err=%v", after, err)
	}
	aiCtx, err := aicontext.Build(ctx, f.store, f.team.ID, aicontext.Full())
	if err != nil {
		t.Fatal(err)
	}
	actualRaw, _ := json.Marshal(payload.Params)
	wantParams := map[string]any{"source_content": "New event {counter}", "prompt_hint": "Unsaved recurring hint", "title_hint": "Event title", "target_account_ids": []string{account.ID}, "schedule": false, "recurring_post_kind": "main", "post_scheduled_at": occurrence.Format(time.RFC3339), "main_event_at": occurrence.Format(time.RFC3339), "announcement_reference_content": "New reminder 7", "announcement_reference_title": "Soon 7"}
	wantRaw, _ := json.Marshal(wantParams)
	gotSystem, gotTask, err := ai.PrepareVoicePrompt(aiCtx, actualRaw)
	if err != nil {
		t.Fatal(err)
	}
	wantSystem, wantTask, err := ai.PrepareVoicePrompt(aiCtx, wantRaw)
	if err != nil {
		t.Fatal(err)
	}
	if gotSystem != wantSystem || gotTask != wantTask {
		t.Fatalf("preview provider prompt differs from normal recurring generation inputs\ngot: %s\nwant: %s", gotTask, wantTask)
	}
}

func TestRecurringAnnouncementAIPreviewUsesDraftRuleCountersAndFailureHasNoSideEffects(t *testing.T) {
	ctx := context.Background()
	f := newAITriggerFixture(t, &fakeRunner{err: errors.New("provider unavailable")}, auth.ScopeRead, auth.ScopeWriteDraft)
	t.Cleanup(f.manager.Wait)
	account := previewAccount(t, f)
	upsertTestLLMConfig(t, f.store, f.team.ID)
	if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: true}); err != nil {
		t.Fatal(err)
	}
	enabled := true
	announcementAI := true
	daysBefore := 2
	tmpl, err := f.store.CreatePostTemplate(ctx, f.team.ID, domain.AuthenticatedPrincipal{User: f.user}, domain.CreatePostTemplateInput{
		Title: "Event {counter}", Content: "Event body", RecurrenceJSON: `{"kind":"weekly","weekdays":[1],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs: []string{account.ID}, AiEnhanceEnabled: &enabled, AiEnhanceAnnouncement: &announcementAI, OutputMode: domain.AutomationOutputScheduled,
		AnnouncementEnabled: &enabled, AnnouncementTitle: "Reminder {counter}", AnnouncementContent: "Reminder body {counter}", AnnouncementDaysBefore: &daysBefore,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := previewHandler(t, f, nil)
	occurrence := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	draftRecurrence := `{"kind":"weekly","weekdays":[2],"hour":10,"minute":0,"timezone":"UTC"}`
	created := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/post-templates/"+tmpl.ID+"/ai-preview", f.bearer, map[string]any{
		"occurrence_at": occurrence.Format(time.RFC3339), "post_kind": "announcement",
		"config": map[string]any{
			"recurrence_json": draftRecurrence, "counter_next": 12, "announcement_counter_next": 33,
			"announcement_content": "Draft reminder {counter}", "announcement_title": "Draft notice {counter}", "announcement_days_before": 3,
		},
	})
	if created.Code != http.StatusAccepted {
		t.Fatalf("announcement preview submit: %d %s", created.Code, created.Body.String())
	}
	var response struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil || response.JobID == "" {
		t.Fatalf("announcement preview response: %s, %v", created.Body.String(), err)
	}
	f.manager.Wait()
	job, err := f.store.GetAIJobByID(ctx, f.team.ID, response.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Origin != domain.AIJobOriginPreview || job.Status != domain.AIJobStatusFailed {
		t.Fatalf("failed announcement preview job: origin=%q status=%q error=%q", job.Origin, job.Status, job.ErrorMessage)
	}
	var payload struct {
		Params  map[string]any `json:"params"`
		Preview struct {
			Template map[string]any `json:"template"`
			Source   map[string]any `json:"source"`
		} `json:"preview"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Params["no_publish"] != true || payload.Params["recurring_automation"] != nil || payload.Params["recurring_post_kind"] != "announcement" || payload.Params["source_content"] != "Draft reminder 33" || payload.Params["days_before_main_event"] != float64(3) || payload.Params["main_event_at"] != occurrence.Format(time.RFC3339) || payload.Preview.Source["occurrence_at"] != occurrence.Format(time.RFC3339) || payload.Preview.Template["title"] != "Draft notice 33" || payload.Preview.Template["counter_next"] != float64(12) || payload.Preview.Template["announcement_counter_next"] != float64(33) {
		t.Fatalf("announcement preview payload: %+v", payload)
	}
	posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("failed announcement preview posts=%d err=%v", len(posts), err)
	}
	after, err := f.store.GetPostTemplate(ctx, f.team.ID, tmpl.ID)
	if err != nil || after.CounterNext != tmpl.CounterNext || after.AnnouncementCounterNext != tmpl.AnnouncementCounterNext || after.AnnouncementContent != tmpl.AnnouncementContent || after.RecurrenceJSON != tmpl.RecurrenceJSON {
		t.Fatalf("failed preview changed template: %+v err=%v", after, err)
	}
}

func TestAutomationPreviewRejectsForeignSourcesAndInvalidTargets(t *testing.T) {
	f := newAITriggerFixture(t, &fakeRunner{}, auth.ScopeRead, auth.ScopeWriteDraft)
	upsertTestLLMConfig(t, f.store, f.team.ID)
	foreign := newAITriggerFixture(t, nil)
	foreignFeed, err := foreign.store.CreateRSSFeedConfig(context.Background(), foreign.team.ID, domain.RSSFeedConfig{FeedURL: "https://feeds.example.test/rss", Name: "foreign"})
	if err != nil {
		t.Fatal(err)
	}
	h := previewHandler(t, f, nil)
	rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/rss-feeds/"+foreignFeed.ID+"/ai-preview", f.bearer, map[string]any{"item_key": "item"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign feed status=%d body=%s", rec.Code, rec.Body.String())
	}
	feed, err := f.store.CreateRSSFeedConfig(context.Background(), f.team.ID, domain.RSSFeedConfig{FeedURL: "https://feeds.example.test/rss", Name: "bad target", TargetAccountIDs: []string{uuid.NewString()}})
	if err != nil {
		t.Fatal(err)
	}
	h = previewHandler(t, f, func(context.Context, string) ([]rss.Item, error) { return []rss.Item{{GUID: "item"}}, nil })
	rec = doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/rss-feeds/"+feed.ID+"/ai-preview", f.bearer, map[string]any{"item_key": "item"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid target status=%d body=%s", rec.Code, rec.Body.String())
	}
}
