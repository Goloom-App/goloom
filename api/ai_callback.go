package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

type aiCallbackRequest struct {
	JobID        string             `json:"job_id"`
	Status       domain.AIJobStatus `json:"status"`
	Result       json.RawMessage    `json:"result"`
	ErrorMessage string             `json:"error_message"`
}

type aiCallbackResult struct {
	Content                string            `json:"content"`
	ScheduledAt            *time.Time        `json:"scheduled_at"`
	SuggestedScheduledAt   *time.Time        `json:"suggested_scheduled_at"`
	AccountContentOverride map[string]string `json:"account_content_override"`
}

func (a *API) handleAICallback(w http.ResponseWriter, r *http.Request) {
	var input aiCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(input.JobID) == "" {
		a.writeError(w, r, "job_id_required", http.StatusBadRequest)
		return
	}
	if input.Status != domain.AIJobStatusCompleted && input.Status != domain.AIJobStatusFailed {
		a.writeError(w, r, "invalid_status", http.StatusBadRequest)
		return
	}

	job, err := a.store.GetAIJobByIDGlobal(r.Context(), input.JobID)
	if err != nil {
		a.writeError(w, r, "ai_job_not_found", http.StatusNotFound)
		return
	}

	if job.Status == domain.AIJobStatusCompleted || job.Status == domain.AIJobStatusFailed {
		auth.WriteJSON(w, http.StatusOK, map[string]bool{"acknowledged": true})
		return
	}

	var resultBytes []byte
	if len(input.Result) > 0 {
		resultBytes = []byte(input.Result)
	}
	if err := a.store.UpdateAIJobStatus(r.Context(), job.ID, input.Status, resultBytes, input.ErrorMessage); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	updatedJob, err := a.store.GetAIJobByIDGlobal(r.Context(), job.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if input.Status == domain.AIJobStatusCompleted {
		if meta := parseRSSAutomationMeta(job.Payload); meta != nil && job.Type == domain.AIJobTypeVoiceEngine {
			a.finishRSSAutomationFromAI(r, job, input.Result, meta)
		} else if meta := parseRecurringAutomationMeta(job.Payload); meta != nil && job.Type == domain.AIJobTypeVoiceEngine {
			a.finishRecurringAutomationFromAI(r, job, input.Result, meta)
		} else {
			profile, profErr := a.store.GetTeamProfile(r.Context(), job.TeamID)
			if profErr == nil && profile.AutoPublishEnabled {
				a.autoCreatePostFromCallbackResult(r, job, input.Result)
			}
		}
	} else if input.Status == domain.AIJobStatusFailed {
		if meta := parseRSSAutomationMeta(job.Payload); meta != nil && job.Type == domain.AIJobTypeVoiceEngine {
			a.finishRSSAutomationFallback(r, job, meta)
		}
	}

	if event, evErr := aiJobStreamEvent(updatedJob); evErr == nil {
		a.hub.Publish(job.TeamID, event)
	}

	auth.WriteJSON(w, http.StatusOK, map[string]bool{"acknowledged": true})
}

func (a *API) autoCreateDraftFromCallbackResult(r *http.Request, job domain.AIJob, rawResult json.RawMessage) {
	var res aiCallbackResult
	if len(rawResult) > 0 {
		_ = json.Unmarshal(rawResult, &res)
	}

	content := strings.TrimSpace(res.Content)
	if content == "" {
		return
	}

	targetAccounts := targetAccountIDsFromJobPayload(job.Payload)
	if len(targetAccounts) == 0 {
		return
	}
	overrides := domain.NormalizeAccountContentOverride(res.AccountContentOverride, targetAccounts)

	principal := domain.AuthenticatedPrincipal{
		User: domain.User{ID: job.AuthorUserID},
		Kind: "api_token",
	}

	_, _ = a.store.CreateScheduledPost(r.Context(), job.TeamID, principal, domain.CreatePostInput{
		Content:                content,
		TargetAccounts:         targetAccounts,
		AccountContentOverride: overrides,
		ScheduledAt:            time.Now().UTC(),
		Draft:                  true,
		AuthorUserID:           &job.AuthorUserID,
		UseVersions:            len(overrides) > 0,
	})
}

func (a *API) autoCreatePostFromCallbackResult(r *http.Request, job domain.AIJob, rawResult json.RawMessage) {
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

	_, _ = a.store.CreateScheduledPost(r.Context(), job.TeamID, principal, domain.CreatePostInput{
		Content:                content,
		TargetAccounts:         targetAccounts,
		AccountContentOverride: overrides,
		ScheduledAt:            scheduledAt,
		Draft:                  false,
		AuthorUserID:           &job.AuthorUserID,
		UseVersions:            len(overrides) > 0,
	})
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
