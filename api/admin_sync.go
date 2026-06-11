package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/scheduler"
)

type metricsSyncRunner interface {
	SyncPostMetricsNow(ctx context.Context)
	SyncAccountMetricsNow(ctx context.Context)
	SyncExternalPostsNow(ctx context.Context)
	SyncRSSFeedsNow(ctx context.Context)
	ImportOldPosts(ctx context.Context, teamID string, input scheduler.ImportOldPostsInput) (scheduler.ImportOldPostsResult, error)
	RegeneratePostTemplateOccurrence(ctx context.Context, teamID, templateID string, occurrenceAt time.Time) (domain.PostTemplateRegenerateResult, error)
	RegeneratePostTemplateHorizon(ctx context.Context, teamID, templateID string) (domain.PostTemplateRegenerateResult, error)
}

func (a *API) handleAdminSyncStatus(w http.ResponseWriter, r *http.Request) {
	since := time.Now().Add(-30 * 24 * time.Hour)
	st, err := a.store.AdminSyncStatus(r.Context(), since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	st.PostMetricsSyncInterval = a.config.SchedulerMetricsSyncInterval.String()
	st.AccountMetricsSyncInterval = (24 * time.Hour).String()
	st.AccountHealthInterval = a.config.SchedulerAccountHealthInterval.String()
	auth.WriteJSON(w, http.StatusOK, st)
}

func (a *API) handleAdminSyncMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeError(w, r, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.metricsSync == nil {
		a.writeError(w, r, "metrics_sync_not_available", http.StatusServiceUnavailable)
		return
	}
	go func() {
		ctx := context.Background()
		a.metricsSync.SyncPostMetricsNow(ctx)
		a.metricsSync.SyncAccountMetricsNow(ctx)
	}()
	auth.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status":  "started",
		"message": "Post engagement and account metrics sync started in the background.",
	})
}

func (a *API) handleAdminSyncExternalPosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeError(w, r, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.metricsSync == nil {
		a.writeError(w, r, "metrics_sync_not_available", http.StatusServiceUnavailable)
		return
	}
	go a.metricsSync.SyncExternalPostsNow(context.Background())
	auth.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status":  "started",
		"message": "External post import started in the background.",
	})
}

func (a *API) handleAdminSyncRSSFeeds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeError(w, r, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.metricsSync == nil {
		a.writeError(w, r, "metrics_sync_not_available", http.StatusServiceUnavailable)
		return
	}
	go a.metricsSync.SyncRSSFeedsNow(context.Background())
	auth.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status":  "started",
		"message": "RSS feed import started in the background.",
	})
}

func (a *API) handleImportOldPosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeError(w, r, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.metricsSync == nil {
		a.writeError(w, r, "metrics_sync_not_available", http.StatusServiceUnavailable)
		return
	}
	teamID := r.PathValue("teamID")
	var input scheduler.ImportOldPostsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	result, err := a.metricsSync.ImportOldPosts(r.Context(), teamID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, result)
}
