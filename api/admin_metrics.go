package api

import (
	"net/http"

	"git.f4mily.net/goloom/internal/auth"
)

func (a *API) handleAdminMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := a.store.AdminMetrics(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, m)
}
