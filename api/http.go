package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/i18n"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/sse"
	"git.f4mily.net/goloom/internal/store"
)

type API struct {
	log         *slog.Logger
	store       store.Store
	auth        *auth.Service
	providers   *provider.Registry
	config      config.Config
	metricsSync metricsSyncRunner
	i18n        *i18n.Catalog
	jobManager  *aijobs.Manager
	hub         *sse.Hub
}

type validationResponse struct {
	MaxChars      int               `json:"max_chars"`
	ContentLength int               `json:"content_length"`
	Valid         bool              `json:"valid"`
	Destinations  []destinationInfo `json:"destinations"`
}

type destinationInfo struct {
	AccountID string `json:"account_id"`
	Provider  string `json:"provider"`
	MaxChars  int    `json:"max_chars"`
	Length    int    `json:"length"`
	Valid     bool   `json:"valid"`
}

func New(logger *slog.Logger, store store.Store, authService *auth.Service, providers *provider.Registry, cfg config.Config, metricsSync metricsSyncRunner, catalog *i18n.Catalog, jobManager *aijobs.Manager, hub *sse.Hub) *API {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if jobManager == nil {
		jobManager = aijobs.NewManager(store, nil)
	}
	if hub == nil {
		hub = sse.NewHub()
	}

	api := &API{
		log:         logger,
		store:       store,
		auth:        authService,
		providers:   providers,
		config:      cfg,
		metricsSync: metricsSync,
		i18n:        catalog,
		jobManager:  jobManager,
		hub:         hub,
	}
	// AI jobs execute in-process; the API applies their completion side effects.
	jobManager.SetCompleter(api)
	return api
}

func (a *API) Handler(limiter *security.Limiter, allowedOrigins []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.handleHealth)
	mux.HandleFunc("GET /v1/discovery", a.handleDiscovery)
	mux.HandleFunc("GET /v1/providers", a.handleProviders)
	mux.HandleFunc("GET /v1/auth/status", a.handleAuthStatus)
	mux.HandleFunc("POST /v1/auth/oidc/start", a.handleStartOIDCLogin)
	mux.HandleFunc("GET /v1/oauth/oidc/callback", a.handleOIDCLoginCallback)
	mux.HandleFunc("GET /v1/oauth/mastodon/callback", a.handleMastodonOAuthCallback)

	mux.Handle("GET /v1/me", a.auth.RequireAuth(http.HandlerFunc(a.handleMe)))
	mux.Handle("GET /v1/me/api-tokens", a.auth.RequireAuth(http.HandlerFunc(a.handleListMyAPITokens)))
	mux.Handle("POST /v1/me/api-tokens", a.auth.RequireAuth(http.HandlerFunc(a.handleCreateMyAPIToken)))
	mux.Handle("DELETE /v1/me/api-tokens/{tokenID}", a.auth.RequireAuth(http.HandlerFunc(a.handleRevokeMyAPIToken)))
	mux.Handle("GET /v1/users", a.auth.RequireAuth(http.HandlerFunc(a.handleListUsers)))
	mux.Handle("GET /v1/teams", a.auth.RequireAuth(http.HandlerFunc(a.handleListTeams)))
	mux.Handle("POST /v1/teams", a.auth.RequireAuth(http.HandlerFunc(a.handleCreateTeam)))
	mux.Handle("PATCH /v1/teams/{teamID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleOwner)(http.HandlerFunc(a.handleUpdateTeam))))
	mux.Handle("GET /v1/admin/users", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleListUsers))))
	mux.Handle("PATCH /v1/admin/users/{userID}", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleUpdateUser))))
	mux.Handle("GET /v1/admin/runtime-config", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleRuntimeConfig))))
	mux.Handle("GET /v1/admin/metrics", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleAdminMetrics))))
	mux.Handle("GET /v1/admin/sync-status", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleAdminSyncStatus))))
	mux.Handle("POST /v1/admin/sync-metrics", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleAdminSyncMetrics))))
	mux.Handle("POST /v1/admin/sync-external-posts", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleAdminSyncExternalPosts))))
	mux.Handle("POST /v1/admin/sync-rss-feeds", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleAdminSyncRSSFeeds))))
	mux.Handle("POST /v1/admin/e2e/automation-draft", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleAdminSeedAutomationDraft))))
	mux.Handle("POST /v1/admin/repair-future-posted", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleAdminRepairFuturePosted))))
	mux.Handle("GET /v1/admin/logs", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleListLogEntries))))
	mux.Handle("POST /v1/admin/logs/{id}/archive", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleArchiveLogEntry))))
	mux.Handle("POST /v1/admin/logs/{id}/unarchive", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleUnarchiveLogEntry))))
	mux.Handle("DELETE /v1/admin/logs/{id}", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleDeleteLogEntry))))
	mux.Handle("POST /v1/admin/logs/prune", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handlePruneLogEntries))))
	mux.Handle("GET /v1/provider-instances", a.auth.RequireAuth(http.HandlerFunc(a.handleListProviderInstances)))
	mux.Handle("GET /v1/provider-instances/{instanceID}", a.auth.RequireAuth(http.HandlerFunc(a.handleGetProviderInstance)))
	mux.Handle("GET /v1/admin/provider-instances", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleListProviderInstances))))
	mux.Handle("POST /v1/admin/provider-instances", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleCreateProviderInstance))))
	mux.Handle("PUT /v1/admin/provider-instances/{instanceID}", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleUpdateProviderInstance))))
	mux.Handle("DELETE /v1/admin/provider-instances/{instanceID}", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleDeleteProviderInstance))))
	mux.Handle("GET /v1/teams/{teamID}/members", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListTeamMembers))))
	mux.Handle("POST /v1/teams/{teamID}/members", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleOwner)(http.HandlerFunc(a.handleAddTeamMember))))
	mux.Handle("DELETE /v1/teams/{teamID}/members/{userID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleOwner)(http.HandlerFunc(a.handleRemoveTeamMember))))
	mux.Handle("POST /v1/teams/{teamID}/invitations", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleOwner)(http.HandlerFunc(a.handleCreateTeamInvitation))))
	mux.Handle("POST /v1/invitations/accept", a.auth.RequireAuth(http.HandlerFunc(a.handleAcceptTeamInvitation)))
	mux.Handle("GET /v1/teams/{teamID}/accounts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListAccounts))))
	mux.Handle("POST /v1/teams/{teamID}/accounts/oauth/mastodon/start", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleStartMastodonOAuth))))
	mux.Handle("POST /v1/teams/{teamID}/accounts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCreateAccount))))
	mux.Handle("GET /v1/teams/{teamID}/media", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleTeamMediaList))))
	mux.Handle("POST /v1/teams/{teamID}/media", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleTeamMediaUploadToLibrary))))
	mux.Handle("DELETE /v1/teams/{teamID}/media/{mediaID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleTeamMediaDelete))))
	mux.Handle("GET /v1/teams/{teamID}/media/{mediaID}/preview", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleTeamMediaPreview))))
	mux.Handle("POST /v1/teams/{teamID}/accounts/{accountID}/migrate", a.auth.RequireAuth(http.HandlerFunc(a.handleMigrateAccount)))
	mux.Handle("PATCH /v1/teams/{teamID}/accounts/{accountID}", a.auth.RequireAuth(http.HandlerFunc(a.handleUpdateAccount)))
	mux.Handle("DELETE /v1/teams/{teamID}/accounts/{accountID}", a.auth.RequireAuth(http.HandlerFunc(a.handleDeleteAccount)))
	mux.Handle("GET /v1/teams/{teamID}/posts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListPosts))))
	mux.Handle("GET /v1/teams/{teamID}/review-queue", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListReviewQueue))))
	mux.Handle("POST /v1/teams/{teamID}/posts", a.auth.RequireAuth(http.HandlerFunc(a.handleCreatePost)))
	mux.Handle("POST /v1/teams/{teamID}/posts/validate", a.auth.RequireAuth(http.HandlerFunc(a.handleValidatePost)))
	mux.Handle("GET /v1/teams/{teamID}/posts/{postID}", a.auth.RequireAuth(http.HandlerFunc(a.handleGetPost)))
	mux.Handle("PATCH /v1/teams/{teamID}/posts/{postID}", a.auth.RequireAuth(http.HandlerFunc(a.handleUpdatePost)))
	mux.Handle("DELETE /v1/teams/{teamID}/posts/{postID}", a.auth.RequireAuth(http.HandlerFunc(a.handleDeletePost)))
	mux.Handle("POST /v1/teams/{teamID}/posts/{postID}/cancel", a.auth.RequireAuth(http.HandlerFunc(a.handleCancelPost)))
	mux.Handle("GET /v1/teams/{teamID}/analytics/summary", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleTeamAnalyticsSummary))))
	mux.Handle("GET /v1/teams/{teamID}/analytics/posts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleTeamAnalyticsPosts))))
	mux.Handle("GET /v1/teams/{teamID}/analytics/chart", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleTeamAnalyticsChart))))
	mux.Handle("GET /v1/teams/{teamID}/analytics/account/{accountID}/growth", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleTeamAccountGrowth))))
	mux.Handle("GET /v1/teams/{teamID}/analytics", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleTeamAnalytics))))
	mux.Handle("GET /v1/teams/{teamID}/posts/{postID}/analytics", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handlePostAnalytics))))
	mux.Handle("GET /v1/teams/{teamID}/posts/{postID}/versions", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListPostVersions))))
	mux.Handle("GET /v1/teams/{teamID}/versions", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListAllTeamPostVersions))))
	mux.Handle("PATCH /v1/teams/{teamID}/posts/{postID}/versions", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handlePatchPostVersions))))
	mux.Handle("GET /v1/teams/{teamID}/analytics/engagement-hours", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleTeamEngagementHourHistogram))))
	mux.Handle("GET /v1/teams/{teamID}/external-post-monitor", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleGetExternalPostMonitor))))
	mux.Handle("PUT /v1/teams/{teamID}/external-post-monitor", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleOwner)(http.HandlerFunc(a.handleUpsertExternalPostMonitor))))
	mux.Handle("POST /v1/teams/{teamID}/import-old-posts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleImportOldPosts))))
	mux.Handle("GET /v1/teams/{teamID}/post-templates", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListPostTemplates))))
	mux.Handle("GET /v1/teams/{teamID}/post-templates/{templateID}/linked-posts", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListPostTemplateLinkedPosts))))
	mux.Handle("POST /v1/teams/{teamID}/post-templates", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCreatePostTemplate))))
	mux.Handle("PATCH /v1/teams/{teamID}/post-templates/{templateID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleUpdatePostTemplate))))
	mux.Handle("DELETE /v1/teams/{teamID}/post-templates/{templateID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleDeletePostTemplate))))
	mux.Handle("POST /v1/teams/{teamID}/post-templates/{templateID}/skip", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleSkipPostTemplateOccurrence))))
	mux.Handle("POST /v1/teams/{teamID}/post-templates/{templateID}/regenerate", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleRegeneratePostTemplate))))
	mux.Handle("PUT /v1/teams/{teamID}/profile", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleUpsertTeamProfile)))))
	mux.Handle("GET /v1/teams/{teamID}/profile", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleGetTeamProfile)))))
	mux.Handle("DELETE /v1/teams/{teamID}/profile", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleDeleteTeamProfile)))))
	mux.Handle("POST /v1/teams/{teamID}/campaign-formats", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCreateCampaignFormat)))))
	mux.Handle("GET /v1/teams/{teamID}/campaign-formats", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListCampaignFormats)))))
	mux.Handle("GET /v1/teams/{teamID}/campaign-formats/{formatID}", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleGetCampaignFormat)))))
	mux.Handle("PATCH /v1/teams/{teamID}/campaign-formats/{formatID}", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleUpdateCampaignFormat)))))
	mux.Handle("DELETE /v1/teams/{teamID}/campaign-formats/{formatID}", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleDeleteCampaignFormat)))))
	mux.Handle("POST /v1/teams/{teamID}/style-examples", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCreateStyleExample)))))
	mux.Handle("GET /v1/teams/{teamID}/style-examples", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListStyleExamples)))))
	mux.Handle("DELETE /v1/teams/{teamID}/style-examples/{exampleID}", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleDeleteStyleExample)))))
	mux.Handle("POST /v1/teams/{teamID}/knowledge-sources", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCreateKnowledgeSource)))))
	mux.Handle("GET /v1/teams/{teamID}/knowledge-sources", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListKnowledgeSources)))))
	mux.Handle("PATCH /v1/teams/{teamID}/knowledge-sources/{sourceID}", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleUpdateKnowledgeSource)))))
	mux.Handle("DELETE /v1/teams/{teamID}/knowledge-sources/{sourceID}", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleDeleteKnowledgeSource)))))
	mux.Handle("POST /v1/teams/{teamID}/ai/prompt-preview", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleAIPromptPreview)))))
	mux.Handle("POST /v1/teams/{teamID}/rss-feeds", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCreateRSSFeed))))
	mux.Handle("GET /v1/teams/{teamID}/rss-feeds", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListRSSFeeds))))
	mux.Handle("PATCH /v1/teams/{teamID}/rss-feeds/{feedID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleUpdateRSSFeed))))
	mux.Handle("DELETE /v1/teams/{teamID}/rss-feeds/{feedID}", a.auth.RequireAuth(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleDeleteRSSFeed))))
	mux.Handle("GET /v1/teams/{teamID}/ai-service-config", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleGetAIServiceConfig)))))
	mux.Handle("PUT /v1/teams/{teamID}/ai-service-config", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleUpsertAIServiceConfig)))))
	mux.Handle("GET /v1/teams/{teamID}/ai-context", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(auth.RequireScope(auth.ScopeAIReadContext)(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleGetAIContext))))))
	mux.Handle("POST /v1/teams/{teamID}/posts/draft", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(auth.RequireScope(auth.ScopeAIWriteDrafts)(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCreateAIDraft))))))
	mux.Handle("POST /v1/teams/{teamID}/ai-trigger", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(auth.RequireScope(auth.ScopeAITriggerJobs)(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleAITrigger))))))
	mux.Handle("POST /v1/teams/{teamID}/ai/chat", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(auth.RequireScope(auth.ScopeAIChat)(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleAIChat))))))
	mux.Handle("GET /v1/teams/{teamID}/ai-jobs", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleListAIJobs)))))
	mux.Handle("GET /v1/teams/{teamID}/ai-jobs/{jobID}", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleGetAIJob)))))
	mux.Handle("POST /v1/teams/{teamID}/ai-jobs/{jobID}/cancel", a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(auth.RequireScope(auth.ScopeAITriggerJobs)(a.auth.RequireTeamRole("teamID", domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleCancelAIJob))))))
	mux.Handle("GET /v1/teams/{teamID}/ai-jobs/stream", a.auth.AcceptQueryToken("token")(a.auth.RequireAuth(a.auth.RequireAIEnabled("teamID")(a.auth.RequireTeamRole("teamID", domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)(http.HandlerFunc(a.handleAIJobStream))))))
	mux.Handle("GET /v1/admin/ai-enabled-teams", a.auth.RequireAuth(a.auth.RequireAdmin(http.HandlerFunc(a.handleAdminListAIEnabledTeams))))

	chain := security.CORSMiddleware(allowedOrigins)(limiter.Middleware(mux))
	if a.log != nil {
		chain = httpDebugRequestLogger(a.log, chain)
	}
	return chain
}

// httpDebugRequestLogger logs each request at Debug when enabled (skip noisy health checks).
func httpDebugRequestLogger(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if !log.Enabled(r.Context(), slog.LevelDebug) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Debug("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	auth.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleProviders(w http.ResponseWriter, _ *http.Request) {
	auth.WriteJSON(w, http.StatusOK, map[string]any{"providers": sliceOrEmpty(a.providers.Supported())})
}

func (a *API) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hasAdminUsers := false
	for _, user := range users {
		if user.IsAdmin {
			hasAdminUsers = true
			break
		}
	}

	initialSetup := len(users) == 0
	bootstrapRecovery := a.config.BootstrapEnabled && strings.TrimSpace(a.config.BootstrapAdminToken) != ""

	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"bootstrap_enabled":          bootstrapRecovery,
		"bootstrap_recovery_enabled": bootstrapRecovery,
		"initial_setup_required":     initialSetup,
		"oidc_enabled":               a.config.OIDCIssuerURL != "" && a.config.OIDCClientID != "",
		"oidc_oauth_enabled":         a.auth.OIDCOAuthReady(),
		"has_users":                  len(users) > 0,
		"has_admin_users":            hasAdminUsers,
		"app_env":                    a.config.AppEnv,
	})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	auth.WriteJSON(w, http.StatusOK, principal)
}

func (a *API) handleListTeams(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	teams, err := a.store.ListTeamsForUser(r.Context(), principal.User.ID, principal.User.IsAdmin)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(teams)})
}

func (a *API) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var input domain.CreateTeamInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" {
		a.writeError(w, r, "name_required", http.StatusBadRequest)
		return
	}

	team, err := a.store.CreateTeam(r.Context(), principal.User.ID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, team)
}

func (a *API) handleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	team, err := a.store.GetTeamByID(r.Context(), r.PathValue("teamID"))
	if err != nil {
		a.writeError(w, r, "team_not_found", http.StatusNotFound)
		return
	}
	var input domain.UpdateTeamInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	if team.IsPersonal {
		if input.IsAIEnabled == nil {
			a.writeError(w, r, "personal_workspace_ai_only", http.StatusBadRequest)
			return
		}
		updated, err := a.store.UpdateTeam(r.Context(), team.ID, domain.UpdateTeamInput{
			Name:        team.Name,
			Description: team.Description,
			IsAIEnabled: input.IsAIEnabled,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		auth.WriteJSON(w, http.StatusOK, updated)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" {
		input.Name = team.Name
	}
	if input.Description == "" {
		input.Description = team.Description
	}
	if input.Name == "" {
		a.writeError(w, r, "name_required", http.StatusBadRequest)
		return
	}

	updated, err := a.store.UpdateTeam(r.Context(), team.ID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, updated)
}

func (a *API) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(users)})
}

func (a *API) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	var input domain.UpdateUserInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	user, err := a.store.SetUserAdmin(r.Context(), r.PathValue("userID"), input.IsAdmin)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, user)
}

func (a *API) handleRuntimeConfig(w http.ResponseWriter, _ *http.Request) {
	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"general": map[string]any{
			"http_addr":  a.config.HTTPAddr,
			"app_env":    a.config.AppEnv,
			"log_level":  a.config.SlogLevel().String(),
			"log_format": a.config.LogFormatName(),
		},
		"security": map[string]any{
			"allowed_origins":                     a.config.AllowedOrigins,
			"rate_limit_per_minute":               a.config.RateLimitPerMinute,
			"rate_limit_authenticated_per_minute": a.config.RateLimitAuthenticatedPerMinute,
			"encryption_configured":               a.config.EncryptionKey != "",
		},
		"scheduler": map[string]any{
			"poll_interval":           a.config.SchedulerPollInterval.String(),
			"metrics_sync_interval":   a.config.SchedulerMetricsSyncInterval.String(),
			"account_health_interval": a.config.SchedulerAccountHealthInterval.String(),
			"workers":                 a.config.SchedulerWorkers,
		},
		"oidc": map[string]any{
			"enabled":    a.config.OIDCIssuerURL != "" && a.config.OIDCClientID != "",
			"issuer_url": a.config.OIDCIssuerURL,
			"client_id":  a.config.OIDCClientID,
			"has_secret": a.config.OIDCClientSecret != "",
		},
	})
}

func (a *API) handleListProviderInstances(w http.ResponseWriter, r *http.Request) {
	providerName := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("provider")))
	if providerName != "" {
		if _, ok := a.providers.Get(providerName); !ok {
			a.writeError(w, r, "unsupported_provider", http.StatusBadRequest)
			return
		}
	}

	items, err := a.store.ListProviderInstances(r.Context(), providerName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleGetProviderInstance(w http.ResponseWriter, r *http.Request) {
	instance, err := a.store.GetProviderInstanceByID(r.Context(), r.PathValue("instanceID"))
	if err != nil {
		a.writeError(w, r, "provider_instance_not_found", http.StatusNotFound)
		return
	}
	auth.WriteJSON(w, http.StatusOK, instance)
}

func (a *API) handleCreateProviderInstance(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var input domain.CreateProviderInstanceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	prepared, err := a.prepareProviderInstance(r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	instance, err := a.store.CreateProviderInstance(r.Context(), principal.User.ID, prepared)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, instance)
}

func (a *API) handleUpdateProviderInstance(w http.ResponseWriter, r *http.Request) {
	existing, err := a.store.GetProviderInstanceByID(r.Context(), r.PathValue("instanceID"))
	if err != nil {
		a.writeError(w, r, "provider_instance_not_found", http.StatusNotFound)
		return
	}

	var input domain.CreateProviderInstanceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(input.Provider) == "" {
		input.Provider = existing.Provider
	}
	if strings.TrimSpace(input.Name) == "" {
		input.Name = existing.Name
	}
	if strings.TrimSpace(input.InstanceURL) == "" {
		input.InstanceURL = existing.InstanceURL
	}
	if strings.TrimSpace(input.ClientID) == "" {
		input.ClientID = existing.ClientID
	}
	if len(input.Scopes) == 0 {
		input.Scopes = existing.Scopes
	}
	if strings.TrimSpace(input.AuthorizationEndpoint) == "" {
		input.AuthorizationEndpoint = existing.AuthorizationEndpoint
	}
	if strings.TrimSpace(input.TokenEndpoint) == "" {
		input.TokenEndpoint = existing.TokenEndpoint
	}
	if strings.TrimSpace(input.ClientSecret) == "" && existing.HasClientSecret {
		clientSecret, err := a.store.DecryptProviderInstanceClientSecret(existing)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		input.ClientSecret = clientSecret
	}

	prepared, err := a.prepareProviderInstance(r, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	instance, err := a.store.UpdateProviderInstance(r.Context(), existing.ID, prepared)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, instance)
}

func (a *API) handleDeleteProviderInstance(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteProviderInstance(r.Context(), r.PathValue("instanceID")); err != nil {
		switch {
		case errors.Is(err, domain.ErrProviderInstanceInUse):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, domain.ErrProviderInstanceNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := a.store.ListTeamAccounts(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.store.FillAccountSyncTimestamps(r.Context(), accounts); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(accounts)})
}

func (a *API) handleListTeamMembers(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListTeamMembers(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	var input domain.AddTeamMemberInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if input.UserID == "" {
		a.writeError(w, r, "user_id_required", http.StatusBadRequest)
		return
	}
	if !slices.Contains([]domain.TeamRole{domain.RoleOwner, domain.RoleEditor, domain.RoleViewer}, input.Role) {
		a.writeError(w, r, "role_invalid", http.StatusBadRequest)
		return
	}

	team, err := a.store.GetTeamByID(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if team.IsPersonal {
		a.writeError(w, r, "cannot_add_members_personal", http.StatusBadRequest)
		return
	}

	membership, err := a.store.AddTeamMember(r.Context(), r.PathValue("teamID"), input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, membership)
}

func (a *API) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	team, err := a.store.GetTeamByID(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if team.IsPersonal {
		a.writeError(w, r, "cannot_remove_members_personal", http.StatusBadRequest)
		return
	}
	if err := a.store.RemoveTeamMember(r.Context(), r.PathValue("teamID"), r.PathValue("userID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var input domain.CreateAccountInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	providerInstance, err := a.resolveProviderInstance(r, input.ProviderInstanceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.Provider == "" && providerInstance != nil {
		input.Provider = providerInstance.Provider
	}
	if input.Provider == "" {
		a.writeError(w, r, "provider_required", http.StatusBadRequest)
		return
	}
	if providerInstance != nil && providerInstance.Provider != input.Provider {
		a.writeError(w, r, "provider_instance_id_mismatch", http.StatusBadRequest)
		return
	}

	providerImpl, ok := a.providers.Get(input.Provider)
	if !ok {
		a.writeError(w, r, "unsupported_provider", http.StatusBadRequest)
		return
	}

	connectedAccount, err := providerImpl.ConnectAccount(a.providerContext(r.Context()), input, providerInstance)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	account, err := a.store.CreateAccount(r.Context(), r.PathValue("teamID"), connectedAccount)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.syncAccountMetricsNow(r.Context(), account, connectedAccount)
	auth.WriteJSON(w, http.StatusCreated, account)
}

func (a *API) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	teamID := r.PathValue("teamID")
	accountID := r.PathValue("accountID")

	acc, err := a.store.GetAccountByID(r.Context(), accountID)
	if err != nil {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	if acc.TeamID != teamID {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	allowed, err := a.auth.PrincipalHasTeamAccess(r.Context(), principal, acc.TeamID, domain.RoleEditor, domain.RoleOwner)
	if err != nil || !allowed {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}

	var input domain.UpdateAccountInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	updated, err := a.store.UpdateAccount(r.Context(), teamID, accountID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, updated)
}

func (a *API) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	acc, err := a.store.GetAccountByID(r.Context(), r.PathValue("accountID"))
	if err != nil {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	allowed, err := a.auth.PrincipalHasTeamAccess(r.Context(), principal, acc.TeamID, domain.RoleEditor, domain.RoleOwner)
	if err != nil || !allowed {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}
	if err := a.store.DeleteSocialAccount(r.Context(), acc.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleListPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := a.store.ListTeamPosts(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.attachPublishedLinks(r, posts); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(posts)})
}

func (a *API) handleGetPost(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	post, err := a.store.GetScheduledPostByID(r.Context(), r.PathValue("postID"))
	if err != nil {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	allowed, err := a.auth.PrincipalHasTeamAccess(r.Context(), principal, post.TeamID, domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)
	if err != nil || !allowed {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}
	posts := []domain.ScheduledPost{post}
	if err := a.attachPublishedLinks(r, posts); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, posts[0])
}

func (a *API) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	var input domain.CreatePostInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	input.Content = strings.TrimSpace(input.Content)
	input.Title = strings.TrimSpace(input.Title)
	input.Visibility = domain.NormalizePostVisibility(input.Visibility)
	input.MediaIDs = domain.NormalizeMediaIDs(input.MediaIDs)
	input.MediaExcludeByAccount = domain.NormalizeMediaExcludeByAccount(input.MediaExcludeByAccount, input.MediaIDs)
	input.AccountContentOverride = domain.NormalizeAccountContentOverride(input.AccountContentOverride, input.TargetAccounts)
	if input.ScheduledAt.IsZero() {
		input.ScheduledAt = time.Now().UTC()
	}

	pathTeamID := strings.TrimSpace(r.PathValue("teamID"))
	validation, effectiveTeam, err := a.validatePostInput(r.Context(), pathTeamID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !validation.Valid {
		auth.WriteJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}
	if pathTeamID != "" && effectiveTeam != pathTeamID {
		a.writeError(w, r, "team_mismatch_url_destinations", http.StatusBadRequest)
		return
	}

	allowed, err := a.auth.PrincipalHasTeamAccess(r.Context(), principal, effectiveTeam, domain.RoleEditor, domain.RoleOwner)
	if err != nil || !allowed {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}

	post, err := a.store.CreateScheduledPost(r.Context(), effectiveTeam, principal, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, post)
}

func (a *API) handleUpdatePost(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	existing, err := a.store.GetScheduledPostByID(r.Context(), r.PathValue("postID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if existing.Source == domain.PostSourceImported {
		a.writeError(w, r, "imported_post_read_only", http.StatusForbidden)
		return
	}
	allowed, err := a.auth.PrincipalHasTeamAccess(r.Context(), principal, existing.TeamID, domain.RoleEditor, domain.RoleOwner)
	if err != nil || !allowed {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}

	var patch domain.UpdatePostPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	versions, err := a.store.ListPostVersionsForTeamPost(r.Context(), existing.TeamID, existing.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	merged, _ := domain.ApplyPostPatch(existing, versions, patch)
	pathTeamID := strings.TrimSpace(r.PathValue("teamID"))
	validation, effectiveTeam, err := a.validatePostInput(r.Context(), pathTeamID, merged)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !validation.Valid {
		auth.WriteJSON(w, http.StatusUnprocessableEntity, validation)
		return
	}
	if pathTeamID != "" && effectiveTeam != pathTeamID {
		a.writeError(w, r, "team_mismatch_url_destinations", http.StatusBadRequest)
		return
	}
	if effectiveTeam != existing.TeamID {
		a.writeError(w, r, "target_accounts_same_team", http.StatusBadRequest)
		return
	}

	post, err := a.store.PatchScheduledPost(r.Context(), existing.TeamID, r.PathValue("postID"), patch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, post)
}

func (a *API) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	post, err := a.store.GetScheduledPostByID(r.Context(), r.PathValue("postID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	allowed, err := a.auth.PrincipalHasTeamAccess(r.Context(), principal, post.TeamID, domain.RoleEditor, domain.RoleOwner)
	if err != nil || !allowed {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}
	a.skipRecurringOccurrenceForDeletedPost(r, post.TeamID, r.PathValue("postID"))
	if err := a.store.DeleteScheduledPost(r.Context(), post.TeamID, r.PathValue("postID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) skipRecurringOccurrenceForDeletedPost(r *http.Request, teamID, postID string) {
	tplID, occAt, role, err := a.store.GetScheduledPostTemplateLink(r.Context(), teamID, postID)
	if err != nil || tplID == "" || occAt == nil {
		return
	}
	switch role {
	case domain.TemplatePostRoleAnnouncement:
		_ = a.store.AddPostTemplateAnnouncementSkip(r.Context(), teamID, tplID, *occAt)
	default:
		_ = a.store.AddPostTemplateSkip(r.Context(), teamID, tplID, *occAt)
	}
}

func (a *API) handleCancelPost(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	post, err := a.store.GetScheduledPostByID(r.Context(), r.PathValue("postID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	allowed, err := a.auth.PrincipalHasTeamAccess(r.Context(), principal, post.TeamID, domain.RoleEditor, domain.RoleOwner)
	if err != nil || !allowed {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}
	if err := a.store.CancelScheduledPost(r.Context(), post.TeamID, r.PathValue("postID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleValidatePost(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var input domain.CreatePostInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	input.Content = strings.TrimSpace(input.Content)
	input.Visibility = domain.NormalizePostVisibility(input.Visibility)
	input.MediaIDs = domain.NormalizeMediaIDs(input.MediaIDs)
	input.AccountContentOverride = domain.NormalizeAccountContentOverride(input.AccountContentOverride, input.TargetAccounts)
	pathTeamID := strings.TrimSpace(r.PathValue("teamID"))
	validation, effectiveTeam, err := a.validatePostInput(r.Context(), pathTeamID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if pathTeamID != "" && effectiveTeam != pathTeamID {
		a.writeError(w, r, "team_mismatch_url_destinations", http.StatusBadRequest)
		return
	}
	allowed, err := a.auth.PrincipalHasTeamAccess(r.Context(), principal, effectiveTeam, domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)
	if err != nil || !allowed {
		a.writeError(w, r, "forbidden", http.StatusForbidden)
		return
	}
	auth.WriteJSON(w, http.StatusOK, validation)
}

func (a *API) validatePostInput(ctx context.Context, pathTeamID string, input domain.CreatePostInput) (validationResponse, string, error) {
	if input.Draft {
		if strings.TrimSpace(pathTeamID) == "" {
			return validationResponse{}, "", errors.New("team id required")
		}
		if len(input.TargetAccounts) > 0 {
			accounts, err := a.store.GetAccountsByIDsGlobal(ctx, input.TargetAccounts)
			if err != nil {
				return validationResponse{}, "", err
			}
			if len(accounts) != len(input.TargetAccounts) {
				return validationResponse{}, "", errors.New("one or more target accounts are missing")
			}
			for _, acc := range accounts {
				if acc.TeamID != pathTeamID {
					return validationResponse{}, "", errors.New("target accounts must belong to the team in the URL")
				}
			}
		}
		return validationResponse{
			Valid:         true,
			MaxChars:      0,
			ContentLength: len([]rune(input.Content)),
			Destinations:  nil,
		}, pathTeamID, nil
	}

	if err := input.Validate(); err != nil {
		return validationResponse{}, "", err
	}

	accounts, err := a.store.GetAccountsByIDsGlobal(ctx, input.TargetAccounts)
	if err != nil {
		return validationResponse{}, "", err
	}
	if len(accounts) == 0 {
		return validationResponse{}, "", errors.New("one or more target accounts are missing")
	}
	effectiveTeam := accounts[0].TeamID

	destinations := make([]destinationInfo, 0, len(accounts))
	maxChars := 0
	allValid := true
	for _, account := range accounts {
		if account.TeamID != effectiveTeam {
			return validationResponse{}, "", errors.New("target accounts must belong to one team")
		}
		if strings.TrimSpace(pathTeamID) != "" && account.TeamID != pathTeamID {
			return validationResponse{}, "", errors.New("target accounts must belong to the team in the URL")
		}
		providerImpl, ok := a.providers.Get(account.Provider)
		if !ok {
			return validationResponse{}, "", errors.New("one or more target accounts use an unsupported provider")
		}

		capabilities, err := providerImpl.Capabilities(ctx, account)
		if err != nil {
			return validationResponse{}, "", err
		}

		effectiveContent := input.EffectiveContent(account.ID)
		contentLen := len([]rune(effectiveContent))
		isValid := capabilities.MaxChars == 0 || contentLen <= capabilities.MaxChars
		if !isValid {
			allValid = false
		}

		destinations = append(destinations, destinationInfo{
			AccountID: account.ID,
			Provider:  account.Provider,
			MaxChars:  capabilities.MaxChars,
			Length:    contentLen,
			Valid:     isValid,
		})
		if maxChars == 0 || capabilities.MaxChars < maxChars {
			maxChars = capabilities.MaxChars
		}
	}

	slices.SortFunc(destinations, func(a, b destinationInfo) int {
		return strings.Compare(a.AccountID, b.AccountID)
	})

	// Calculate global content length for reporting
	globalContentLen := len([]rune(input.Content))

	return validationResponse{
		MaxChars:      maxChars,
		ContentLength: globalContentLen,
		Valid:         allValid,
		Destinations:  destinations,
	}, effectiveTeam, nil
}

func (a *API) handleMigrateAccount(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var input domain.MigrateAccountInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(input.TargetTeamID) == "" {
		a.writeError(w, r, "target_team_id_required", http.StatusBadRequest)
		return
	}
	accountID := r.PathValue("accountID")
	if err := a.store.MigrateAccountToTeam(r.Context(), principal.User.ID, accountID, input.TargetTeamID, principal.User.IsAdmin); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "forbidden") {
			http.Error(w, msg, http.StatusForbidden)
			return
		}
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	acc, err := a.store.GetAccountByID(r.Context(), accountID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, acc)
}

func (a *API) handleCreateTeamInvitation(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var input domain.CreateTeamInvitationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	inv, token, err := a.store.CreateTeamInvitation(r.Context(), r.PathValue("teamID"), principal.User.ID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, map[string]any{
		"invitation": inv,
		"token":      token,
	})
}

func (a *API) handleAcceptTeamInvitation(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var input domain.AcceptTeamInvitationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(input.Token) == "" {
		a.writeError(w, r, "token_required", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(strings.ToLower(principal.User.Email))
	membership, err := a.store.AcceptTeamInvitation(r.Context(), principal.User.ID, email, input.Token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	auth.WriteJSON(w, http.StatusOK, membership)
}

func (a *API) attachPublishedLinks(r *http.Request, posts []domain.ScheduledPost) error {
	if len(posts) == 0 {
		return nil
	}
	postIDs := make([]string, 0, len(posts))
	for _, post := range posts {
		postIDs = append(postIDs, post.ID)
	}
	links, err := a.store.LoadPublishedLinksByPostIDs(r.Context(), postIDs)
	if err != nil {
		return err
	}
	for idx := range posts {
		posts[idx].PublishedLinks = links[posts[idx].ID]
	}
	return nil
}

func (a *API) resolveProviderInstance(r *http.Request, instanceID string) (*domain.ProviderInstance, error) {
	if strings.TrimSpace(instanceID) == "" {
		return nil, nil
	}
	instance, err := a.store.GetProviderInstanceByID(r.Context(), strings.TrimSpace(instanceID))
	if err != nil {
		return nil, errors.New("provider_instance_id is invalid")
	}
	return &instance, nil
}

func (a *API) prepareProviderInstance(r *http.Request, input domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error) {
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	if input.Provider == "" {
		return domain.PreparedProviderInstance{}, errors.New("provider is required")
	}

	providerImpl, ok := a.providers.Get(input.Provider)
	if !ok {
		return domain.PreparedProviderInstance{}, errors.New("unsupported provider")
	}
	return providerImpl.PrepareProviderInstance(a.providerContext(r.Context()), input)
}

func (a *API) providerContext(ctx context.Context) context.Context {
	return provider.WithOutboundInstancePolicy(ctx, provider.OutboundPolicy{
		AllowPrivateLAN: a.config.AllowPrivateProviderInstanceURLs,
	})
}

func (a *API) syncAccountMetricsNow(ctx context.Context, account domain.SocialAccount, connected domain.ConnectedAccount) {
	providerImpl, ok := a.providers.Get(account.Provider)
	if !ok {
		return
	}
	metrics, err := providerImpl.GetAccountMetrics(ctx, account, provider.PublishAuth{
		AccessToken:  connected.AccessToken,
		RefreshToken: connected.RefreshToken,
	})
	if err != nil {
		if a.log != nil {
			a.log.Debug("initial account metrics sync failed", "account_id", account.ID, "provider", account.Provider, "error", err)
		}
		return
	}
	snapshot := make(map[string]int64, len(metrics))
	for _, metric := range metrics {
		name := strings.TrimSpace(metric.Name)
		if name == "" {
			continue
		}
		snapshot[name] = metric.Value
	}
	if err := a.store.UpsertAccountMetrics(ctx, account.ID, snapshot, time.Now().UTC()); err != nil && a.log != nil {
		a.log.Debug("initial account metrics upsert failed", "account_id", account.ID, "provider", account.Provider, "error", err)
	}
}

func sliceOrEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
