package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

type aiCallbackFixture struct {
	store  *sqlitestore.Store
	h      http.Handler
	bearer string
	team   domain.Team
	user   domain.User
}

func newAICallbackFixture(t *testing.T, scopes ...string) aiCallbackFixture {
	t.Helper()
	ctx := context.Background()
	s := newAICRUDStore(t)
	h := newAICRUDHandler(t, s)

	u, err := s.UpsertOIDCUser(ctx, "ai-cb-"+uuid.NewString(), "callback@example.test", "AI Callback")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "cb-team-" + uuid.NewString()})
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
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "cb-token", nil, string(rawScopes), nil)
	if err != nil {
		t.Fatal(err)
	}
	return aiCallbackFixture{store: s, h: h, bearer: bearer, team: team, user: u}
}

func makeCallbackJob(t *testing.T, f aiCallbackFixture, status domain.AIJobStatus) domain.AIJob {
	t.Helper()
	job, err := f.store.CreateAIJob(context.Background(), domain.AIJob{
		TeamID:       f.team.ID,
		AuthorUserID: f.user.ID,
		Type:         domain.AIJobTypeVoiceEngine,
		Status:       status,
		Payload:      json.RawMessage(`{"params":{}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestAICallback(t *testing.T) {
	t.Run("UpdatesStatusNoAutoDraft", func(t *testing.T) {
		ctx := context.Background()
		f := newAICallbackFixture(t, auth.ScopeAIWriteDrafts)
		job := makeCallbackJob(t, f, domain.AIJobStatusPending)

		rec := doRequest(t, f.h, http.MethodPost, "/v1/webhooks/ai-callback", f.bearer, map[string]any{
			"job_id":        job.ID,
			"status":        "completed",
			"result":        map[string]any{"content": "hello"},
			"error_message": "",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("POST ai-callback: got %d; body: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]bool
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !resp["acknowledged"] {
			t.Fatalf("acknowledged = false, want true")
		}

		updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetAIJobByIDGlobal: %v", err)
		}
		if updated.Status != domain.AIJobStatusCompleted {
			t.Fatalf("job status = %q, want %q", updated.Status, domain.AIJobStatusCompleted)
		}

		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil {
			t.Fatalf("ListTeamPosts: %v", err)
		}
		if len(posts) != 0 {
			t.Fatalf("expected 0 auto-created posts (auto_publish_enabled=false), got %d", len(posts))
		}
	})

	t.Run("AutoCreatesDraftOnCompletion", func(t *testing.T) {
		ctx := context.Background()
		f := newAICallbackFixture(t, auth.ScopeAIWriteDrafts)

		if _, err := f.store.CreateTeamProfile(ctx, f.team.ID, domain.TeamProfile{
			TeamID:             f.team.ID,
			AutoPublishEnabled: true,
			StyleMetadata:      domain.StyleMetadata{Tonality: "casual"},
		}); err != nil {
			t.Fatal(err)
		}

		job := makeCallbackJob(t, f, domain.AIJobStatusPending)
		scheduledAt := time.Date(2027, 1, 15, 12, 0, 0, 0, time.UTC)

		rec := doRequest(t, f.h, http.MethodPost, "/v1/webhooks/ai-callback", f.bearer, map[string]any{
			"job_id": job.ID,
			"status": "completed",
			"result": map[string]any{
				"content":      "AI-generated post content",
				"scheduled_at": scheduledAt.Format(time.RFC3339),
			},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("POST ai-callback auto-draft: got %d; body: %s", rec.Code, rec.Body.String())
		}

		updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetAIJobByIDGlobal: %v", err)
		}
		if updated.Status != domain.AIJobStatusCompleted {
			t.Fatalf("job status = %q, want %q", updated.Status, domain.AIJobStatusCompleted)
		}

		posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
		if err != nil {
			t.Fatalf("ListTeamPosts: %v", err)
		}
		if len(posts) != 1 {
			t.Fatalf("expected 1 auto-created post, got %d", len(posts))
		}
		if posts[0].Content != "AI-generated post content" {
			t.Fatalf("post content = %q, want %q", posts[0].Content, "AI-generated post content")
		}
		if posts[0].Status != domain.PostStatusPending {
			t.Fatalf("post status = %q, want %q", posts[0].Status, domain.PostStatusPending)
		}
	})

	t.Run("Returns401WhenNoAuth", func(t *testing.T) {
		f := newAICallbackFixture(t, auth.ScopeAIWriteDrafts)
		job := makeCallbackJob(t, f, domain.AIJobStatusPending)

		rec := doRequest(t, f.h, http.MethodPost, "/v1/webhooks/ai-callback", "", map[string]any{
			"job_id": job.ID,
			"status": "completed",
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("IdempotentForTerminalJob", func(t *testing.T) {
		ctx := context.Background()
		f := newAICallbackFixture(t, auth.ScopeAIWriteDrafts)
		job := makeCallbackJob(t, f, domain.AIJobStatusCompleted)

		rec := doRequest(t, f.h, http.MethodPost, "/v1/webhooks/ai-callback", f.bearer, map[string]any{
			"job_id": job.ID,
			"status": "completed",
			"result": map[string]any{"content": "duplicate"},
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for duplicate callback, got %d; body: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]bool
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if !resp["acknowledged"] {
			t.Fatalf("acknowledged = false, want true")
		}

		refetched, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
		if err != nil {
			t.Fatalf("GetAIJobByIDGlobal: %v", err)
		}
		if refetched.Status != domain.AIJobStatusCompleted {
			t.Fatalf("job status changed unexpectedly: got %q", refetched.Status)
		}
	})

	t.Run("Returns404ForNonexistentJob", func(t *testing.T) {
		f := newAICallbackFixture(t, auth.ScopeAIWriteDrafts)

		rec := doRequest(t, f.h, http.MethodPost, "/v1/webhooks/ai-callback", f.bearer, map[string]any{
			"job_id": uuid.NewString(),
			"status": "completed",
		})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})
}
