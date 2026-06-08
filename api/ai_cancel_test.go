package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func TestCancelAIJob(t *testing.T) {
	t.Run("CancelsPendingJob", func(t *testing.T) {
		f := newAICancelFixture(t)
		job := makeCancelJob(t, f, domain.AIJobStatusPending)

		rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+job.ID+"/cancel", f.bearer, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST cancel: got %d; body: %s", rec.Code, rec.Body.String())
		}

		var got domain.AIJob
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.Status != domain.AIJobStatusFailed {
			t.Fatalf("status = %q, want %q", got.Status, domain.AIJobStatusFailed)
		}
		if got.ErrorMessage != "cancelled" {
			t.Fatalf("error_message = %q, want cancelled", got.ErrorMessage)
		}
		if got.CompletedAt == nil {
			t.Fatal("expected completed_at to be set")
		}
	})

	t.Run("CancelsProcessingJob", func(t *testing.T) {
		f := newAICancelFixture(t)
		job := makeCancelJob(t, f, domain.AIJobStatusProcessing)

		rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+job.ID+"/cancel", f.bearer, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST cancel: got %d; body: %s", rec.Code, rec.Body.String())
		}

		var got domain.AIJob
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got.Status != domain.AIJobStatusFailed {
			t.Fatalf("status = %q, want %q", got.Status, domain.AIJobStatusFailed)
		}
	})

	t.Run("IdempotentForFailedJob", func(t *testing.T) {
		f := newAICancelFixture(t)
		job, err := f.store.CreateAIJob(context.Background(), domain.AIJob{
			TeamID:       f.team.ID,
			AuthorUserID: f.user.ID,
			Type:         domain.AIJobTypeProfileAnalysis,
			Status:       domain.AIJobStatusFailed,
			Payload:      json.RawMessage(`{"params":{}}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.UpdateAIJobStatus(context.Background(), job.ID, domain.AIJobStatusFailed, nil, "cancelled"); err != nil {
			t.Fatal(err)
		}

		rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+job.ID+"/cancel", f.bearer, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST cancel failed job: got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Returns409ForCompletedJob", func(t *testing.T) {
		f := newAICancelFixture(t)
		job := makeCancelJob(t, f, domain.AIJobStatusCompleted)

		rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+job.ID+"/cancel", f.bearer, nil)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Returns404ForUnknownJob", func(t *testing.T) {
		f := newAICancelFixture(t)

		rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/00000000-0000-0000-0000-000000000099/cancel", f.bearer, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Returns401WhenNoAuth", func(t *testing.T) {
		f := newAICancelFixture(t)
		job := makeCancelJob(t, f, domain.AIJobStatusPending)

		rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+job.ID+"/cancel", "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d; body: %s", rec.Code, rec.Body.String())
		}
	})
}

type aiCancelFixture struct {
	store  *sqlitestore.Store
	h      http.Handler
	bearer string
	team   domain.Team
	user   domain.User
}

func newAICancelFixture(t *testing.T) aiCancelFixture {
	t.Helper()
	ctx := context.Background()
	s := newAICRUDStore(t)
	h := newAICRUDHandler(t, s)

	u, err := s.UpsertOIDCUser(ctx, "ai-cancel-"+uuid.NewString(), "cancel@example.test", "AI Cancel")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "cancel-team-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}

	rawScopes, err := json.Marshal([]string{auth.ScopeAITriggerJobs})
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "cancel-token", nil, string(rawScopes), nil)
	if err != nil {
		t.Fatal(err)
	}
	return aiCancelFixture{store: s, h: h, bearer: bearer, team: team, user: u}
}

func makeCancelJob(t *testing.T, f aiCancelFixture, status domain.AIJobStatus) domain.AIJob {
	t.Helper()
	job, err := f.store.CreateAIJob(context.Background(), domain.AIJob{
		TeamID:       f.team.ID,
		AuthorUserID: f.user.ID,
		Type:         domain.AIJobTypeProfileAnalysis,
		Status:       status,
		Payload:      json.RawMessage(`{"params":{}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != domain.AIJobStatusPending {
		var result []byte
		if status == domain.AIJobStatusCompleted {
			result = []byte(`{}`)
		}
		if err := f.store.UpdateAIJobStatus(context.Background(), job.ID, status, result, ""); err != nil {
			t.Fatal(err)
		}
		job, err = f.store.GetAIJobByID(context.Background(), f.team.ID, job.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	return job
}
