package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

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
	auth.WriteJSON(w, http.StatusOK, summary)
}

func (a *API) handlePostAnalytics(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	postID := r.PathValue("postID")
	if _, err := a.store.GetScheduledPost(r.Context(), teamID, postID); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	items, err := a.store.ListPostMetricsForTeamPost(r.Context(), teamID, postID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleListPostVersions(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	postID := r.PathValue("postID")
	if _, err := a.store.GetScheduledPost(r.Context(), teamID, postID); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	items, err := a.store.ListPostVersionsForTeamPost(r.Context(), teamID, postID)
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
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var body patchPostVersionsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
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
			Content:   sanitizeContent(a.sanitizer, row.Content),
		})
	}
	if err := a.store.ApplyPostVersionsPatch(r.Context(), teamID, postID, patches); err != nil {
		if err.Error() == "post not found" {
			http.Error(w, "not found", http.StatusNotFound)
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
