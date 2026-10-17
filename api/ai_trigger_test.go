package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

type aiTriggerFixture struct {
	store   *sqlitestore.Store
	h       http.Handler
	bearer  string
	team    domain.Team
	user    domain.User
	manager *aijobs.Manager
}

// fakeRunner executes AI jobs without an LLM provider.
type fakeRunner struct {
	result json.RawMessage
	err    error
	ran    chan domain.AIJob
}

func (r *fakeRunner) RunJob(_ context.Context, job domain.AIJob, _ domain.AIServiceConfig, _ domain.AIContext) (json.RawMessage, error) {
	if r.ran != nil {
		select {
		case r.ran <- job:
		default:
		}
	}
	return r.result, r.err
}

func newAITriggerFixture(t *testing.T, runner aijobs.Runner, scopes ...string) aiTriggerFixture {
	t.Helper()
	ctx := context.Background()
	s := newAICRUDStore(t)
	var jobManager *aijobs.Manager
	if runner != nil {
		jobManager = aijobs.NewManager(s, runner)
	}
	h := newAICRUDHandlerWithManager(t, s, jobManager)

	u, err := s.UpsertOIDCUser(ctx, "ai-trigger-"+uuid.NewString(), "trigger@example.test", "AI Trigger")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "trigger-team-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	rawScopes, err := json.Marshal(scopes)
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "trigger-token", nil, string(rawScopes), nil, "")
	if err != nil {
		t.Fatal(err)
	}

	return aiTriggerFixture{store: s, h: h, bearer: bearer, team: team, user: u, manager: jobManager}
}

func upsertTestLLMConfig(t *testing.T, s *sqlitestore.Store, teamID string) {
	t.Helper()
	if _, err := s.UpsertAIServiceConfig(context.Background(), teamID, domain.AIServiceConfig{
		Provider:    "openai",
		Model:       "gpt-test",
		APIKey:      "test-api-key",
		Description: "test",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAITrigger(t *testing.T) {
	t.Run("Creates202WithJobID", func(t *testing.T) {
		ctx := context.Background()
		runner := &fakeRunner{result: json.RawMessage(`{"content":"hello"}`), ran: make(chan domain.AIJob, 1)}
		fixture := newAITriggerFixture(t, runner, auth.ScopeWriteDraft)
		upsertTestLLMConfig(t, fixture.store, fixture.team.ID)

		rec := doRequest(t, fixture.h, http.MethodPost, "/v1/teams/"+fixture.team.ID+"/ai-trigger", fixture.bearer, map[string]any{
			"type":   domain.AIJobTypeVoiceEngine,
			"origin": "automation",
			"params": map[string]any{"prompt_hint": "ship it", "origin": "automation"},
		})
		if rec.Code != http.StatusAccepted {
			t.Fatalf("POST ai-trigger: got %d; body: %s", rec.Code, rec.Body.String())
		}

		var got struct {
			JobID  string             `json:"job_id"`
			Status domain.AIJobStatus `json:"status"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.JobID == "" {
			t.Fatalf("job_id is empty")
		}
		if got.Status != domain.AIJobStatusPending {
			t.Fatalf("status = %q, want %q", got.Status, domain.AIJobStatusPending)
		}

		select {
		case <-runner.ran:
		case <-time.After(time.Second):
			t.Fatal("expected in-process AI job execution")
		}
		fixture.manager.Wait()

		job, err := fixture.store.GetAIJobByID(ctx, fixture.team.ID, got.JobID)
		if err != nil {
			t.Fatalf("GetAIJobByID: %v", err)
		}
		if job.Origin != domain.AIJobOriginUser {
			t.Fatalf("API origin = %q, want user", job.Origin)
		}
		if job.AuthorUserID != fixture.user.ID {
			t.Fatalf("author_user_id = %q, want %q", job.AuthorUserID, fixture.user.ID)
		}
		if job.Type != domain.AIJobTypeVoiceEngine {
			t.Fatalf("type = %q, want %q", job.Type, domain.AIJobTypeVoiceEngine)
		}
		if job.Status != domain.AIJobStatusCompleted {
			t.Fatalf("stored status = %q, want %q", job.Status, domain.AIJobStatusCompleted)
		}
	})

	t.Run("Returns422WhenNoConfig", func(t *testing.T) {
		ctx := context.Background()
		fixture := newAITriggerFixture(t, &fakeRunner{}, auth.ScopeWriteDraft)

		rec := doRequest(t, fixture.h, http.MethodPost, "/v1/teams/"+fixture.team.ID+"/ai-trigger", fixture.bearer, map[string]any{
			"type":   domain.AIJobTypeCampaignAutopilot,
			"params": map[string]any{"campaign_format_id": "fmt-1"},
		})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("POST ai-trigger without config: got %d; body: %s", rec.Code, rec.Body.String())
		}

		jobs, err := fixture.store.ListAIJobs(ctx, fixture.team.ID, 20)
		if err != nil {
			t.Fatalf("ListAIJobs: %v", err)
		}
		if len(jobs) != 0 {
			t.Fatalf("expected 0 jobs, got %d", len(jobs))
		}
	})

	t.Run("ListAIJobs", func(t *testing.T) {
		ctx := context.Background()
		fixture := newAITriggerFixture(t, nil)

		first, err := fixture.store.CreateAIJob(ctx, domain.AIJob{
			TeamID:       fixture.team.ID,
			AuthorUserID: fixture.user.ID,
			Type:         domain.AIJobTypeVoiceEngine,
			Status:       domain.AIJobStatusPending,
			Payload:      json.RawMessage(`{"params":{"n":1}}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		second, err := fixture.store.CreateAIJob(ctx, domain.AIJob{
			TeamID:       fixture.team.ID,
			AuthorUserID: fixture.user.ID,
			Type:         domain.AIJobTypeProactiveTrigger,
			Status:       domain.AIJobStatusPending,
			Payload:      json.RawMessage(`{"params":{"n":2}}`),
		})
		if err != nil {
			t.Fatal(err)
		}

		otherUser, err := fixture.store.UpsertOIDCUser(ctx, "ai-trigger-other-"+uuid.NewString(), "other@example.test", "Other")
		if err != nil {
			t.Fatal(err)
		}
		otherTeam, err := fixture.store.CreateTeam(ctx, otherUser.ID, domain.CreateTeamInput{Name: "other-team-" + uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.store.CreateAIJob(ctx, domain.AIJob{
			TeamID:       otherTeam.ID,
			AuthorUserID: otherUser.ID,
			Type:         domain.AIJobTypeCampaignAutopilot,
			Status:       domain.AIJobStatusPending,
			Payload:      json.RawMessage(`{"params":{"n":3}}`),
		}); err != nil {
			t.Fatal(err)
		}

		rec := doRequest(t, fixture.h, http.MethodGet, "/v1/teams/"+fixture.team.ID+"/ai-jobs", fixture.bearer, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET ai-jobs: got %d; body: %s", rec.Code, rec.Body.String())
		}

		var got struct {
			Items []domain.AIJob `json:"items"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(got.Items) != 2 {
			t.Fatalf("items = %d, want 2", len(got.Items))
		}
		if got.Items[0].ID != second.ID {
			t.Fatalf("first item = %q, want %q", got.Items[0].ID, second.ID)
		}
		if got.Items[1].ID != first.ID {
			t.Fatalf("second item = %q, want %q", got.Items[1].ID, first.ID)
		}
	})

	t.Run("GetAIJob", func(t *testing.T) {
		ctx := context.Background()
		fixture := newAITriggerFixture(t, nil)

		job, err := fixture.store.CreateAIJob(ctx, domain.AIJob{
			TeamID:       fixture.team.ID,
			AuthorUserID: fixture.user.ID,
			Type:         domain.AIJobTypeVoiceEngine,
			Status:       domain.AIJobStatusCompleted,
			Payload:      json.RawMessage(`{"params":{"kind":"demo"}}`),
			Result:       json.RawMessage(`{"draft":"hello"}`),
		})
		if err != nil {
			t.Fatal(err)
		}

		rec := doRequest(t, fixture.h, http.MethodGet, "/v1/teams/"+fixture.team.ID+"/ai-jobs/"+job.ID, fixture.bearer, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET ai-job: got %d; body: %s", rec.Code, rec.Body.String())
		}

		var got domain.AIJob
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.ID != job.ID {
			t.Fatalf("id = %q, want %q", got.ID, job.ID)
		}
	})

	t.Run("MarksJobFailedWhenProviderErrors", func(t *testing.T) {
		ctx := context.Background()
		runner := &fakeRunner{err: errors.New("llm api error: status 401")}
		fixture := newAITriggerFixture(t, runner, auth.ScopeWriteDraft)
		upsertTestLLMConfig(t, fixture.store, fixture.team.ID)

		rec := doRequest(t, fixture.h, http.MethodPost, "/v1/teams/"+fixture.team.ID+"/ai-trigger", fixture.bearer, map[string]any{
			"type":   domain.AIJobTypeProactiveTrigger,
			"params": map[string]any{"source": "rss"},
		})
		if rec.Code != http.StatusAccepted {
			t.Fatalf("POST ai-trigger failing runner: got %d; body: %s", rec.Code, rec.Body.String())
		}

		var got struct {
			JobID  string             `json:"job_id"`
			Status domain.AIJobStatus `json:"status"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.JobID == "" {
			t.Fatal("job_id is empty")
		}
		if got.Status != domain.AIJobStatusPending {
			t.Fatalf("response status = %q, want %q", got.Status, domain.AIJobStatusPending)
		}

		fixture.manager.Wait()

		job, err := fixture.store.GetAIJobByID(ctx, fixture.team.ID, got.JobID)
		if err != nil {
			t.Fatalf("GetAIJobByID: %v", err)
		}
		if job.Status != domain.AIJobStatusFailed {
			t.Fatalf("stored status = %q, want %q", job.Status, domain.AIJobStatusFailed)
		}
	})
}

func TestAITriggerRejectsReservedAutomationParams(t *testing.T) {
	for _, key := range []string{"recurring_automation", "rss_automation", "Recurring_Automation", "RSS_AUTOMATION"} {
		for _, value := range []any{map[string]any{"fallback_content": "forged"}, nil} {
			t.Run(fmt.Sprintf("%s/%v", key, value), func(t *testing.T) {
				f := newAITriggerFixture(t, &fakeRunner{}, auth.ScopeWriteDraft)
				upsertTestLLMConfig(t, f.store, f.team.ID)
				var logs bytes.Buffer
				authSvc, err := auth.New(context.Background(), config.Config{}, f.store)
				if err != nil {
					t.Fatal(err)
				}
				instance := api.New(slog.New(slog.NewTextHandler(&logs, nil)), f.store, authSvc, provider.NewRegistry(), config.Config{}, nil, nil, f.manager, nil)
				f.h = instance.Handler(security.NewLimiter(10000, 10000), nil)
				defer f.manager.Wait()
				rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-trigger", f.bearer, map[string]any{
					"type":   domain.AIJobTypeVoiceEngine,
					"params": map[string]any{key: value},
				})
				if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "ai_job_params_reserved") {
					t.Errorf("forged params: status=%d body=%s", rec.Code, rec.Body.String())
				}
				if !strings.Contains(logs.String(), "reserved automation param rejected") {
					t.Errorf("missing rejection log: %s", logs.String())
				}
				jobs, err := f.store.ListAIJobs(context.Background(), f.team.ID, 20)
				if err != nil || len(jobs) != 0 {
					t.Errorf("forged params created jobs: count=%d err=%v", len(jobs), err)
				}
			})
		}
	}
}

type delayedScopeRunner struct {
	started chan struct{}
	release chan struct{}
}

func (r *delayedScopeRunner) RunJob(ctx context.Context, _ domain.AIJob, _ domain.AIServiceConfig, _ domain.AIContext) (json.RawMessage, error) {
	close(r.started)
	select {
	case <-r.release:
		return json.RawMessage(`{"content":"generated after trigger","scheduled_at":"2027-01-15T12:00:00Z"}`), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestAITrigger_CompletionPreservesScheduleAuthority(t *testing.T) {
	for _, tc := range []struct {
		name        string
		scopes      []string
		autoPublish bool
		wantStatus  domain.PostStatus
	}{
		{name: "draft only with auto publish", scopes: []string{auth.ScopeWriteDraft}, autoPublish: true, wantStatus: domain.PostStatusDraft},
		{name: "draft only before profile enables auto publish", scopes: []string{auth.ScopeWriteDraft}, wantStatus: domain.PostStatusDraft},
		{name: "schedule scope", scopes: []string{auth.ScopeWriteDraft, auth.ScopeWriteSchedule}, autoPublish: true, wantStatus: domain.PostStatusPending},
		{name: "write scope", scopes: []string{auth.ScopeWrite}, autoPublish: true, wantStatus: domain.PostStatusPending},
		{name: "unscoped token", autoPublish: true, wantStatus: domain.PostStatusPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			runner := &delayedScopeRunner{started: make(chan struct{}), release: make(chan struct{})}
			f := newAITriggerFixture(t, runner, tc.scopes...)
			t.Cleanup(f.manager.Wait)
			t.Cleanup(func() { close(runner.release) })
			upsertTestLLMConfig(t, f.store, f.team.ID)
			profile, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{AutoPublishEnabled: tc.autoPublish})
			if err != nil {
				t.Fatal(err)
			}
			rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-trigger", f.bearer, map[string]any{
				"type": domain.AIJobTypeVoiceEngine, "params": map[string]any{},
			})
			if rec.Code != http.StatusAccepted {
				t.Fatalf("POST ai-trigger: got %d; body: %s", rec.Code, rec.Body.String())
			}
			select {
			case <-runner.started:
			case <-time.After(time.Second):
				t.Fatal("AI job did not start")
			}
			if !profile.AutoPublishEnabled {
				profile.AutoPublishEnabled = true
				if _, err := f.store.UpdateTeamProfile(ctx, f.team.ID, profile); err != nil {
					t.Fatal(err)
				}
			}
			runner.release <- struct{}{}
			f.manager.Wait()
			posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(posts) != 1 {
				t.Fatalf("posts = %d, want 1", len(posts))
			}
			if posts[0].Status != tc.wantStatus {
				t.Fatalf("post status = %q, want %q", posts[0].Status, tc.wantStatus)
			}
		})
	}
}

func TestAITrigger_RejectsReservedAuthorityParams(t *testing.T) {
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{name: "top level capability", body: map[string]any{"_can_schedule": true}},
		{name: "null capability", body: map[string]any{"_can_schedule": nil}},
		{name: "nested capability", body: map[string]any{"params": map[string]any{"_can_schedule": true}}},
		{name: "RSS automation", body: map[string]any{"params": map[string]any{"rss_automation": map[string]any{"feed_id": "fake", "item_key": "fake", "draft": false}}}},
		{name: "recurring automation", body: map[string]any{"params": map[string]any{"recurring_automation": map[string]any{"template_id": "fake", "draft": false}}}},
		{name: "case insensitive RSS automation", body: map[string]any{"params": map[string]any{"RSS_AUTOMATION": map[string]any{"feed_id": "fake", "item_key": "fake"}}}},
		{name: "Unicode folded RSS automation", body: map[string]any{"params": map[string]any{"rſs_automation": map[string]any{"feed_id": "fake", "item_key": "fake"}}}},
		{name: "case insensitive recurring automation", body: map[string]any{"params": map[string]any{"Recurring_Automation": map[string]any{"template_id": "fake"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			f := newAITriggerFixture(t, &fakeRunner{}, auth.ScopeWriteDraft)
			t.Cleanup(f.manager.Wait)
			upsertTestLLMConfig(t, f.store, f.team.ID)
			tc.body["type"] = domain.AIJobTypeVoiceEngine
			rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-trigger", f.bearer, tc.body)
			if rec.Code != http.StatusBadRequest || rec.Header().Get("X-Error-Code") != "ai_job_params_reserved" {
				t.Fatalf("expected 400 ai_job_params_reserved; got %d; body: %s", rec.Code, rec.Body.String())
			}
			jobs, err := f.store.ListAIJobs(ctx, f.team.ID, 20)
			if err != nil || len(jobs) != 0 {
				t.Fatalf("reserved params submitted jobs: %v, error: %v", jobs, err)
			}
		})
	}
}
