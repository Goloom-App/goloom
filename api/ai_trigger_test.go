package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

type aiTriggerFixture struct {
	store  *sqlitestore.Store
	h      http.Handler
	bearer string
	team   domain.Team
	user   domain.User
}

type failingTransport struct {
	err error
}

func (t failingTransport) Dispatch(context.Context, domain.AIJob, string) error {
	return t.err
}

func newAITriggerFixture(t *testing.T, transport aijobs.Transport, scopes ...string) aiTriggerFixture {
	t.Helper()
	ctx := context.Background()
	s := newAICRUDStore(t)
	var jobManager *aijobs.Manager
	if transport != nil {
		jobManager = aijobs.NewManager(s, transport, "http://goloom.test")
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
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "trigger-token", nil, string(rawScopes), nil)
	if err != nil {
		t.Fatal(err)
	}

	return aiTriggerFixture{store: s, h: h, bearer: bearer, team: team, user: u}
}

func TestAITrigger(t *testing.T) {
	t.Run("Creates202WithJobID", func(t *testing.T) {
		ctx := context.Background()
		dispatched := make(chan struct{}, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			dispatched <- struct{}{}
			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()

		fixture := newAITriggerFixture(t, nil, auth.ScopeAITriggerJobs)
		if _, err := fixture.store.UpsertAIServiceConfig(ctx, fixture.team.ID, domain.AIServiceConfig{ServiceURL: server.URL, Description: "test"}); err != nil {
			t.Fatal(err)
		}

		rec := doRequest(t, fixture.h, http.MethodPost, "/v1/teams/"+fixture.team.ID+"/ai-trigger", fixture.bearer, map[string]any{
			"type":   domain.AIJobTypeVoiceEngine,
			"params": map[string]any{"prompt_hint": "ship it"},
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
		case <-dispatched:
		case <-time.After(time.Second):
			t.Fatal("expected AI dispatch request")
		}

		job, err := fixture.store.GetAIJobByID(ctx, fixture.team.ID, got.JobID)
		if err != nil {
			t.Fatalf("GetAIJobByID: %v", err)
		}
		if job.AuthorUserID != fixture.user.ID {
			t.Fatalf("author_user_id = %q, want %q", job.AuthorUserID, fixture.user.ID)
		}
		if job.Type != domain.AIJobTypeVoiceEngine {
			t.Fatalf("type = %q, want %q", job.Type, domain.AIJobTypeVoiceEngine)
		}
	})

	t.Run("Returns422WhenNoConfig", func(t *testing.T) {
		ctx := context.Background()
		fixture := newAITriggerFixture(t, nil, auth.ScopeAITriggerJobs)

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

	t.Run("Returns202WhenServiceUnreachable", func(t *testing.T) {
		ctx := context.Background()
		transport := failingTransport{err: errors.New("dial tcp 127.0.0.1:1: connect: connection refused")}
		fixture := newAITriggerFixture(t, transport, auth.ScopeAITriggerJobs)

		if _, err := fixture.store.UpsertAIServiceConfig(ctx, fixture.team.ID, domain.AIServiceConfig{ServiceURL: "http://127.0.0.1:1", Description: "unreachable"}); err != nil {
			t.Fatal(err)
		}

		rec := doRequest(t, fixture.h, http.MethodPost, "/v1/teams/"+fixture.team.ID+"/ai-trigger", fixture.bearer, map[string]any{
			"type":   domain.AIJobTypeProactiveTrigger,
			"params": map[string]any{"source": "rss"},
		})
		if rec.Code != http.StatusAccepted {
			t.Fatalf("POST ai-trigger unreachable: got %d; body: %s", rec.Code, rec.Body.String())
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

		job, err := fixture.store.GetAIJobByID(ctx, fixture.team.ID, got.JobID)
		if err != nil {
			t.Fatalf("GetAIJobByID: %v", err)
		}
		if job.Status != domain.AIJobStatusFailed {
			t.Fatalf("stored status = %q, want %q", job.Status, domain.AIJobStatusFailed)
		}
	})
}
