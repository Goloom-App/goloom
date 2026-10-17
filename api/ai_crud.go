package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/netguard"
)

func (a *API) handleUpsertTeamProfile(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.TeamProfile
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	if err := validateTeamProfile(input); err != nil {
		a.writeError(w, r, "invalid_team_profile", http.StatusBadRequest)
		return
	}

	var profile domain.TeamProfile
	var opErr error

	before, err := a.store.GetTeamProfile(r.Context(), teamID)
	switch {
	case err == nil:
		profile, opErr = a.store.UpdateTeamProfile(r.Context(), teamID, input)
	case errors.Is(err, domain.ErrNotFound):
		profile, opErr = a.store.CreateTeamProfile(r.Context(), teamID, input)
	default:
		a.writeStoreError(w, r, err, "")
		return
	}
	if opErr != nil {
		a.writeStoreError(w, r, opErr, "")
		return
	}
	if auditChanged(before, profile) {
		a.recordAudit(r, teamID, "brand_profile.update", "brand_profile", &profile.ID, "Updated brand profile")
	}
	auth.WriteJSON(w, http.StatusOK, profile)
}

func (a *API) handleGetTeamProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := a.store.GetTeamProfile(r.Context(), r.PathValue("teamID"))
	if err != nil {
		a.writeStoreError(w, r, err, "team_profile_not_found")
		return
	}
	auth.WriteJSON(w, http.StatusOK, profile)
}

func (a *API) handleDeleteTeamProfile(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	profile, err := a.store.GetTeamProfile(r.Context(), teamID)
	if errors.Is(err, domain.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	if err := a.store.DeleteTeamProfile(r.Context(), teamID); err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	a.recordAudit(r, teamID, "brand_profile.delete", "brand_profile", &profile.ID, "Deleted brand profile")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleCreateCampaignFormat(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.CampaignFormat
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if err := validateCampaignFormat(input); err != nil {
		a.writeError(w, r, "invalid_campaign_format", http.StatusBadRequest)
		return
	}
	format, err := a.store.CreateCampaignFormat(r.Context(), teamID, input)
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	a.recordAudit(r, teamID, "campaign_format.create", "campaign_format", &format.ID, "Created campaign format")
	auth.WriteJSON(w, http.StatusCreated, format)
}

func (a *API) handleListCampaignFormats(w http.ResponseWriter, r *http.Request) {
	formats, err := a.store.ListCampaignFormats(r.Context(), r.PathValue("teamID"))
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(formats)})
}

func (a *API) handleGetCampaignFormat(w http.ResponseWriter, r *http.Request) {
	format, err := a.store.GetCampaignFormatByID(r.Context(), r.PathValue("teamID"), r.PathValue("formatID"))
	if err != nil {
		a.writeStoreError(w, r, err, "campaign_format_not_found")
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
	if err := validateCampaignFormat(input); err != nil {
		a.writeError(w, r, "invalid_campaign_format", http.StatusBadRequest)
		return
	}
	before, err := a.store.GetCampaignFormatByID(r.Context(), teamID, formatID)
	if err != nil {
		a.writeStoreError(w, r, err, "campaign_format_not_found")
		return
	}
	format, err := a.store.UpdateCampaignFormat(r.Context(), teamID, formatID, input)
	if err != nil {
		a.writeStoreError(w, r, err, "campaign_format_not_found")
		return
	}
	if auditChanged(before, format) {
		a.recordAudit(r, teamID, "campaign_format.update", "campaign_format", &formatID, "Updated campaign format")
	}
	auth.WriteJSON(w, http.StatusOK, format)
}

func (a *API) handleDeleteCampaignFormat(w http.ResponseWriter, r *http.Request) {
	teamID, formatID := r.PathValue("teamID"), r.PathValue("formatID")
	_, err := a.store.GetCampaignFormatByID(r.Context(), teamID, formatID)
	if errors.Is(err, domain.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	if err := a.store.DeleteCampaignFormat(r.Context(), teamID, formatID); err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	a.recordAudit(r, teamID, "campaign_format.delete", "campaign_format", &formatID, "Deleted campaign format")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleCreateStyleExample(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.StyleExample
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if err := validateStyleExample(input); err != nil {
		a.writeError(w, r, "invalid_style_example", http.StatusBadRequest)
		return
	}
	example, err := a.store.CreateStyleExample(r.Context(), teamID, input)
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	a.recordAudit(r, teamID, "style_example.create", "style_example", &example.ID, "Created style example")
	auth.WriteJSON(w, http.StatusCreated, example)
}

func (a *API) handleListStyleExamples(w http.ResponseWriter, r *http.Request) {
	examples, err := a.store.ListStyleExamples(r.Context(), r.PathValue("teamID"))
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(examples)})
}

func (a *API) handleDeleteStyleExample(w http.ResponseWriter, r *http.Request) {
	teamID, exampleID := r.PathValue("teamID"), r.PathValue("exampleID")
	examples, err := a.store.ListStyleExamples(r.Context(), teamID)
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	found := false
	for _, example := range examples {
		if example.ID == exampleID {
			found = true
			break
		}
	}
	if !found {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := a.store.DeleteStyleExample(r.Context(), teamID, exampleID); err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	a.recordAudit(r, teamID, "style_example.delete", "style_example", &exampleID, "Deleted style example")
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
	a.recordAudit(r, teamID, "rss_feed.create", "rss_feed", &feed.ID, "Created RSS feed")
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
	if !reflect.DeepEqual(existing, merged) {
		a.recordAudit(r, teamID, "rss_feed.update", "rss_feed", &feedID, "Updated RSS feed")
	}
	auth.WriteJSON(w, http.StatusOK, feed)
}

func (a *API) handleDeleteRSSFeed(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	if _, err := a.store.GetRSSFeedConfigByID(r.Context(), r.PathValue("teamID"), feedID); err != nil {
		a.writeStoreError(w, r, err, "audit mutation")
		return
	}
	if err := a.store.DeleteRSSFeedConfig(r.Context(), r.PathValue("teamID"), r.PathValue("feedID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.recordAudit(r, r.PathValue("teamID"), "rss_feed.delete", "rss_feed", &feedID, "Deleted RSS feed")
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleGetAIServiceConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.store.GetAIServiceConfig(r.Context(), r.PathValue("teamID"))
	if err != nil {
		a.writeStoreError(w, r, err, "ai_service_config_not_found")
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
	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL != "" {
		if err := netguard.NewPolicy(a.config.AIAllowedPrivateHosts).CheckURL(r.Context(), baseURL); err != nil {
			a.log.WarnContext(r.Context(), "rejected AI base_url", "team_id", teamID, "error", err)
			a.writeError(w, r, "invalid_ai_base_url", http.StatusBadRequest)
			return
		}
	}
	a.auditedHandler(func(w http.ResponseWriter, r *http.Request) {
		apiKey := strings.TrimSpace(input.APIKey)
		// The stored key is only kept for the endpoint it was entered for: a new
		// provider or base_url must come with the key again, or the next LLM call
		// would hand the existing key to whatever host was just configured.
		if apiKey == "" {
			existing, err := a.store.GetAIServiceConfig(r.Context(), teamID)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				// No stored key that could leak.
			case err != nil:
				// Fail closed: the upsert would keep a key we could not check.
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			case existing.APIKey != "" &&
				(existing.Provider != provider || normalizeAIBaseURL(existing.BaseURL) != normalizeAIBaseURL(baseURL)):
				a.writeError(w, r, "ai_api_key_required", http.StatusBadRequest)
				return
			}
		}
		before, err := a.store.GetAIServiceConfig(r.Context(), teamID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			a.writeStoreError(w, r, err, "")
			return
		}
		cfg, err := a.store.UpsertAIServiceConfig(r.Context(), teamID, domain.AIServiceConfig{
			Provider:    provider,
			Model:       strings.TrimSpace(input.Model),
			BaseURL:     baseURL,
			Description: input.Description,
			APIKey:      apiKey,
		})
		if err != nil {
			a.writeStoreError(w, r, err, "")
			return
		}
		// Never include the API key in the audit summary.
		if auditChanged(before, cfg) || before.APIKey != cfg.APIKey {
			a.recordAudit(r, teamID, "ai_config.update", "ai_config", &cfg.ID, "Updated AI provider configuration")
		}
		auth.WriteJSON(w, http.StatusOK, cfg)
	})(w, r)
}

// normalizeAIBaseURL compares base URLs the way the LLM clients use them
// (they trim whitespace and trailing slashes before appending the path).
func normalizeAIBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func (a *API) handleGetProactiveSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.store.GetProactiveTriggerSettings(r.Context(), r.PathValue("teamID"))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			auth.WriteJSON(w, http.StatusOK, domain.ProactiveTriggerSettings{
				TeamID:                  r.PathValue("teamID"),
				ContentGapThresholdDays: 3,
				AutoFillEnabled:         false,
				MaxTriggersPerDay:       5,
				CronSchedule:            "0 * * * *",
			})
			return
		}
		a.writeStoreError(w, r, err, "")
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
	input.CronSchedule = strings.TrimSpace(input.CronSchedule)
	if err := validateProactiveSettings(input); err != nil {
		a.writeError(w, r, "invalid_proactive_settings", http.StatusBadRequest)
		return
	}
	before, err := a.store.GetProactiveTriggerSettings(r.Context(), teamID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		a.writeStoreError(w, r, err, "")
		return
	}
	settings, err := a.store.UpsertProactiveTriggerSettings(r.Context(), teamID, input)
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	if auditChanged(before, settings) {
		a.recordAudit(r, teamID, "proactive_settings.update", "proactive_settings", &settings.ID, "Updated proactive trigger settings")
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
