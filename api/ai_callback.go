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

	var completed *domain.AIJob
	err := a.store.WithAuditTransaction(ctx, func(ctx context.Context) error {
		job, err := a.store.GetAIJobByIDGlobal(ctx, jobID)
		if err != nil {
			return err
		}
		if job.Status == domain.AIJobStatusCompleted || job.Status == domain.AIJobStatusFailed {
			return nil
		}
		claimed, err := a.store.ClaimAIJobCompletion(ctx, job.ID, status, result, errorMessage)
		if err != nil || !claimed {
			return err
		}
		action := "ai_job.complete"
		outcome := "success"
		if status == domain.AIJobStatusFailed {
			action, outcome = "ai_job.fail", "failure"
		}
		rssMeta := parseRSSAutomationMeta(job.Payload)
		recurringMeta := parseRecurringAutomationMeta(job.Payload)
		var res aiCallbackResult
		_ = json.Unmarshal(result, &res)
		fallback := status == domain.AIJobStatusFailed || strings.TrimSpace(res.Content) == ""
		if job.Type == domain.AIJobTypeVoiceEngine && (rssMeta != nil || recurringMeta != nil) {
			if fallback {
				action, outcome = "ai_job.fallback", "fallback"
			}
			switch {
			case rssMeta != nil && fallback:
				err = a.finishRSSAutomationFallback(ctx, job, rssMeta)
			case rssMeta != nil:
				err = a.finishRSSAutomationFromAI(ctx, job, result, rssMeta)
			case fallback:
				err = a.finishRecurringAutomationFallback(ctx, job, recurringMeta)
			default:
				err = a.finishRecurringAutomationFromAI(ctx, job, result, recurringMeta)
			}
		} else if status == domain.AIJobStatusCompleted {
			profile, profErr := a.store.GetTeamProfile(ctx, job.TeamID)
			if profErr == nil && profile.AutoPublishEnabled {
				err = a.autoCreatePostFromCallbackResult(ctx, job, result)
			}
		}
		if err != nil {
			return err
		}
		metadata := map[string]string{"outcome": outcome}
		if rssMeta != nil {
			metadata["feed_id"] = rssMeta.FeedID
		}
		if recurringMeta != nil {
			metadata["template_id"], metadata["post_kind"] = recurringMeta.TemplateID, recurringMeta.PostKind
			if recurringMeta.TemplateOccurrenceAt != nil {
				metadata["occurrence_at"] = recurringMeta.TemplateOccurrenceAt.UTC().Format(time.RFC3339Nano)
			}
		}
		if err := a.store.InsertAuditEvent(ctx, jobAuditEvent(job, action, "ai_job", job.ID, metadata)); err != nil {
			return err
		}
		updated, err := a.store.GetAIJobByIDGlobal(ctx, job.ID)
		if err == nil {
			completed = &updated
		}
		return err
	})
	if err != nil {
		a.log.ErrorContext(ctx, "ai job completion transaction failed", "job_id", jobID, "error", err)
		return
	}
	if completed != nil {
		if event, err := aiJobStreamEvent(*completed); err == nil {
			a.hub.Publish(completed.TeamID, event)
		}
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
		_ = json.Unmarshal(rawResult, &res)
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
	overrides := domain.NormalizeAccountContentOverride(res.AccountContentOverride, targetAccounts)

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
	_, err := a.createJobAutomationPost(ctx, job, aiPost, map[string]string{})
	return err
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
