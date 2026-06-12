package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

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
	input.InitialSyncMode = domain.NormalizeRSSInitialSyncMode(string(input.InitialSyncMode))
	input.OutputMode = domain.NormalizeAutomationOutputMode(string(input.OutputMode))
	if strings.TrimSpace(input.ContentTemplate) == "" {
		input.ContentTemplate = domain.DefaultRSSContentTemplate
	}
	if strings.TrimSpace(input.TitleTemplate) == "" {
		input.TitleTemplate = domain.DefaultRSSTitleTemplate
	}
	if input.MaxPostsPerDay <= 0 {
		input.MaxPostsPerDay = 10
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

type rssFeedPatchRequest struct {
	FeedURL          *string    `json:"feed_url"`
	Name             *string    `json:"name"`
	IsActive         *bool      `json:"is_active"`
	ContentTemplate  *string    `json:"content_template"`
	TitleTemplate    *string    `json:"title_template"`
	TitleHint        *string    `json:"title_hint"`
	OutputMode       *string    `json:"output_mode"`
	MaxPostsPerDay   *int       `json:"max_posts_per_day"`
	AiEnhanceEnabled *bool      `json:"ai_enhance_enabled"`
	PromptHint       *string    `json:"prompt_hint"`
	TargetAccountIDs *[]string  `json:"target_account_ids"`
	Tonality         *string    `json:"tonality"`
	LastFetchedAt    *time.Time `json:"last_fetched_at"`
	InitialSyncMode  *string    `json:"initial_sync_mode"`
}

func (a *API) handleUpdateRSSFeed(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	feedID := r.PathValue("feedID")
	var patch rssFeedPatchRequest
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	existing, err := a.store.GetRSSFeedConfigByID(r.Context(), teamID, feedID)
	if err != nil {
		a.writeError(w, r, "rss_feed_not_found", http.StatusNotFound)
		return
	}

	merged := existing
	if patch.FeedURL != nil {
		merged.FeedURL = *patch.FeedURL
	}
	if patch.Name != nil {
		merged.Name = *patch.Name
	}
	if patch.IsActive != nil {
		merged.IsActive = *patch.IsActive
	}
	if patch.ContentTemplate != nil {
		merged.ContentTemplate = *patch.ContentTemplate
	}
	if patch.TitleTemplate != nil {
		merged.TitleTemplate = *patch.TitleTemplate
	}
	if patch.TitleHint != nil {
		merged.TitleHint = *patch.TitleHint
	}
	if patch.OutputMode != nil {
		merged.OutputMode = domain.NormalizeAutomationOutputMode(*patch.OutputMode)
	}
	if patch.MaxPostsPerDay != nil {
		merged.MaxPostsPerDay = *patch.MaxPostsPerDay
	}
	if patch.AiEnhanceEnabled != nil {
		merged.AiEnhanceEnabled = *patch.AiEnhanceEnabled
	}
	if patch.PromptHint != nil {
		merged.PromptHint = *patch.PromptHint
	}
	if patch.TargetAccountIDs != nil {
		merged.TargetAccountIDs = domain.NormalizeMediaIDs(*patch.TargetAccountIDs)
	}
	if patch.Tonality != nil {
		merged.Tonality = *patch.Tonality
	}
	if patch.LastFetchedAt != nil {
		t := patch.LastFetchedAt.UTC()
		merged.LastFetchedAt = &t
	}
	if patch.InitialSyncMode != nil {
		merged.InitialSyncMode = domain.NormalizeRSSInitialSyncMode(*patch.InitialSyncMode)
	}

	feed, err := a.store.UpdateRSSFeedConfig(r.Context(), teamID, feedID, merged)
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

type aiServiceConfigRequest struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	BaseURL     string `json:"base_url"`
	Description string `json:"description"`
	// APIKey is write-only: empty keeps the stored key.
	APIKey string `json:"api_key"`
}

func (a *API) handleUpsertAIServiceConfig(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input aiServiceConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	switch provider {
	case "", "openai", "anthropic":
	default:
		a.writeError(w, r, "invalid_ai_provider", http.StatusBadRequest)
		return
	}
	if provider == "" {
		provider = "openai"
	}
	cfg, err := a.store.UpsertAIServiceConfig(r.Context(), teamID, domain.AIServiceConfig{
		Provider:    provider,
		Model:       strings.TrimSpace(input.Model),
		BaseURL:     strings.TrimSpace(input.BaseURL),
		Description: input.Description,
		APIKey:      strings.TrimSpace(input.APIKey),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, cfg)
}

func (a *API) handleGetProactiveSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.store.GetProactiveTriggerSettings(r.Context(), r.PathValue("teamID"))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			auth.WriteJSON(w, http.StatusOK, domain.ProactiveTriggerSettings{
				TeamID:                  r.PathValue("teamID"),
				ContentGapThresholdDays: 3,
				AutoFillEnabled:         false,
				MaxTriggersPerDay:       5,
				CronSchedule:            "0 * * * *",
			})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
