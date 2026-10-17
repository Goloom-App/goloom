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

// Only guaranteed pre-insertion failures may release an RSS reservation.
var errAutomationPostNotCreated = errors.New("automation post not created")

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

		rssMeta, recurringMeta := parseRSSAutomationMeta(job.Payload), parseRecurringAutomationMeta(job.Payload)
		legacyAutomation := job.Origin != domain.AIJobOriginAutomation && (rssMeta != nil || recurringMeta != nil)
		if legacyAutomation {
			status = domain.AIJobStatusFailed
			errorMessage = "Automation job from before upgrade; please re-run"
			rssMeta, recurringMeta = nil, nil
		}
		automation := job.Type == domain.AIJobTypeVoiceEngine && (rssMeta != nil || recurringMeta != nil)
		var sourceErr error
		if automation {
			if rssMeta != nil {
				sourceErr = a.store.LockAutomationSource(ctx, "rss_feed", rssMeta.FeedID)
			} else {
				sourceErr = a.store.LockAutomationSource(ctx, "post_template", recurringMeta.TemplateID)
			}
			if sourceErr != nil && !errors.Is(sourceErr, domain.ErrNotFound) {
				return sourceErr
			}
		}
		claimed, err := a.store.ClaimAIJobCompletion(ctx, job.ID, status, result, errorMessage)
		if err != nil || !claimed {
			return err
		}
		if legacyAutomation {
			a.log.WarnContext(ctx, "ai job completion: rejecting automation metadata without server provenance", "job_id", job.ID, "team_id", job.TeamID, "origin", job.Origin)
		}
		var effectErr error
		// Failed side effects roll back independently. A successful template fallback
		// retains its post and records the user-visible failed AI outcome.
		err = a.store.WithAuditTransaction(ctx, func(ctx context.Context) error {
			switch {
			case sourceErr != nil:
				effectErr = sourceErr
			case automation && rssMeta != nil && status == domain.AIJobStatusCompleted:
				effectErr = a.finishRSSAutomationFromAI(ctx, job, result, rssMeta)
			case automation && rssMeta != nil:
				effectErr = a.finishRSSAutomationFallback(ctx, job, rssMeta, "provider_failure")
			case automation && status == domain.AIJobStatusCompleted:
				effectErr = a.finishRecurringAutomationFromAI(ctx, job, result, recurringMeta)
			case automation:
				effectErr = a.finishRecurringAutomationFallback(ctx, job, recurringMeta, "provider_failure")
			case status == domain.AIJobStatusCompleted && !noPublishFromJobPayload(job.Payload):
				profile, profErr := a.store.GetTeamProfile(ctx, job.TeamID)
				if profErr == nil && profile.AutoPublishEnabled {
					var draftResult json.RawMessage
					draftResult, effectErr = a.autoCreatePostFromCallbackResult(ctx, job, result)
					if effectErr == nil && draftResult != nil {
						// Notice, draft and audit share the completion transaction.
						effectErr = a.store.UpdateAIJobStatus(ctx, job.ID, status, draftResult, errorMessage)
						if effectErr == nil {
							result = draftResult
						}
					}
				}
			}
			if errors.Is(effectErr, errAutomationTemplateFallback) {
				return nil
			}
			return effectErr
		})
		if (err != nil && !errors.Is(err, effectErr)) || errors.Is(err, errAutomationAudit) || ctx.Err() != nil {
			return err
		}
		action, outcome := "ai_job.complete", "success"
		if status == domain.AIJobStatusFailed {
			action, outcome = "ai_job.fail", "failure"
		}
		if effectErr != nil {
			if rssMeta != nil && errors.Is(effectErr, errAutomationPostNotCreated) {
				attempts, releaseErr := a.store.ReleaseRSSItemAttempt(ctx, job.TeamID, rssMeta.FeedID, rssMeta.ItemKey)
				if errors.Is(releaseErr, domain.ErrNotFound) {
					a.log.InfoContext(ctx, "rss automation: item already linked or unavailable", "job_id", job.ID, "feed_id", rssMeta.FeedID)
				} else if releaseErr != nil {
					a.log.ErrorContext(ctx, "rss automation: release item failed", "job_id", job.ID, "feed_id", rssMeta.FeedID, "error", releaseErr)
				} else {
					a.log.WarnContext(ctx, "rss automation: item attempt failed", "job_id", job.ID, "feed_id", rssMeta.FeedID, "attempts", attempts, "exhausted", attempts >= domain.MaxRSSItemAttempts)
				}
			}
			logMessage := "ai job completion side effect failed"
			if !automation {
				logMessage = "ai job auto-publish failed"
			}
			a.log.ErrorContext(ctx, logMessage, "job_id", job.ID, "team_id", job.TeamID, "error", effectErr)
			status = domain.AIJobStatusFailed
			errorMessage = "Auto-publish failed: post could not be created"
			if automation {
				errorMessage = "Automation failed: post could not be created"
			}
			if errors.Is(effectErr, errAutomationTemplateFallback) {
				errorMessage = "Automation failed: used template fallback"
			}
			if err := a.store.UpdateAIJobStatus(ctx, job.ID, status, result, errorMessage); err != nil {
				return err
			}
			action, outcome = "ai_job.fail", "failure"
		}
		if automation && (errors.Is(effectErr, errAutomationTemplateFallback) || (effectErr == nil && status == domain.AIJobStatusFailed)) {
			action, outcome = "ai_job.fallback", "fallback"
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
		if err := a.store.InsertAuditEvent(ctx, jobAuditEvent(ctx, job, action, "ai_job", job.ID, metadata)); err != nil {
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

// Returns an enriched result only when a post was saved as a draft. Preparing
// JSON before insertion ensures encoding failures cannot follow a created post.
func (a *API) autoCreatePostFromCallbackResult(ctx context.Context, job domain.AIJob, rawResult json.RawMessage) (json.RawMessage, error) {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		if err := json.Unmarshal(rawResult, &res); err != nil {
			return nil, fmt.Errorf("decode auto-publish result: %w", err)
		}
	}

	content := strings.TrimSpace(res.Content)
	if content == "" {
		return nil, nil
	}

	scheduledAt := time.Now().UTC()
	switch {
	case res.ScheduledAt != nil && !res.ScheduledAt.IsZero():
		scheduledAt = res.ScheduledAt.UTC()
	case res.SuggestedScheduledAt != nil && !res.SuggestedScheduledAt.IsZero():
		scheduledAt = res.SuggestedScheduledAt.UTC()
	}

	targetAccounts := targetAccountIDsFromJobPayload(job.Payload)

	aiPost := domain.CreatePostInput{
		Title:                  strings.TrimSpace(res.Title),
		Content:                content,
		TargetAccounts:         targetAccounts,
		AccountContentOverride: a.aiAccountContentOverrides(ctx, job, res.AccountContentOverride, targetAccounts),
		ScheduledAt:            scheduledAt,
		Draft:                  !aiJobCanSchedule(job.Payload),
		AuthorUserID:           &job.AuthorUserID,
	}
	aiPost.EnsureTitle()
	prepared, err := a.posts.Prepare(ctx, job.TeamID, aiPost, postservice.Options{RequireTeam: true, CheckLimits: !aiPost.Draft})
	if err != nil {
		return nil, fmt.Errorf("validate auto-publish post: %w", err)
	}
	if err := postservice.ValidationError(prepared.Validation); err != nil {
		return nil, err
	}
	var draftResult json.RawMessage
	if aiPost.Draft {
		var output map[string]json.RawMessage
		if err := json.Unmarshal(rawResult, &output); err != nil {
			return nil, err
		}
		output["notice"] = json.RawMessage(`"saved_as_draft_without_schedule_authority"`)
		draftResult, err = json.Marshal(output)
		if err != nil {
			return nil, err
		}
	}
	if _, err := a.createJobAutomationPost(ctx, job, prepared.Input, map[string]string{}); err != nil {
		return nil, fmt.Errorf("create auto-publish post: %w", err)
	}
	if aiPost.Draft {
		a.log.InfoContext(ctx, "ai job completion: created draft without schedule authority", "job_id", job.ID)
	}
	return draftResult, nil
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

// Missing authority (including jobs queued before this field existed) fails
// closed. Scheduler automations finish through their separate trusted paths.
func aiJobCanSchedule(payload json.RawMessage) bool {
	var envelope struct {
		CanSchedule bool `json:"_can_schedule"`
	}
	return json.Unmarshal(payload, &envelope) == nil && envelope.CanSchedule
}

// Editor-only jobs still complete normally, but must not create a publishable post.
func noPublishFromJobPayload(payload json.RawMessage) bool {
	var envelope struct {
		Params struct {
			NoPublish bool `json:"no_publish"`
		} `json:"params"`
	}
	return json.Unmarshal(payload, &envelope) == nil && envelope.Params.NoPublish
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
