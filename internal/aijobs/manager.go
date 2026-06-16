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
	"git.f4mily.net/goloom/internal/domain"
	storepkg "git.f4mily.net/goloom/internal/store"
	"github.com/jackc/pgx/v5"
)

var ErrAIServiceNotConfigured = errors.New("ai service not configured")

const jobExecutionTimeout = 10 * time.Minute

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
}

func NewManager(store storepkg.Store, runner Runner) *Manager {
	if runner == nil {
		runner = ai.NewService()
	}
	return &Manager{
		store:  store,
		runner: runner,
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

	job, err := m.store.CreateAIJob(ctx, input)
	if err != nil {
		return domain.AIJob{}, fmt.Errorf("Manager.SubmitJob: create ai job: %w", err)
	}

	aiContext, err := m.store.GetTeamAIContext(ctx, input.TeamID)
	if err != nil {
		return job, fmt.Errorf("Manager.SubmitJob: get ai context: %w", err)
	}

	m.wg.Add(1)
	// The job intentionally outlives the submitting request, so it must not
	// inherit the request context (execute derives its own context internally).
	go m.execute(job, config, aiContext) // #nosec G118 -- background job, decoupled from request lifecycle

	return job, nil
}

func (m *Manager) execute(job domain.AIJob, config domain.AIServiceConfig, aiContext domain.AIContext) {
	defer m.wg.Done()

	log := m.log().With("job_id", job.ID, "job_type", string(job.Type), "team_id", job.TeamID)
	started := time.Now()
	log.Info("ai job started")

	ctx, cancel := context.WithTimeout(context.Background(), jobExecutionTimeout)
	defer cancel()

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

	if completer := m.getCompleter(); completer != nil {
		completer.CompleteAIJob(ctx, job.ID, status, result, errorMessage)
		return
	}
	_ = m.store.UpdateAIJobStatus(ctx, job.ID, status, result, errorMessage)
}

func isMissingAIServiceConfig(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "ai service config") && strings.Contains(msg, "not found")
}
