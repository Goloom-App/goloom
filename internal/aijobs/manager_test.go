package aijobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

type fakeRunner struct {
	result json.RawMessage
	err    error
}

func (r *fakeRunner) RunJob(_ context.Context, _ domain.AIJob, _ domain.AIServiceConfig, _ domain.AIContext) (json.RawMessage, error) {
	return r.result, r.err
}

type recordingCompleter struct {
	mu      sync.Mutex
	jobID   string
	status  domain.AIJobStatus
	result  json.RawMessage
	message string
	calls   int
}

func (c *recordingCompleter) CompleteAIJob(_ context.Context, jobID string, status domain.AIJobStatus, result json.RawMessage, errorMessage string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jobID = jobID
	c.status = status
	c.result = result
	c.message = errorMessage
	c.calls = 1 + c.calls
}

func newJobTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	enc, err := security.NewEncrypter("aijobs-test-secret-32-bytes-long!")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.New(context.Background(), "file:"+uuid.NewString()+"?mode=memory&cache=shared", enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newJobTestTeam(t *testing.T, s *sqlite.Store, withAIConfig bool) (domain.User, domain.Team) {
	t.Helper()
	ctx := context.Background()
	user, err := s.UpsertOIDCUser(ctx, "job-user-"+uuid.NewString(), "jobs@test", "Jobs")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "jobs-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if withAIConfig {
		if _, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{
			Provider: "openai",
			Model:    "gpt-test",
			APIKey:   "sk-test",
		}); err != nil {
			t.Fatalf("UpsertAIServiceConfig: %v", err)
		}
	}
	return user, team
}

func TestSubmitJobWithoutAIConfigFails(t *testing.T) {
	s := newJobTestStore(t)
	user, team := newJobTestTeam(t, s, false)
	manager := aijobs.NewManager(s, &fakeRunner{})

	_, err := manager.SubmitJob(context.Background(), domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: user.ID,
		Type:         domain.AIJobTypeVoiceEngine,
	})
	if !errors.Is(err, aijobs.ErrAIServiceNotConfigured) {
		t.Fatalf("want ErrAIServiceNotConfigured, got %v", err)
	}
}

func TestSubmitJobWithoutAPIKeyFails(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, false)
	if _, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{Provider: "openai"}); err != nil {
		t.Fatal(err)
	}
	manager := aijobs.NewManager(s, &fakeRunner{})

	_, err := manager.SubmitJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: user.ID,
		Type:         domain.AIJobTypeVoiceEngine,
	})
	if !errors.Is(err, aijobs.ErrAIServiceNotConfigured) {
		t.Fatalf("want ErrAIServiceNotConfigured for empty api key, got %v", err)
	}
}

func TestSubmitJobPersistsResultOnSuccess(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	manager := aijobs.NewManager(s, &fakeRunner{result: json.RawMessage(`{"content":"hello"}`)})

	job, err := manager.SubmitJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: user.ID,
		Type:         domain.AIJobTypeVoiceEngine,
		Payload:      json.RawMessage(`{"params":{}}`),
	})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}
	if job.Status != domain.AIJobStatusPending {
		t.Fatalf("job status after submit = %s, want pending", job.Status)
	}
	manager.Wait()

	stored, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatalf("GetAIJobByID: %v", err)
	}
	if stored.Status != domain.AIJobStatusCompleted {
		t.Fatalf("stored status = %s, want completed (error=%q)", stored.Status, stored.ErrorMessage)
	}
	if string(stored.Result) != `{"content":"hello"}` {
		t.Fatalf("stored result = %s", stored.Result)
	}
}

func TestSubmitJobPersistsFailure(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	manager := aijobs.NewManager(s, &fakeRunner{err: errors.New("llm exploded")})

	job, err := manager.SubmitJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: user.ID,
		Type:         domain.AIJobTypeVoiceEngine,
	})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}
	manager.Wait()

	stored, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.AIJobStatusFailed {
		t.Fatalf("stored status = %s, want failed", stored.Status)
	}
	if stored.ErrorMessage != "llm exploded" {
		t.Fatalf("error message = %q", stored.ErrorMessage)
	}
	if len(stored.Result) != 0 {
		t.Fatalf("failed job must not keep a result, got %s", stored.Result)
	}
}

func TestCompleterReceivesCompletion(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	manager := aijobs.NewManager(s, &fakeRunner{result: json.RawMessage(`{"content":"done"}`)})
	completer := &recordingCompleter{}
	manager.SetCompleter(completer)

	job, err := manager.SubmitJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: user.ID,
		Type:         domain.AIJobTypeVoiceEngine,
	})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}
	manager.Wait()

	completer.mu.Lock()
	defer completer.mu.Unlock()
	if completer.calls != 1 {
		t.Fatalf("completer calls = %d, want 1", completer.calls)
	}
	if completer.jobID != job.ID || completer.status != domain.AIJobStatusCompleted {
		t.Fatalf("completer got job=%s status=%s", completer.jobID, completer.status)
	}
	if string(completer.result) != `{"content":"done"}` {
		t.Fatalf("completer result = %s", completer.result)
	}

	// With a completer registered the manager must not write the status itself.
	stored, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.AIJobStatusPending {
		t.Fatalf("status persistence is the completer's job; store has %s", stored.Status)
	}
}
