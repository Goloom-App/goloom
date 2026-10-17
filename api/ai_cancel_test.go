package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"

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

	t.Run("Returns409ForFailedJob", func(t *testing.T) {
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
		if err := f.store.UpdateAIJobStatus(context.Background(), job.ID, domain.AIJobStatusFailed, nil, "provider failed"); err != nil {
			t.Fatal(err)
		}

		rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+job.ID+"/cancel", f.bearer, nil)
		if rec.Code != http.StatusConflict {
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

	rawScopes, err := json.Marshal([]string{auth.ScopeWriteDraft})
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "cancel-token", nil, string(rawScopes), nil, "")
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

// completionOnCancelReadStore returns the processing snapshot after another
// caller has won completion, reproducing the read/update cancellation race.
type completionOnCancelReadStore struct{ store.Store }

func (s completionOnCancelReadStore) GetAIJobByID(ctx context.Context, teamID, jobID string) (domain.AIJob, error) {
	job, err := s.Store.GetAIJobByID(ctx, teamID, jobID)
	if err == nil && job.Status == domain.AIJobStatusProcessing {
		_, err = s.Store.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusCompleted, []byte(`{"content":"winner"}`), "")
	}
	return job, err
}

func TestCancelAIJobDoesNotOverwriteConcurrentCompletion(t *testing.T) {
	f := newAICancelFixture(t)
	job := makeCancelJob(t, f, domain.AIJobStatusProcessing)
	wrapped := completionOnCancelReadStore{Store: f.store}
	authSvc, err := auth.New(context.Background(), config.Config{}, wrapped)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	h := api.New(nil, wrapped, authSvc, provider.NewRegistry(), config.Config{}, nil, catalog, nil, nil).Handler(security.NewLimiter(10000, 10000), nil)
	rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+job.ID+"/cancel", f.bearer, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("cancel lost race: status=%d body=%s", rec.Code, rec.Body.String())
	}
	got, err := f.store.GetAIJobByID(context.Background(), f.team.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.AIJobStatusCompleted || string(got.Result) != `{"content":"winner"}` || got.ErrorMessage != "" {
		t.Fatalf("completion overwritten: %+v", got)
	}
}

func TestCancelRSSAIJobReleasesItem(t *testing.T) {
	ctx := context.Background()
	f := newAICancelFixture(t)
	feed, err := f.store.CreateRSSFeedConfig(ctx, f.team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "cancel-test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.RecordRSSImportedItem(ctx, feed.ID, "cancel-item", ""); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"params": map[string]any{"rss_automation": map[string]string{"feed_id": feed.ID, "item_key": "cancel-item"}}})
	if err != nil {
		t.Fatal(err)
	}
	job, err := f.store.CreateAIJob(ctx, domain.AIJob{TeamID: f.team.ID, AuthorUserID: f.user.ID, Type: domain.AIJobTypeVoiceEngine, Origin: domain.AIJobOriginAutomation, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+f.team.ID+"/ai-jobs/"+job.ID+"/cancel", f.bearer, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
	imported, err := f.store.RSSItemAlreadyImported(ctx, feed.ID, "cancel-item")
	if err != nil {
		t.Fatal(err)
	}
	if imported {
		t.Fatal("cancelled RSS item remains imported")
	}
}
