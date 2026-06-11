package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/scheduler"
)

func (a *API) handleTeamEngagementHourHistogram(w http.ResponseWriter, r *http.Request) {
	teamID := strings.TrimSpace(r.PathValue("teamID"))
	days := 90
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 366 {
			days = n
		}
	}
	items, err := a.store.GetTeamEngagementHourHistogram(r.Context(), teamID, days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"hours": sliceOrEmpty(items)})
}

func (a *API) handleListPostTemplates(w http.ResponseWriter, r *http.Request) {
	teamID := strings.TrimSpace(r.PathValue("teamID"))
	items, err := a.store.ListPostTemplates(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleCreatePostTemplate(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	teamID := strings.TrimSpace(r.PathValue("teamID"))
	var input domain.CreatePostTemplateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Content = strings.TrimSpace(input.Content)
	input.Visibility = domain.NormalizePostVisibility(input.Visibility)
	input.MediaIDs = domain.NormalizeMediaIDs(input.MediaIDs)
	input.MediaExcludeByAccount = domain.NormalizeMediaExcludeByAccount(input.MediaExcludeByAccount, input.MediaIDs)
	input.TargetAccountIDs = domain.NormalizeMediaIDs(input.TargetAccountIDs)
	if input.Content == "" || strings.TrimSpace(input.RecurrenceJSON) == "" || len(input.TargetAccountIDs) == 0 {
		a.writeError(w, r, "content_recurrence_targets_required", http.StatusBadRequest)
		return
	}
	accs, err := a.store.GetAccountsByIDs(r.Context(), teamID, input.TargetAccountIDs)
	if err != nil || len(accs) != len(input.TargetAccountIDs) {
		a.writeError(w, r, "invalid_target_account_ids", http.StatusBadRequest)
		return
	}
	tmpl, err := a.store.CreatePostTemplate(r.Context(), teamID, principal, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, tmpl)
}

func (a *API) handleUpdatePostTemplate(w http.ResponseWriter, r *http.Request) {
	teamID := strings.TrimSpace(r.PathValue("teamID"))
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	var input domain.UpdatePostTemplateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if input.Title != nil {
		t := strings.TrimSpace(*input.Title)
		input.Title = &t
	}
	if input.Content != nil {
		c := strings.TrimSpace(*input.Content)
		input.Content = &c
	}
	if input.Visibility != nil {
		v := domain.NormalizePostVisibility(*input.Visibility)
		input.Visibility = &v
	}
	if input.MediaIDs != nil {
		n := domain.NormalizeMediaIDs(*input.MediaIDs)
		input.MediaIDs = &n
	}
	if input.MediaExcludeByAccount != nil {
		mids := input.MediaIDs
		var mediaIDs []string
		if mids != nil {
			mediaIDs = *mids
		}
		input.MediaExcludeByAccount = domain.NormalizeMediaExcludeByAccount(input.MediaExcludeByAccount, mediaIDs)
	}
	if input.TargetAccountIDs != nil {
		tg := domain.NormalizeMediaIDs(*input.TargetAccountIDs)
		input.TargetAccountIDs = &tg
		accs, err := a.store.GetAccountsByIDs(r.Context(), teamID, tg)
		if err != nil || len(accs) != len(tg) {
			a.writeError(w, r, "invalid_target_account_ids", http.StatusBadRequest)
			return
		}
	}
	tmpl, err := a.store.UpdatePostTemplate(r.Context(), teamID, templateID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	auth.WriteJSON(w, http.StatusOK, tmpl)
}

func (a *API) handleListPostTemplateLinkedPosts(w http.ResponseWriter, r *http.Request) {
	teamID := strings.TrimSpace(r.PathValue("teamID"))
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	items, err := a.store.ListPostTemplateLinkedPosts(r.Context(), teamID, templateID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleDeletePostTemplate(w http.ResponseWriter, r *http.Request) {
	teamID := strings.TrimSpace(r.PathValue("teamID"))
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	if err := a.store.DeletePostTemplate(r.Context(), teamID, templateID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type skipTemplateBody struct {
	OccurrenceAt time.Time `json:"occurrence_at"`
	ShiftTo      time.Time `json:"shift_to,omitempty"`
}

func (a *API) handleSkipPostTemplateOccurrence(w http.ResponseWriter, r *http.Request) {
	teamID := strings.TrimSpace(r.PathValue("teamID"))
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	var body skipTemplateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if body.OccurrenceAt.IsZero() {
		a.writeError(w, r, "occurrence_at_required", http.StatusBadRequest)
		return
	}
	if !body.ShiftTo.IsZero() {
		if err := a.store.ShiftPostTemplateOccurrence(r.Context(), teamID, templateID, body.OccurrenceAt.UTC(), body.ShiftTo.UTC()); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if err := a.store.AddPostTemplateSkip(r.Context(), teamID, templateID, body.OccurrenceAt.UTC()); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

type regenerateTemplateBody struct {
	Mode         string    `json:"mode"`
	OccurrenceAt time.Time `json:"occurrence_at,omitempty"`
}

func (a *API) handleRegeneratePostTemplate(w http.ResponseWriter, r *http.Request) {
	teamID := strings.TrimSpace(r.PathValue("teamID"))
	templateID := strings.TrimSpace(r.PathValue("templateID"))
	if a.metricsSync == nil {
		a.writeError(w, r, "template_regenerate_not_available", http.StatusServiceUnavailable)
		return
	}
	var body regenerateTemplateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	mode := strings.TrimSpace(strings.ToLower(body.Mode))
	var (
		result domain.PostTemplateRegenerateResult
		err    error
	)
	switch mode {
	case "occurrence":
		if body.OccurrenceAt.IsZero() {
			a.writeError(w, r, "occurrence_at_required", http.StatusBadRequest)
			return
		}
		result, err = a.metricsSync.RegeneratePostTemplateOccurrence(r.Context(), teamID, templateID, body.OccurrenceAt.UTC())
	case "horizon":
		result, err = a.metricsSync.RegeneratePostTemplateHorizon(r.Context(), teamID, templateID)
	default:
		a.writeError(w, r, "invalid_regenerate_mode", http.StatusBadRequest)
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, scheduler.ErrRegenerateBlocked):
			a.writeError(w, r, "regenerate_blocked_posted", http.StatusConflict)
		case errors.Is(err, scheduler.ErrRegenerateNoPosts):
			a.writeError(w, r, "regenerate_no_posts", http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
	auth.WriteJSON(w, http.StatusOK, result)
}
