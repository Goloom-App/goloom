// Package aijobs queues and executes AI jobs in-process using the native Go
// AI engine (internal/ai). Job completion side effects (post creation,
// automation finishing, SSE events) are applied through a Completer that the
// API layer registers.
package aijobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"git.f4mily.net/goloom/internal/ai"
	"git.f4mily.net/goloom/internal/aicontext"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	storepkg "git.f4mily.net/goloom/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrAIServiceNotConfigured = errors.New("ai service not configured")

const jobExecutionTimeout = 10 * time.Minute

// DefaultMaxConcurrent caps simultaneously running jobs (global). Jobs over
// the limit wait in status pending until a slot frees up.
const DefaultMaxConcurrent = 4

// staleMargin is added to jobExecutionTimeout before an unfinished job counts
// as certainly dead (no live instance can still be running it).
const staleMargin = 2 * time.Minute

// recoveryInterval is how often RunRecovery sweeps for dead jobs.
const recoveryInterval = 5 * time.Minute

// InterruptedMessage is the error recorded on jobs orphaned by a restart.
const InterruptedMessage = "interrupted by restart"

// StatusNotifier is optionally implemented by the Completer to broadcast a
// non-final status change (processing) to live clients.
type StatusNotifier interface {
	NotifyAIJobStatus(ctx context.Context, jobID string)
}

// Runner executes one AI job and returns its result payload.
type Runner interface {
	RunJob(ctx context.Context, job domain.AIJob, cfg domain.AIServiceConfig, aiContext domain.AIContext) (json.RawMessage, error)
}

// Completer applies a finished job: persists the status and runs side effects
// (auto-publishing, automation finishing, SSE events). Registered by the API.
type Completer interface {
	CompleteAIJob(ctx context.Context, jobID string, status domain.AIJobStatus, result json.RawMessage, errorMessage string)
}

type Manager struct {
	store  storepkg.Store
	runner Runner
	logger *slog.Logger

	mu        sync.RWMutex
	completer Completer

	// wg lets tests wait for in-flight jobs.
	wg sync.WaitGroup

	// cancels holds the cancel func of every queued or running job.
	cancelMu sync.Mutex
	cancels  map[string]context.CancelFunc

	staleAfter time.Duration

	// slots is the concurrency semaphore; replaced via SetMaxConcurrent.
	slots chan struct{}
}

func NewManager(store storepkg.Store, runner Runner) *Manager {
	if runner == nil {
		runner = ai.NewService()
	}
	return &Manager{
		store:   store,
		runner:  runner,
		cancels: map[string]context.CancelFunc{},
		slots:   make(chan struct{}, DefaultMaxConcurrent),

		staleAfter: jobExecutionTimeout + staleMargin,
	}
}

// SetLogger registers the structured logger used to record AI job activity.
// Called once at startup before any job runs; without it, the manager logs
// nothing and AI activity never reaches the persisted log.
func (m *Manager) SetLogger(logger *slog.Logger) {
	m.logger = logger
}

func (m *Manager) log() *slog.Logger {
	if m.logger != nil {
		return m.logger
	}
	return slog.New(slog.DiscardHandler)
}

// SetCompleter registers the completion handler. Without one, the manager
// only persists the job status.
func (m *Manager) SetCompleter(completer Completer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completer = completer
}

func (m *Manager) getCompleter() Completer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.completer
}

// SetMaxConcurrent sets the global limit of simultaneously running jobs.
// Call before submitting jobs.
func (m *Manager) SetMaxConcurrent(n int) {
	if n < 1 {
		n = 1
	}
	m.slots = make(chan struct{}, n)
}

// Cancel aborts the queued or running job with the given id. It reports
// whether the manager knew the job (false: unknown, finished, or run by
// another process).
func (m *Manager) Cancel(jobID string) bool {
	m.cancelMu.Lock()
	cancel, ok := m.cancels[jobID]
	m.cancelMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// SetStaleAfter overrides the age after which an unfinished job counts as
// dead (default jobExecutionTimeout + margin). Intended for tests.
func (m *Manager) SetStaleAfter(d time.Duration) { m.staleAfter = d }

// RunRecovery calls RecoverInterrupted now and then every recoveryInterval
// (or the given interval) until ctx ends. Call after the completer is set.
func (m *Manager) RunRecovery(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = recoveryInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if n, err := m.RecoverInterrupted(ctx); err != nil {
			m.log().Error("recover interrupted ai jobs failed", "error", err.Error())
		} else if n > 0 {
			m.log().Warn("failed interrupted ai jobs", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RecoverInterrupted fails jobs left pending/processing whose last update is
// older than the execution timeout plus margin: their goroutines died with
// their process. Younger jobs are left alone because another instance sharing
// the database may still be running them. The completer must be registered
// first so automation fallbacks and SSE events fire.
func (m *Manager) RecoverInterrupted(ctx context.Context) (int, error) {
	jobs, err := m.store.ListUnfinishedAIJobs(ctx, time.Now().Add(-m.staleAfter), 1000)
	if err != nil {
		return 0, fmt.Errorf("Manager.RecoverInterrupted: list jobs: %w", err)
	}
	for _, job := range jobs {
		m.log().Warn("ai job interrupted by restart", "job_id", job.ID, "job_type", string(job.Type), "team_id", job.TeamID)
		if completer := m.getCompleter(); completer != nil {
			completer.CompleteAIJob(ctx, job.ID, domain.AIJobStatusFailed, nil, InterruptedMessage)
			continue
		}
		if err := m.persistCompletion(ctx, job.ID, domain.AIJobStatusFailed, nil, InterruptedMessage); err != nil {
			return 0, fmt.Errorf("Manager.RecoverInterrupted: fail job %s: %w", job.ID, err)
		}
	}
	return len(jobs), nil
}

// Wait blocks until all in-flight jobs finished (used by tests).
func (m *Manager) Wait() {
	m.wg.Wait()
}

// SubmitJob persists the job and executes it asynchronously in-process.
func (m *Manager) SubmitJob(ctx context.Context, input domain.AIJob) (domain.AIJob, error) {
	input.Status = domain.AIJobStatusPending

	config, err := m.store.GetAIServiceConfig(ctx, input.TeamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) || isMissingAIServiceConfig(err) {
			return domain.AIJob{}, fmt.Errorf("%w: %v", ErrAIServiceNotConfigured, err)
		}
		return domain.AIJob{}, fmt.Errorf("Manager.SubmitJob: get ai service config: %w", err)
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return domain.AIJob{}, fmt.Errorf("%w: missing api key", ErrAIServiceNotConfigured)
	}

	// Load everything that can fail before persisting, so a setup error never
	// leaves a job pending forever.
	aiContext, err := aicontext.Build(ctx, m.store, input.TeamID, aicontext.Full())
	if err != nil {
		return domain.AIJob{}, fmt.Errorf("Manager.SubmitJob: get ai context: %w", err)
	}

	var envelope struct {
		Params json.RawMessage `json:"params"`
	}
	if len(input.Payload) > 0 {
		if err := json.Unmarshal(input.Payload, &envelope); err != nil {
			return domain.AIJob{}, err
		}
	}
	aiContext, err = aicontext.Select(aiContext, envelope.Params)
	if err != nil {
		return domain.AIJob{}, err
	}

	var job domain.AIJob
	err = m.store.WithAuditTransaction(ctx, func(ctx context.Context) error {
		var err error
		job, err = m.store.CreateAIJob(ctx, input)
		if err != nil {
			return err
		}
		channel := "scheduler"
		if job.Origin == domain.AIJobOriginPreview {
			channel = "preview"
		}
		event := domain.AuditEvent{ID: uuid.NewString(), TeamID: job.TeamID, ActorKind: domain.AuditActorSystem, ActorName: "Automation", Action: "ai_job.submit", TargetType: "ai_job", TargetID: &job.ID, Metadata: map[string]string{"channel": channel, "outcome": "success", "job_id": job.ID, "author_user_id": job.AuthorUserID}}
		if p := auth.PrincipalFromContext(ctx); p != nil {
			if job.Origin != domain.AIJobOriginPreview {
				event.Metadata["channel"] = "rest"
			}
			event.Metadata["initiator_user_id"] = p.User.ID
			if p.Kind == domain.AuditActorToken && p.TokenID != nil {
				event.Metadata["initiator_token_id"] = *p.TokenID
			}
			if job.Origin != domain.AIJobOriginAutomation {
				event.ActorUserID, event.ActorName, event.ActorEmail, event.ActorKind = p.User.ID, p.User.Name, p.User.Email, p.Kind
				if p.Kind == domain.AuditActorToken {
					event.TokenID, event.TokenName = p.TokenID, p.TokenName
				}
			}
		}
		return m.store.InsertAuditEvent(ctx, event)
	})
	if err != nil {
		return domain.AIJob{}, fmt.Errorf("Manager.SubmitJob: create ai job: %w", err)
	}

	storepkg.AfterAuditCommit(ctx, func(ctx context.Context) {
		// The job intentionally outlives the submitting request: it keeps the
		// request's values but not its cancellation; Cancel/execute end it.
		jobCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		m.cancelMu.Lock()
		m.cancels[job.ID] = cancel
		m.cancelMu.Unlock()

		m.wg.Add(1)
		go m.execute(jobCtx, cancel, m.slots, job, config, aiContext) // #nosec G118 -- background job, decoupled from request lifecycle; cancelled via Cancel/execute

	})

	return job, nil
}

func (m *Manager) execute(jobCtx context.Context, cancelJob context.CancelFunc, slots chan struct{}, job domain.AIJob, config domain.AIServiceConfig, aiContext domain.AIContext) {
	defer m.wg.Done()
	defer func() {
		cancelJob()
		m.cancelMu.Lock()
		delete(m.cancels, job.ID)
		m.cancelMu.Unlock()
	}()

	log := m.log().With("job_id", job.ID, "job_type", string(job.Type), "team_id", job.TeamID)

	// Wait for a free slot; the job stays pending meanwhile.
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
		if jobCtx.Err() != nil { // select may pick the slot although cancelled
			m.finish(context.Background(), job.ID, domain.AIJobStatusFailed, nil, "cancelled")
			return
		}
	case <-jobCtx.Done():
		m.finish(context.Background(), job.ID, domain.AIJobStatusFailed, nil, "cancelled")
		return
	}

	started := time.Now()
	log.Info("ai job started")

	ctx, cancel := context.WithTimeout(jobCtx, jobExecutionTimeout)
	defer cancel()

	// Conditional pending -> processing: a job cancelled in the meantime stays
	// failed and must not run.
	moved, err := m.store.MarkAIJobProcessing(ctx, job.ID)
	if err != nil {
		log.Warn("ai job: mark processing failed", "error", err.Error())
	} else if !moved {
		log.Info("ai job no longer pending, skipping")
		return
	}
	if moved {
		if n, ok := m.getCompleter().(StatusNotifier); ok {
			n.NotifyAIJobStatus(ctx, job.ID)
		}
	}

	result, err := m.runner.RunJob(ctx, job, config, aiContext)
	status := domain.AIJobStatusCompleted
	errorMessage := ""
	if err != nil {
		status = domain.AIJobStatusFailed
		errorMessage = err.Error()
		result = nil
		log.Error("ai job failed", "duration_ms", time.Since(started).Milliseconds(), "error", errorMessage)
	} else {
		log.Info("ai job completed", "duration_ms", time.Since(started).Milliseconds())
	}

	m.finish(ctx, job.ID, status, result, errorMessage)
}

// finish persists the final status via the completer (or the store directly).
// A cancelled context must not prevent the final write.
func (m *Manager) finish(ctx context.Context, jobID string, status domain.AIJobStatus, result json.RawMessage, errorMessage string) {
	if ctx.Err() != nil {
		ctx = context.WithoutCancel(ctx)
	}
	if completer := m.getCompleter(); completer != nil {
		completer.CompleteAIJob(ctx, jobID, status, result, errorMessage)
		return
	}
	if err := m.persistCompletion(ctx, jobID, status, result, errorMessage); err != nil {
		m.log().Error("AI completion audit transaction failed", "job_id", jobID, "error", err)
	}
}

func (m *Manager) persistCompletion(ctx context.Context, jobID string, status domain.AIJobStatus, result json.RawMessage, errorMessage string) error {
	return m.store.WithAuditTransaction(ctx, func(ctx context.Context) error {
		claimed, err := m.store.ClaimAIJobCompletion(ctx, jobID, status, result, errorMessage)
		if err != nil || !claimed {
			return err
		}
		job, err := m.store.GetAIJobByIDGlobal(ctx, jobID)
		if err != nil {
			return err
		}
		action, outcome := "ai_job.complete", "success"
		if status == domain.AIJobStatusFailed {
			action, outcome = "ai_job.fail", "failure"
		}
		return m.store.InsertAuditEvent(ctx, domain.AuditEvent{TeamID: job.TeamID, ActorKind: domain.AuditActorSystem, ActorName: "Automation", Action: action, TargetType: "ai_job", TargetID: &job.ID, Metadata: map[string]string{"channel": "ai", "outcome": outcome, "job_id": job.ID, "author_user_id": job.AuthorUserID}})
	})
}

func isMissingAIServiceConfig(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "ai service config") && strings.Contains(msg, "not found")
}
