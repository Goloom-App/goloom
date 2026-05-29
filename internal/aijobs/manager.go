package aijobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
	storepkg "git.f4mily.net/goloom/internal/store"
	"github.com/jackc/pgx/v5"
)

const callbackPath = "/v1/webhooks/ai-callback"

var ErrAIServiceNotConfigured = errors.New("ai service not configured")

type Manager struct {
	store         storepkg.Store
	transport     Transport
	goloomBaseURL string
}

func NewManager(store storepkg.Store, transport Transport, goloomBaseURL string) *Manager {
	if transport == nil {
		transport = &HTTPTransport{}
	}
	return &Manager{
		store:         store,
		transport:     transport,
		goloomBaseURL: goloomBaseURL,
	}
}

func (m *Manager) SubmitJob(ctx context.Context, input domain.AIJob) (domain.AIJob, error) {
	input.Status = domain.AIJobStatusPending

	config, err := m.store.GetAIServiceConfig(ctx, input.TeamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) || isMissingAIServiceConfig(err) {
			return domain.AIJob{}, fmt.Errorf("%w: %v", ErrAIServiceNotConfigured, err)
		}
		return domain.AIJob{}, fmt.Errorf("Manager.SubmitJob: get ai service config: %w", err)
	}

	job, err := m.store.CreateAIJob(ctx, input)
	if err != nil {
		return domain.AIJob{}, fmt.Errorf("Manager.SubmitJob: create ai job: %w", err)
	}

	aiContext, err := m.store.GetTeamAIContext(ctx, input.TeamID)
	if err != nil {
		return job, fmt.Errorf("Manager.SubmitJob: get ai context: %w", err)
	}

	dispatchJob, err := withDispatchEnvelope(job, callbackURL(m.goloomBaseURL), aiContext)
	if err != nil {
		return job, fmt.Errorf("Manager.SubmitJob: marshal dispatch payload: %w", err)
	}

	if err := m.transport.Dispatch(ctx, dispatchJob, config.ServiceURL); err != nil {
		if updateErr := m.store.UpdateAIJobStatus(ctx, job.ID, domain.AIJobStatusFailed, nil, err.Error()); updateErr != nil {
			return job, fmt.Errorf("Manager.SubmitJob: update ai job failed status: %w", updateErr)
		}
		return job, nil
	}

	return job, nil
}

func isMissingAIServiceConfig(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "ai service config") && strings.Contains(msg, "not found")
}

func withDispatchEnvelope(job domain.AIJob, callback string, aiContext domain.AIContext) (domain.AIJob, error) {
	envelope, err := decodeTransportEnvelope(job.Payload)
	if err != nil {
		return domain.AIJob{}, err
	}

	contextRaw, err := json.Marshal(aiContext)
	if err != nil {
		return domain.AIJob{}, err
	}
	envelope.CallbackURL = callback
	envelope.Context = contextRaw

	payload, err := json.Marshal(envelope)
	if err != nil {
		return domain.AIJob{}, err
	}

	job.Payload = payload
	return job, nil
}

func callbackURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		return callbackPath
	}
	return trimmed + callbackPath
}
