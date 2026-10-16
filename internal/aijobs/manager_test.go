package aijobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	storepkg "git.f4mily.net/goloom/internal/store"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

// captureHandler records emitted log records for assertions.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) find(message string) (slog.Record, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Message == message {
			return r, true
		}
	}
	return slog.Record{}, false
}

func attrValue(r slog.Record, key string) string {
	var out string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			out = a.Value.String()
			return false
		}
		return true
	})
	return out
}

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

func TestSubmitJobLogsLifecycle(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)

	handler := &captureHandler{}
	manager := aijobs.NewManager(s, &fakeRunner{err: errors.New("llm exploded")})
	manager.SetLogger(slog.New(handler))

	if _, err := manager.SubmitJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: user.ID,
		Type:         domain.AIJobTypeVoiceEngine,
	}); err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}
	manager.Wait()

	if _, ok := handler.find("ai job started"); !ok {
		t.Fatal("expected an 'ai job started' log entry")
	}
	failed, ok := handler.find("ai job failed")
	if !ok {
		t.Fatal("expected an 'ai job failed' log entry")
	}
	if got := attrValue(failed, "error"); got != "llm exploded" {
		t.Fatalf("failed log error attr = %q, want 'llm exploded'", got)
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
	if stored.Status != domain.AIJobStatusProcessing {
		t.Fatalf("final status persistence is the completer's job; store has %s", stored.Status)
	}
}

// failingContextStore makes loading the AI context fail.
type failingContextStore struct {
	storepkg.Store
}

func (failingContextStore) ListTeamAccounts(context.Context, string) ([]domain.SocialAccount, error) {
	return nil, errors.New("context boom")
}

func TestSubmitJobContextFailureLeavesNoPendingJob(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	manager := aijobs.NewManager(failingContextStore{Store: s}, &fakeRunner{})

	if _, err := manager.SubmitJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine}); err == nil {
		t.Fatal("expected error when context load fails")
	}
	jobs, err := s.ListAIJobs(ctx, team.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("no job may be left behind, got %d (status %s)", len(jobs), jobs[0].Status)
	}
}

type funcRunner func(ctx context.Context, job domain.AIJob) (json.RawMessage, error)

func (f funcRunner) RunJob(ctx context.Context, job domain.AIJob, _ domain.AIServiceConfig, _ domain.AIContext) (json.RawMessage, error) {
	return f(ctx, job)
}

func TestExecuteMarksJobProcessing(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	var seen atomic.Value
	manager := aijobs.NewManager(s, funcRunner(func(ctx context.Context, job domain.AIJob) (json.RawMessage, error) {
		stored, err := s.GetAIJobByID(ctx, team.ID, job.ID)
		if err == nil {
			seen.Store(stored.Status)
		}
		return json.RawMessage(`{}`), nil
	}))
	notifier := &recordingNotifier{}
	manager.SetCompleter(notifier)

	if _, err := manager.SubmitJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine}); err != nil {
		t.Fatal(err)
	}
	manager.Wait()
	if got, _ := seen.Load().(domain.AIJobStatus); got != domain.AIJobStatusProcessing {
		t.Fatalf("status during run = %q, want processing", got)
	}
	if notifier.count() != 1 {
		t.Fatalf("status notifier calls = %d, want 1", notifier.count())
	}
}

type recordingNotifier struct {
	recordingCompleter
	n atomic.Int32
}

func (r *recordingNotifier) NotifyAIJobStatus(context.Context, string) { r.n.Add(1) }
func (r *recordingNotifier) count() int                                { return int(r.n.Load()) }

func TestCancelAbortsRunningJob(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	started := make(chan struct{})
	var runErr atomic.Value
	manager := aijobs.NewManager(s, funcRunner(func(ctx context.Context, _ domain.AIJob) (json.RawMessage, error) {
		close(started)
		select {
		case <-ctx.Done():
			runErr.Store(ctx.Err())
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return json.RawMessage(`{}`), nil
		}
	}))

	job, err := manager.SubmitJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if !manager.Cancel(job.ID) {
		t.Fatal("Cancel returned false for a running job")
	}
	manager.Wait()
	if err, _ := runErr.Load().(error); !errors.Is(err, context.Canceled) {
		t.Fatalf("runner ctx err = %v, want context.Canceled", err)
	}
	if manager.Cancel(job.ID) {
		t.Fatal("Cancel must return false once the job finished")
	}
}

func TestConcurrencyLimitQueuesExcessJobs(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	var running, peak atomic.Int32
	manager := aijobs.NewManager(s, funcRunner(func(context.Context, domain.AIJob) (json.RawMessage, error) {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		running.Add(-1)
		return json.RawMessage(`{}`), nil
	}))
	manager.SetMaxConcurrent(2)

	for i := 0; i < 6; i++ {
		if _, err := manager.SubmitJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine}); err != nil {
			t.Fatal(err)
		}
	}
	manager.Wait()
	if peak.Load() > 2 {
		t.Fatalf("peak concurrency = %d, want <= 2", peak.Load())
	}
	jobs, _ := s.ListAIJobs(ctx, team.ID, 10)
	for _, j := range jobs {
		if j.Status != domain.AIJobStatusCompleted && j.Status != domain.AIJobStatusProcessing {
			t.Fatalf("job %s status %s; queued jobs must wait, not fail", j.ID, j.Status)
		}
	}
}

func TestCancelAbortsQueuedJob(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	release := make(chan struct{})
	var runs atomic.Int32
	manager := aijobs.NewManager(s, funcRunner(func(ctx context.Context, _ domain.AIJob) (json.RawMessage, error) {
		runs.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return json.RawMessage(`{}`), nil
	}))
	manager.SetMaxConcurrent(1)
	first, _ := manager.SubmitJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine})
	queued, err := manager.SubmitJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine})
	if err != nil {
		t.Fatal(err)
	}
	if !manager.Cancel(queued.ID) {
		t.Fatal("Cancel returned false for a queued job")
	}
	close(release)
	manager.Wait()
	if runs.Load() != 1 {
		t.Fatalf("runner invoked %d times, queued cancelled job must not run (first=%s)", runs.Load(), first.ID)
	}
}

func TestRecoverInterruptedFailsStaleJobs(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	mk := func(status domain.AIJobStatus) string {
		j, err := s.CreateAIJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine, Status: domain.AIJobStatusPending, Payload: json.RawMessage(`{}`)})
		if err != nil {
			t.Fatal(err)
		}
		if status != domain.AIJobStatusPending {
			_ = s.UpdateAIJobStatus(ctx, j.ID, status, nil, "")
		}
		return j.ID
	}
	pending, processing, done := mk(domain.AIJobStatusPending), mk(domain.AIJobStatusProcessing), mk(domain.AIJobStatusCompleted)

	manager := aijobs.NewManager(s, &fakeRunner{})
	n, err := manager.RecoverInterrupted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("recovered = %d, want 2", n)
	}
	for _, id := range []string{pending, processing} {
		j, _ := s.GetAIJobByID(ctx, team.ID, id)
		if j.Status != domain.AIJobStatusFailed || j.ErrorMessage != aijobs.InterruptedMessage {
			t.Fatalf("job %s = %s/%q", id, j.Status, j.ErrorMessage)
		}
	}
	if j, _ := s.GetAIJobByID(ctx, team.ID, done); j.Status != domain.AIJobStatusCompleted {
		t.Fatalf("completed job changed to %s", j.Status)
	}
}

func TestRecoverInterruptedUsesCompleter(t *testing.T) {
	s := newJobTestStore(t)
	ctx := context.Background()
	user, team := newJobTestTeam(t, s, true)
	j, _ := s.CreateAIJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine, Status: domain.AIJobStatusPending, Payload: json.RawMessage(`{}`)})
	manager := aijobs.NewManager(s, &fakeRunner{})
	c := &recordingCompleter{}
	manager.SetCompleter(c)
	if _, err := manager.RecoverInterrupted(ctx); err != nil {
		t.Fatal(err)
	}
	if c.calls != 1 || c.jobID != j.ID || c.status != domain.AIJobStatusFailed || c.message != aijobs.InterruptedMessage {
		t.Fatalf("completer got %+v", c)
	}
}
