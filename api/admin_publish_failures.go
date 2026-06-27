package api

import (
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

// handleAdminListPublishFailures returns failed, not-yet-acknowledged posts with
// their per-account errors so an admin can inspect what went wrong.
func (a *API) handleAdminListPublishFailures(w http.ResponseWriter, r *http.Request) {
	failures, err := a.store.AdminListPublishFailures(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if failures == nil {
		failures = []domain.PublishFailure{}
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": failures})
}

// handleAdminAcknowledgePublishFailure marks a failed post as reviewed ("it's
// fine") so it stops counting toward the admin health attention banner.
func (a *API) handleAdminAcknowledgePublishFailure(w http.ResponseWriter, r *http.Request) {
	postID := strings.TrimSpace(r.PathValue("postID"))
	if postID == "" {
		a.writeError(w, r, "post_id_required", http.StatusBadRequest)
		return
	}
	ok, err := a.store.AdminAcknowledgeFailedPost(r.Context(), postID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		a.writeError(w, r, "failure_not_found", http.StatusNotFound)
		return
	}
	a.log.InfoContext(r.Context(), "admin acknowledged publish failure", "post_id", postID)
	auth.WriteJSON(w, http.StatusOK, map[string]any{"status": "acknowledged"})
}

// handleAdminRetryPublishFailure re-queues a failed post for publication.
func (a *API) handleAdminRetryPublishFailure(w http.ResponseWriter, r *http.Request) {
	postID := strings.TrimSpace(r.PathValue("postID"))
	if postID == "" {
		a.writeError(w, r, "post_id_required", http.StatusBadRequest)
		return
	}
	ok, err := a.store.AdminRetryFailedPost(r.Context(), postID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		a.writeError(w, r, "failure_not_found", http.StatusNotFound)
		return
	}
	a.log.InfoContext(r.Context(), "admin retried publish failure", "post_id", postID)
	auth.WriteJSON(w, http.StatusOK, map[string]any{"status": "requeued"})
}
