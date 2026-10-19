package api

import (
	"net/http"
)

// Validate selections against the route's active team, never caller-supplied team IDs.
func (a *API) validateKnowledgeSelection(w http.ResponseWriter, r *http.Request, ids []string) bool {
	if ids == nil {
		return true
	}
	if len(ids) > 50 {
		a.writeError(w, r, "knowledge_source_unavailable", http.StatusBadRequest)
		return false
	}
	sources, err := a.store.ListKnowledgeSources(r.Context(), r.PathValue("teamID"))
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return false
	}
	available := map[string]bool{}
	for _, s := range sources {
		if s.TeamID == r.PathValue("teamID") {
			available[s.ID] = true
		}
	}
	for _, id := range ids {
		if !available[id] {
			a.writeError(w, r, "knowledge_source_unavailable", http.StatusBadRequest)
			return false
		}
	}
	return true
}
