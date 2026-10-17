package api

import (
	"context"
	"git.f4mily.net/goloom/internal/store"
	"net/http"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
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

	switch job.Status {
	case domain.AIJobStatusCompleted:
		a.writeError(w, r, "ai_job_not_cancellable", http.StatusConflict)
		return
	case domain.AIJobStatusFailed:
		auth.WriteJSON(w, http.StatusOK, job)
		return
	}

	claimed, err := a.store.ClaimAIJobCompletion(r.Context(), job.ID, domain.AIJobStatusFailed, nil, aiJobCancelledMessage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !claimed {
		latest, err := a.store.GetAIJobByID(r.Context(), teamID, jobID)
		if err != nil {
			a.writeError(w, r, "internal_error", http.StatusInternalServerError)
			return
		}
		if latest.Status == domain.AIJobStatusCompleted {
			a.writeError(w, r, "ai_job_not_cancellable", http.StatusConflict)
			return
		}
		auth.WriteJSON(w, http.StatusOK, latest)
		return
	}
	a.recordAudit(r, teamID, "ai_job.cancel", "ai_job", &jobID, "Cancelled AI job", map[string]string{"job_id": jobID})
	// Abort the in-process LLM call (no-op if the job is not running here).
	store.AfterAuditCommit(r.Context(), func(context.Context) { a.jobManager.Cancel(job.ID) })

	updatedJob, err := a.store.GetAIJobByID(r.Context(), teamID, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if event, evErr := aiJobStreamEvent(updatedJob); evErr == nil {
		store.AfterAuditCommit(r.Context(), func(context.Context) { a.hub.Publish(teamID, event) })
	}

	auth.WriteJSON(w, http.StatusOK, updatedJob)
}
