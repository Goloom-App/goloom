package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postservice"
)

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
	if err := a.store.UpdateAIJobStatus(ctx, job.ID, status, resultBytes, errorMessage); err != nil {
		a.log.ErrorContext(ctx, "ai job completion: update status failed", "job_id", jobID, "error", err)
		return
	}

	// Persist terminal status before side effects so a retry cannot create another post.
	// Automation callbacks own their separate post/fallback lifecycle below.
	if status == domain.AIJobStatusCompleted &&
		!(job.Type == domain.AIJobTypeVoiceEngine && (parseRSSAutomationMeta(job.Payload) != nil || parseRecurringAutomationMeta(job.Payload) != nil)) {
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

	updatedJob, err := a.store.GetAIJobByIDGlobal(ctx, job.ID)
	if err != nil {
		a.log.ErrorContext(ctx, "ai job completion: reload job failed", "job_id", jobID, "error", err)
		return
	}

	if status == domain.AIJobStatusCompleted {
		if meta := parseRSSAutomationMeta(job.Payload); meta != nil && job.Type == domain.AIJobTypeVoiceEngine {
			a.finishRSSAutomationFromAI(ctx, job, result, meta)
		} else if meta := parseRecurringAutomationMeta(job.Payload); meta != nil && job.Type == domain.AIJobTypeVoiceEngine {
			a.finishRecurringAutomationFromAI(ctx, job, result, meta)
		}
	} else {
		if meta := parseRSSAutomationMeta(job.Payload); meta != nil && job.Type == domain.AIJobTypeVoiceEngine {
			a.finishRSSAutomationFallback(ctx, job, meta)
		} else if meta := parseRecurringAutomationMeta(job.Payload); meta != nil && job.Type == domain.AIJobTypeVoiceEngine {
			a.finishRecurringAutomationFallback(ctx, job, meta)
		}
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
		AccountContentOverride: res.AccountContentOverride,
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
