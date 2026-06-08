package api

import (
	"encoding/json"
	"net/http"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) handleGetExternalPostMonitor(w http.ResponseWriter, r *http.Request) {
	settings, err := a.store.GetExternalPostMonitorSettings(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, settings)
}

func (a *API) handleUpsertExternalPostMonitor(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.UpsertExternalPostMonitorInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	settings, err := a.store.UpsertExternalPostMonitorSettings(r.Context(), teamID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, settings)
}
