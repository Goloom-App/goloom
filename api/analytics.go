package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) handleTeamAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	top := 10
	if raw := strings.TrimSpace(r.URL.Query().Get("top_posts")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 100 {
			top = n
		}
	}
	report, err := a.store.GetTeamAnalyticsReport(r.Context(), r.PathValue("teamID"), top)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	decodePostEngagementTitles(report.TopPosts)
	auth.WriteJSON(w, http.StatusOK, report)
}

func (a *API) handleTeamAnalyticsPosts(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			offset = n
		}
	}
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	items, err := a.store.ListTeamPostAnalyticsRanking(r.Context(), r.PathValue("teamID"), sort, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	decodePostAnalyticsListTitles(items)
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleTeamAnalyticsChart(w http.ResponseWriter, r *http.Request) {
	metric := strings.TrimSpace(r.URL.Query().Get("metric"))
	if metric == "" {
		a.writeError(w, r, "metric_query_required", http.StatusBadRequest)
		return
	}
	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			days = n
		}
	}
	series, err := a.store.GetTeamMetricHistorySeries(r.Context(), r.PathValue("teamID"), metric, days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"metric": metric,
		"days":   days,
		"series": sliceOrEmpty(series),
	})
}

func (a *API) handleTeamAccountGrowth(w http.ResponseWriter, r *http.Request) {
	days := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			days = n
		}
	}
	teamID := r.PathValue("teamID")
	accountID := strings.TrimSpace(r.PathValue("accountID"))
	series, err := a.store.GetTeamAccountMetricHistorySeries(r.Context(), teamID, accountID, days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"days":    days,
		"account": accountID,
		"series":  sliceOrEmpty(series),
	})
}

func (a *API) handleTeamHashtagAnalytics(w http.ResponseWriter, r *http.Request) {
	days := 90
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 366 {
			days = n
		}
	}
	limit := 30
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))
	teamID := r.PathValue("teamID")
	items, err := a.store.ListTeamHashtagPerformance(r.Context(), teamID, days, provider, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	insights, err := a.store.GetTeamHashtagInsights(r.Context(), teamID, days, provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"days":     days,
		"provider": provider,
		"items":    sliceOrEmpty(items),
		"insights": insights,
	})
}

func (a *API) handleTeamEngagementHeatmap(w http.ResponseWriter, r *http.Request) {
	days := 90
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 366 {
			days = n
		}
	}
	accountID := strings.TrimSpace(r.URL.Query().Get("account"))
	items, err := a.store.GetTeamEngagementHeatmap(r.Context(), r.PathValue("teamID"), days, accountID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"days": days, "account": accountID, "buckets": sliceOrEmpty(items)})
}

func (a *API) handleTeamAnalytics(w http.ResponseWriter, r *http.Request) {
	top := 10
	if raw := strings.TrimSpace(r.URL.Query().Get("top_posts")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 100 {
			top = n
		}
	}
	summary, err := a.store.GetTeamAnalytics(r.Context(), r.PathValue("teamID"), top)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	decodePostEngagementTitles(summary.TopPosts)
	auth.WriteJSON(w, http.StatusOK, summary)
}

func (a *API) handlePostAnalytics(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	postID := r.PathValue("postID")
	if _, err := a.store.GetScheduledPost(r.Context(), teamID, postID); err != nil {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	items, err := a.store.ListPostMetricsForTeamPost(r.Context(), teamID, postID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	links, err := a.store.LoadPublishedLinksByPostIDs(r.Context(), []string{postID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	publishedLinks := links[postID]
	if publishedLinks == nil {
		publishedLinks = map[string]string{}
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items), "published_links": publishedLinks})
}

func (a *API) handleListPostVersions(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	postID := r.PathValue("postID")
	if _, err := a.store.GetScheduledPost(r.Context(), teamID, postID); err != nil {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	items, err := a.store.ListPostVersionsForTeamPost(r.Context(), teamID, postID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleListAllTeamPostVersions(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	items, err := a.store.ListAllPostVersionsForTeam(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

type patchPostVersionsRequest struct {
	Versions []struct {
		AccountID string `json:"account_id"`
		Content   string `json:"content"`
	} `json:"versions"`
}

func (a *API) handlePatchPostVersions(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	postID := r.PathValue("postID")
	if _, err := a.store.GetScheduledPost(r.Context(), teamID, postID); err != nil {
		a.writeError(w, r, "not_found", http.StatusNotFound)
		return
	}
	var body patchPostVersionsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	patches := make([]domain.PostVersion, 0, len(body.Versions))
	for _, row := range body.Versions {
		aid := strings.TrimSpace(row.AccountID)
		if aid == "" {
			continue
		}
		patches = append(patches, domain.PostVersion{
			PostID:    postID,
			AccountID: aid,
			Content:   strings.TrimSpace(row.Content),
		})
	}
	if err := a.store.ApplyPostVersionsPatch(r.Context(), teamID, postID, patches); err != nil {
		if err.Error() == "post not found" {
			a.writeError(w, r, "not_found", http.StatusNotFound)
			return
		}
		if err.Error() == "account not targeted by post" {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := a.store.ListPostVersionsForTeamPost(r.Context(), teamID, postID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}
