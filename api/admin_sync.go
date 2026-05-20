package api

import (
	"context"
	"net/http"
	"time"

	"git.f4mily.net/goloom/internal/auth"
)

type metricsSyncRunner interface {
	SyncPostMetricsNow(ctx context.Context)
	SyncAccountMetricsNow(ctx context.Context)
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
