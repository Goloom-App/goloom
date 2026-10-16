package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

type blockingRunner struct {
	started chan struct{}
	ctxErr  chan error
}

func (r *blockingRunner) RunJob(ctx context.Context, _ domain.AIJob, _ domain.AIServiceConfig, _ domain.AIContext) (json.RawMessage, error) {
	close(r.started)
	select {
	case <-ctx.Done():
		r.ctxErr <- ctx.Err()
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return json.RawMessage(`{}`), nil
	}
}

func TestCancelAIJobAbortsRunningLLMCall(t *testing.T) {
	ctx := context.Background()
	s := newAICRUDStore(t)
	u, err := s.UpsertOIDCUser(ctx, "ai-cancel-run-"+uuid.NewString(), "cancelrun@example.test", "Cancel Run")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "cancel-run-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{Provider: "openai", Model: "m", APIKey: "sk-test"}); err != nil {
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
	bearer, _, err := s.CreateUserAPIToken(ctx, u.ID, "cancel-run-token", nil, string(rawScopes), nil, "")
	if err != nil {
		t.Fatal(err)
	}

	runner := &blockingRunner{started: make(chan struct{}), ctxErr: make(chan error, 1)}
	manager := aijobs.NewManager(s, runner)
	h := newAICRUDHandlerWithManager(t, s, manager)

	job, err := manager.SubmitJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: u.ID, Type: domain.AIJobTypeVoiceEngine, Payload: json.RawMessage(`{"params":{}}`)})
	if err != nil {
		t.Fatal(err)
	}
	<-runner.started

	rec := doRequest(t, h, http.MethodPost, "/v1/teams/"+team.ID+"/ai-jobs/"+job.ID+"/cancel", bearer, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST cancel: got %d; body: %s", rec.Code, rec.Body.String())
	}
	select {
	case err := <-runner.ctxErr:
		if err != context.Canceled {
			t.Fatalf("runner ctx err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("running LLM call was not cancelled")
	}
	manager.Wait()
	stored, _ := s.GetAIJobByID(ctx, team.ID, job.ID)
	if stored.Status != domain.AIJobStatusFailed || stored.ErrorMessage != "cancelled" {
		t.Fatalf("stored = %s/%q, want failed/cancelled", stored.Status, stored.ErrorMessage)
	}
}
