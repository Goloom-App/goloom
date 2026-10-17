package api

import (
	"context"
	"net/http"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/store"
)

const aiJobCancelledMessage = "cancelled"

func (a *API) handleCancelAIJob(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	jobID := r.PathValue("jobID")

	job, err := a.store.GetAIJobByID(r.Context(), teamID, jobID)
	if err != nil {
		a.writeError(w, r, "ai_job_not_found", http.StatusNotFound)
		return
	}

	won, err := a.store.ClaimAIJobCompletion(r.Context(), job.ID, domain.AIJobStatusFailed, nil, aiJobCancelledMessage)
	if err != nil {
		a.log.ErrorContext(r.Context(), "cancel AI job failed", "job_id", job.ID, "error", err)
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}
	if !won {
		current, loadErr := a.store.GetAIJobByID(r.Context(), teamID, jobID)
		if loadErr != nil {
			a.log.ErrorContext(r.Context(), "load AI job after cancellation conflict failed", "job_id", job.ID, "error", loadErr)
			a.writeError(w, r, "internal_error", http.StatusInternalServerError)
			return
		}
		if current.Status == domain.AIJobStatusFailed && current.ErrorMessage == aiJobCancelledMessage {
			auth.WriteJSON(w, http.StatusOK, current)
			return
		}
		a.writeError(w, r, "ai_job_not_cancellable", http.StatusConflict)
		return
	}

	if job.Origin == domain.AIJobOriginAutomation && job.Type == domain.AIJobTypeVoiceEngine {
		if meta := parseRSSAutomationMeta(job.Payload); meta != nil {
			attempts, releaseErr := a.store.ReleaseRSSItemAttempt(context.WithoutCancel(r.Context()), job.TeamID, meta.FeedID, meta.ItemKey)
			if releaseErr != nil {
				a.log.WarnContext(r.Context(), "cancelled RSS AI job item release failed", "job_id", job.ID, "feed_id", meta.FeedID, "item_key", meta.ItemKey, "attempts", attempts, "error", releaseErr)
			} else if attempts >= domain.MaxRSSItemAttempts {
				a.log.WarnContext(r.Context(), "cancelled RSS AI job exhausted item attempts", "job_id", job.ID, "feed_id", meta.FeedID, "item_key", meta.ItemKey, "attempts", attempts)
			}
		}
	}

	a.recordAudit(r, teamID, "ai_job.cancel", "ai_job", &jobID, "Cancelled AI job", map[string]string{"job_id": jobID})
	// Abort the in-process LLM call (no-op if the job is not running here).
	store.AfterAuditCommit(r.Context(), func(context.Context) { a.jobManager.Cancel(job.ID) })

	updatedJob, err := a.store.GetAIJobByID(r.Context(), teamID, jobID)
	if err != nil {
		a.log.ErrorContext(r.Context(), "load cancelled AI job failed", "job_id", job.ID, "error", err)
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}

	if event, evErr := aiJobStreamEvent(updatedJob); evErr == nil {
		store.AfterAuditCommit(r.Context(), func(context.Context) { a.hub.Publish(teamID, event) })
	}

	auth.WriteJSON(w, http.StatusOK, updatedJob)
}
