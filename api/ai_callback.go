package api

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
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
		} else {
			profile, profErr := a.store.GetTeamProfile(ctx, job.TeamID)
			if profErr == nil && profile.AutoPublishEnabled {
				a.autoCreatePostFromCallbackResult(ctx, job, result)
			}
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

func (a *API) autoCreatePostFromCallbackResult(ctx context.Context, job domain.AIJob, rawResult json.RawMessage) {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		_ = json.Unmarshal(rawResult, &res)
	}

	content := strings.TrimSpace(res.Content)
	if content == "" {
		return
	}

	scheduledAt := time.Now().UTC()
	switch {
	case res.ScheduledAt != nil && !res.ScheduledAt.IsZero():
		scheduledAt = res.ScheduledAt.UTC()
	case res.SuggestedScheduledAt != nil && !res.SuggestedScheduledAt.IsZero():
		scheduledAt = res.SuggestedScheduledAt.UTC()
	}

	targetAccounts := targetAccountIDsFromJobPayload(job.Payload)
	overrides := domain.NormalizeAccountContentOverride(res.AccountContentOverride, targetAccounts)

	principal := domain.AuthenticatedPrincipal{
		User: domain.User{ID: job.AuthorUserID},
		Kind: "api_token",
	}

	aiPost := domain.CreatePostInput{
		Title:                  strings.TrimSpace(res.Title),
		Content:                content,
		TargetAccounts:         targetAccounts,
		AccountContentOverride: overrides,
		ScheduledAt:            scheduledAt,
		Draft:                  false,
		AuthorUserID:           &job.AuthorUserID,
		UseVersions:            len(overrides) > 0,
	}
	aiPost.EnsureTitle()
	_, _ = a.store.CreateScheduledPost(ctx, job.TeamID, principal, aiPost)
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
