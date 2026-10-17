package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
)

func TestCancelAIJobIdempotent(t *testing.T) {
	f := newAICancelFixture(t)
	job := makeCancelJob(t, f, domain.AIJobStatusPending)
	url := "/v1/teams/" + f.team.ID + "/ai-jobs/" + job.ID + "/cancel"
	var first domain.AIJob
	for attempt := range 2 {
		rec := doRequest(t, f.h, http.MethodPost, url, f.bearer, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("cancel attempt %d: status=%d body=%s", attempt+1, rec.Code, rec.Body.String())
		}
		var got domain.AIJob
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Status != domain.AIJobStatusFailed || got.ErrorMessage != "cancelled" || got.CompletedAt == nil {
			t.Fatalf("cancelled job: %+v", got)
		}
		if attempt == 0 {
			first = got
		} else if !got.CompletedAt.Equal(*first.CompletedAt) {
			t.Fatal("repeated cancellation changed completion timestamp")
		}
	}
}

// Another cancellation wins after this request reads the processing snapshot.
type cancellationOnCancelReadStore struct{ store.Store }

func (s cancellationOnCancelReadStore) GetAIJobByID(ctx context.Context, teamID, jobID string) (domain.AIJob, error) {
	job, err := s.Store.GetAIJobByID(ctx, teamID, jobID)
	if err == nil && job.Status == domain.AIJobStatusProcessing {
		_, err = s.Store.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusFailed, nil, "cancelled")
	}
	return job, err
}

func TestCancelAIJobConcurrentCancellationIsIdempotent(t *testing.T) {
	f := newAICancelFixture(t)
	job := makeCancelJob(t, f, domain.AIJobStatusProcessing)
	wrapped := cancellationOnCancelReadStore{f.store}
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
	if rec.Code != http.StatusOK {
		t.Fatalf("concurrent cancel: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCancelForeignCancelledJobStillReturns404(t *testing.T) {
	f := newAICancelFixture(t)
	other, err := f.store.CreateTeam(context.Background(), f.user.ID, domain.CreateTeamInput{Name: "other cancel team"})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	if _, err := f.store.UpdateTeam(context.Background(), other.ID, domain.UpdateTeamInput{Name: other.Name, IsAIEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	job := makeCancelJob(t, f, domain.AIJobStatusPending)
	if err := f.store.UpdateAIJobStatus(context.Background(), job.ID, domain.AIJobStatusFailed, nil, "cancelled"); err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, f.h, http.MethodPost, "/v1/teams/"+other.ID+"/ai-jobs/"+job.ID+"/cancel", f.bearer, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign cancelled job: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
