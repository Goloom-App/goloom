package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postservice"
)

// Only validation failures before insertion are safe to retry with template content.
var errAutomationPostValidation = errors.New("invalid automation post")

var errAutomationTemplateFallback = errors.New("used template fallback")

// aiCallbackResult is the shape of completed AI job results consumed by the
// completion side effects below.
type aiCallbackResult struct {
	Title                  string            `json:"title"`
	Content                string            `json:"content"`
	ScheduledAt            *time.Time        `json:"scheduled_at"`
	SuggestedScheduledAt   *time.Time        `json:"suggested_scheduled_at"`
	AccountContentOverride map[string]string `json:"account_content_override"`
}

// CompleteAIJob implements aijobs.Completer: it persists the final job status
// and applies completion side effects (automation finishing, auto publishing,
// SSE updates).
func (a *API) CompleteAIJob(ctx context.Context, jobID string, status domain.AIJobStatus, result json.RawMessage, errorMessage string) {
	if status != domain.AIJobStatusCompleted && status != domain.AIJobStatusFailed {
		return
	}

	job, err := a.store.GetAIJobByIDGlobal(ctx, jobID)
	if err != nil {
		a.log.ErrorContext(ctx, "ai job completion: job not found", "job_id", jobID, "error", err)
		return
	}
	if job.Status == domain.AIJobStatusCompleted || job.Status == domain.AIJobStatusFailed {
		return
	}

	var resultBytes []byte
	if len(result) > 0 {
		resultBytes = []byte(result)
	}
	claimed, err := a.store.ClaimAIJobCompletion(ctx, job.ID, status, resultBytes, errorMessage)
	if err != nil {
		a.log.ErrorContext(ctx, "ai job completion: update status failed", "job_id", jobID, "error", err)
		return
	}

	if !claimed {
		return
	}

	rssMeta := parseRSSAutomationMeta(job.Payload)
	recurringMeta := parseRecurringAutomationMeta(job.Payload)
	if job.Origin != domain.AIJobOriginAutomation && (rssMeta != nil || recurringMeta != nil) {
		a.log.WarnContext(ctx, "ai job completion: ignoring automation metadata without server provenance; completing as normal job", "job_id", job.ID, "team_id", job.TeamID, "origin", job.Origin)
		rssMeta, recurringMeta = nil, nil
	}
	automation := job.Type == domain.AIJobTypeVoiceEngine && (rssMeta != nil || recurringMeta != nil)

	// Only the atomic terminal-claim winner runs side effects, including across replicas.
	// Automation callbacks own their separate post/fallback lifecycle below.
	if status == domain.AIJobStatusCompleted && !automation {
		profile, profErr := a.store.GetTeamProfile(ctx, job.TeamID)
		if profErr == nil && profile.AutoPublishEnabled {
			if err := a.autoCreatePostFromCallbackResult(ctx, job, result); err != nil {
				a.log.ErrorContext(ctx, "ai job auto-publish failed", "job_id", job.ID, "team_id", job.TeamID, "error", err)
				status = domain.AIJobStatusFailed
				errorMessage = "Auto-publish failed: post could not be created"
				if err := a.store.UpdateAIJobStatus(ctx, job.ID, status, resultBytes, errorMessage); err != nil {
					a.log.ErrorContext(ctx, "ai job auto-publish: update failure status failed", "job_id", job.ID, "error", err)
					return
				}
			}
		}
	}

	var automationErr error
	if automation {
		if status == domain.AIJobStatusCompleted {
			if rssMeta != nil {
				automationErr = a.finishRSSAutomationFromAI(ctx, job, result, rssMeta)
			} else {
				automationErr = a.finishRecurringAutomationFromAI(ctx, job, result, recurringMeta)
			}
		} else {
			if rssMeta != nil {
				automationErr = a.finishRSSAutomationFallback(ctx, job, rssMeta)
			} else {
				automationErr = a.finishRecurringAutomationFallback(ctx, job, recurringMeta)
			}
		}
	}

	if automationErr != nil {
		a.log.ErrorContext(ctx, "ai job automation failed", "job_id", job.ID, "team_id", job.TeamID, "error", automationErr, "job_error", errorMessage)
		errorMessage = "Automation failed: post could not be created"
		if errors.Is(automationErr, errAutomationTemplateFallback) {
			errorMessage = "Automation failed: used template fallback"
		}
		if err := a.store.UpdateAIJobStatus(ctx, job.ID, domain.AIJobStatusFailed, resultBytes, errorMessage); err != nil {
			a.log.ErrorContext(ctx, "ai job automation: update failure status failed", "job_id", job.ID, "error", err)
			return
		}
	}
	updatedJob, err := a.store.GetAIJobByIDGlobal(ctx, job.ID)
	if err != nil {
		a.log.ErrorContext(ctx, "ai job completion: reload job failed", "job_id", jobID, "error", err)
		return
	}

	if event, evErr := aiJobStreamEvent(updatedJob); evErr == nil {
		a.hub.Publish(job.TeamID, event)
	}
}

// NotifyAIJobStatus implements aijobs.StatusNotifier: publishes the current
// (non-final) job state, e.g. processing, to the team's SSE stream.
func (a *API) NotifyAIJobStatus(ctx context.Context, jobID string) {
	job, err := a.store.GetAIJobByIDGlobal(ctx, jobID)
	if err != nil {
		a.log.ErrorContext(ctx, "ai job status notify: job not found", "job_id", jobID, "error", err)
		return
	}
	if event, evErr := aiJobStreamEvent(job); evErr == nil {
		a.hub.Publish(job.TeamID, event)
	}
}

func (a *API) autoCreatePostFromCallbackResult(ctx context.Context, job domain.AIJob, rawResult json.RawMessage) error {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		if err := json.Unmarshal(rawResult, &res); err != nil {
			return fmt.Errorf("decode auto-publish result: %w", err)
		}
	}

	content := strings.TrimSpace(res.Content)
	if content == "" {
		return nil
	}

	scheduledAt := time.Now().UTC()
	switch {
	case res.ScheduledAt != nil && !res.ScheduledAt.IsZero():
		scheduledAt = res.ScheduledAt.UTC()
	case res.SuggestedScheduledAt != nil && !res.SuggestedScheduledAt.IsZero():
		scheduledAt = res.SuggestedScheduledAt.UTC()
	}

	targetAccounts := targetAccountIDsFromJobPayload(job.Payload)

	principal := domain.AuthenticatedPrincipal{
		User: domain.User{ID: job.AuthorUserID},
		Kind: "api_token",
	}

	aiPost := domain.CreatePostInput{
		Title:                  strings.TrimSpace(res.Title),
		Content:                content,
		TargetAccounts:         targetAccounts,
		AccountContentOverride: a.aiAccountContentOverrides(ctx, job, res.AccountContentOverride, targetAccounts),
		ScheduledAt:            scheduledAt,
		Draft:                  false,
		AuthorUserID:           &job.AuthorUserID,
	}
	aiPost.EnsureTitle()
	prepared, err := a.posts.Prepare(ctx, job.TeamID, aiPost, postservice.Options{RequireTeam: true, CheckLimits: true})
	if err != nil {
		return fmt.Errorf("validate auto-publish post: %w", err)
	}
	if err := postservice.ValidationError(prepared.Validation); err != nil {
		return err
	}
	if _, err := a.store.CreateScheduledPost(ctx, job.TeamID, principal, prepared.Input); err != nil {
		return fmt.Errorf("create auto-publish post: %w", err)
	}
	return nil
}

// AI output may invent override keys; discard them without weakening target validation.
func (a *API) aiAccountContentOverrides(ctx context.Context, job domain.AIJob, overrides map[string]string, targets []string) map[string]string {
	for id := range overrides {
		if !slices.Contains(targets, id) {
			a.log.WarnContext(ctx, "ai job: dropping override outside targets", "job_id", job.ID, "team_id", job.TeamID, "account_id", id)
		}
	}
	return domain.NormalizeAccountContentOverride(overrides, targets)
}

func targetAccountIDsFromJobPayload(payload json.RawMessage) []string {
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil
	}
	var envelope struct {
		Params struct {
			TargetAccountIDs []string `json:"target_account_ids"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil
	}
	return domain.NormalizeMediaIDs(envelope.Params.TargetAccountIDs)
}
