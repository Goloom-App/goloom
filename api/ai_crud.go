package api

import (
	"encoding/json"
	"net/http"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) handleUpsertTeamProfile(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.TeamProfile
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	var profile domain.TeamProfile
	var opErr error

	_, err := a.store.GetTeamProfile(r.Context(), teamID)
	if err != nil {
		profile, opErr = a.store.CreateTeamProfile(r.Context(), teamID, input)
	} else {
		profile, opErr = a.store.UpdateTeamProfile(r.Context(), teamID, input)
	}
	if opErr != nil {
		http.Error(w, opErr.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, profile)
}

func (a *API) handleGetTeamProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := a.store.GetTeamProfile(r.Context(), r.PathValue("teamID"))
	if err != nil {
		a.writeError(w, r, "profile_not_found", http.StatusNotFound)
		return
	}
	auth.WriteJSON(w, http.StatusOK, profile)
}

func (a *API) handleDeleteTeamProfile(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteTeamProfile(r.Context(), r.PathValue("teamID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleCreateCampaignFormat(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.CampaignFormat
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	format, err := a.store.CreateCampaignFormat(r.Context(), teamID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, format)
}

func (a *API) handleListCampaignFormats(w http.ResponseWriter, r *http.Request) {
	formats, err := a.store.ListCampaignFormats(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(formats)})
}

func (a *API) handleGetCampaignFormat(w http.ResponseWriter, r *http.Request) {
	format, err := a.store.GetCampaignFormatByID(r.Context(), r.PathValue("teamID"), r.PathValue("formatID"))
	if err != nil {
		a.writeError(w, r, "format_not_found", http.StatusNotFound)
		return
	}
	auth.WriteJSON(w, http.StatusOK, format)
}

func (a *API) handleUpdateCampaignFormat(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	formatID := r.PathValue("formatID")
	var input domain.CampaignFormat
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	format, err := a.store.UpdateCampaignFormat(r.Context(), teamID, formatID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, format)
}

func (a *API) handleDeleteCampaignFormat(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteCampaignFormat(r.Context(), r.PathValue("teamID"), r.PathValue("formatID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleCreateStyleExample(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.StyleExample
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	example, err := a.store.CreateStyleExample(r.Context(), teamID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, example)
}

func (a *API) handleListStyleExamples(w http.ResponseWriter, r *http.Request) {
	examples, err := a.store.ListStyleExamples(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(examples)})
}

func (a *API) handleDeleteStyleExample(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteStyleExample(r.Context(), r.PathValue("teamID"), r.PathValue("exampleID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleCreateRSSFeed(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.RSSFeedConfig
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	feed, err := a.store.CreateRSSFeedConfig(r.Context(), teamID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, feed)
}

func (a *API) handleListRSSFeeds(w http.ResponseWriter, r *http.Request) {
	feeds, err := a.store.ListRSSFeedConfigs(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(feeds)})
}

func (a *API) handleUpdateRSSFeed(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	feedID := r.PathValue("feedID")
	var input domain.RSSFeedConfig
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	feed, err := a.store.UpdateRSSFeedConfig(r.Context(), teamID, feedID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, feed)
}

func (a *API) handleDeleteRSSFeed(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteRSSFeedConfig(r.Context(), r.PathValue("teamID"), r.PathValue("feedID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleGetAIServiceConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.store.GetAIServiceConfig(r.Context(), r.PathValue("teamID"))
	if err != nil {
		a.writeError(w, r, "ai_service_config_not_found", http.StatusNotFound)
		return
	}
	auth.WriteJSON(w, http.StatusOK, cfg)
}

func (a *API) handleUpsertAIServiceConfig(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.AIServiceConfig
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	cfg, err := a.store.UpsertAIServiceConfig(r.Context(), teamID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, cfg)
}

func (a *API) handleGetProactiveSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.store.GetProactiveTriggerSettings(r.Context(), r.PathValue("teamID"))
	if err != nil {
		a.writeError(w, r, "proactive_settings_not_found", http.StatusNotFound)
		return
	}
	auth.WriteJSON(w, http.StatusOK, settings)
}

func (a *API) handleUpsertProactiveSettings(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.ProactiveTriggerSettings
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	settings, err := a.store.UpsertProactiveTriggerSettings(r.Context(), teamID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, settings)
}

func (a *API) handleAdminListAIEnabledTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := a.store.ListAIEnabledTeams(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(teams)})
}
