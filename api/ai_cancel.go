package api

import (
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

	if err := a.store.UpdateAIJobStatus(r.Context(), job.ID, domain.AIJobStatusFailed, nil, aiJobCancelledMessage); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	updatedJob, err := a.store.GetAIJobByID(r.Context(), teamID, jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if event, evErr := aiJobStreamEvent(updatedJob); evErr == nil {
		a.hub.Publish(teamID, event)
	}

	auth.WriteJSON(w, http.StatusOK, updatedJob)
}
