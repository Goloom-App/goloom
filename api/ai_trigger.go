package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

const aiJobListLimit = 20

type aiTriggerRequest struct {
	Type        domain.AIJobType `json:"type"`
	Params      json.RawMessage  `json:"params"`
	CanSchedule json.RawMessage  `json:"_can_schedule"`
	Preview     json.RawMessage  `json:"_preview"`
}

type aiTriggerResponse struct {
	JobID  string             `json:"job_id"`
	Status domain.AIJobStatus `json:"status"`
}

func (a *API) handleAITrigger(w http.ResponseWriter, r *http.Request) {
	var input aiTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if !isValidAIJobType(input.Type) {
		a.writeError(w, r, "invalid_ai_job_type", http.StatusBadRequest)
		return
	}
	// Authority and automation metadata belong to the server. User-supplied
	// automation keys could otherwise bypass the normal completion scope check.
	var params map[string]json.RawMessage
	if err := json.Unmarshal(ensureJSONObject(input.Params), &params); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if input.CanSchedule != nil || input.Preview != nil {
		a.writeError(w, r, "ai_job_params_reserved", http.StatusBadRequest)
		return
	}
	for key := range params {
		if strings.EqualFold(key, "_can_schedule") || strings.EqualFold(key, "_preview") || strings.EqualFold(key, "rss_automation") || strings.EqualFold(key, "recurring_automation") {
			a.log.WarnContext(r.Context(), "ai trigger: reserved automation param rejected", "team_id", r.PathValue("teamID"), "param", key)
			a.writeError(w, r, "ai_job_params_reserved", http.StatusBadRequest)
			return
		}
	}

	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	enrichedParams, err := enrichAIJobParams(r.Context(), ensureJSONObject(input.Params))
	if err != nil {
		a.writeError(w, r, err.Error(), http.StatusBadRequest)
		return
	}

	payload, err := json.Marshal(struct {
		Params      json.RawMessage `json:"params"`
		CanSchedule bool            `json:"_can_schedule"`
	}{Params: enrichedParams, CanSchedule: auth.PrincipalAllows(principal, auth.ScopeWriteSchedule)})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	job, err := a.jobManager.SubmitJob(r.Context(), domain.AIJob{
		TeamID:       r.PathValue("teamID"),
		AuthorUserID: principal.User.ID,
		Type:         input.Type,
		Payload:      payload,
	})
	if err != nil {
		if errors.Is(err, aijobs.ErrAIServiceNotConfigured) {
			a.writeError(w, r, "ai_service_not_configured", http.StatusUnprocessableEntity)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	auth.WriteJSON(w, http.StatusAccepted, aiTriggerResponse{JobID: job.ID, Status: job.Status})
}

func (a *API) handleListAIJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := a.store.ListAIJobs(r.Context(), r.PathValue("teamID"), aiJobListLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(jobs)})
}

func (a *API) handleGetAIJob(w http.ResponseWriter, r *http.Request) {
	job, err := a.store.GetAIJobByID(r.Context(), r.PathValue("teamID"), r.PathValue("jobID"))
	if err != nil {
		a.writeError(w, r, "ai_job_not_found", http.StatusNotFound)
		return
	}
	auth.WriteJSON(w, http.StatusOK, job)
}

func isValidAIJobType(jobType domain.AIJobType) bool {
	switch jobType {
	case domain.AIJobTypeVoiceEngine, domain.AIJobTypeCampaignAutopilot, domain.AIJobTypeProactiveTrigger, domain.AIJobTypeProfileAnalysis, domain.AIJobTypeVibePreview, domain.AIJobTypeProfileAssistant:
		return true
	default:
		return false
	}
}

func ensureJSONObject(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}
