package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) handleListKnowledgeSources(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListKnowledgeSources(r.Context(), r.PathValue("teamID"))
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(items)})
}

func (a *API) handleCreateKnowledgeSource(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input domain.KnowledgeSource
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	input.TeamID = teamID

	if input.Type == domain.KnowledgeSourceURL && strings.TrimSpace(input.Content) == "" {
		content, err := fetchURLText(r.Context(), input.SourceURL)
		if err != nil {
			a.writeError(w, r, "knowledge_url_fetch_failed", http.StatusBadRequest)
			return
		}
		input.Content = content
	}

	if !a.checkKnowledgeInput(w, r, input) {
		return
	}

	a.auditedHandler(func(w http.ResponseWriter, r *http.Request) {
		existing, err := a.store.ListKnowledgeSources(r.Context(), teamID)
		if err != nil {
			a.writeStoreError(w, r, err, "")
			return
		}
		if len(existing) >= maxKnowledgeSources {
			a.writeError(w, r, "knowledge_source_limit_reached", http.StatusConflict)
			return
		}

		created, err := a.store.CreateKnowledgeSource(r.Context(), teamID, input)
		if err != nil {
			a.writeStoreError(w, r, err, "")
			return
		}
		a.recordAudit(r, teamID, "knowledge_source.create", "knowledge_source", &created.ID, "Created knowledge source")
		auth.WriteJSON(w, http.StatusCreated, created)
	})(w, r)
}

func (a *API) handleUpdateKnowledgeSource(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	sourceID := r.PathValue("sourceID")
	var input domain.KnowledgeSource
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	input.TeamID = teamID

	if input.Type == domain.KnowledgeSourceURL && strings.TrimSpace(input.Content) == "" {
		content, err := fetchURLText(r.Context(), input.SourceURL)
		if err != nil {
			a.writeError(w, r, "knowledge_url_fetch_failed", http.StatusBadRequest)
			return
		}
		input.Content = content
	}

	if !a.checkKnowledgeInput(w, r, input) {
		return
	}

	a.auditedHandler(func(w http.ResponseWriter, r *http.Request) {
		before, err := a.store.GetKnowledgeSourceByID(r.Context(), teamID, sourceID)
		if err != nil {
			a.writeStoreError(w, r, err, "knowledge_source_not_found")
			return
		}
		updated, err := a.store.UpdateKnowledgeSource(r.Context(), teamID, sourceID, input)
		if err != nil {
			a.writeStoreError(w, r, err, "knowledge_source_not_found")
			return
		}
		if auditChanged(before, updated) {
			a.recordAudit(r, teamID, "knowledge_source.update", "knowledge_source", &sourceID, "Updated knowledge source")
		}
		auth.WriteJSON(w, http.StatusOK, updated)
	})(w, r)
}

func (a *API) handleDeleteKnowledgeSource(w http.ResponseWriter, r *http.Request) {
	teamID, sourceID := r.PathValue("teamID"), r.PathValue("sourceID")
	_, err := a.store.GetKnowledgeSourceByID(r.Context(), teamID, sourceID)
	if errors.Is(err, domain.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	if err := a.store.DeleteKnowledgeSource(r.Context(), teamID, sourceID); err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}
	a.recordAudit(r, teamID, "knowledge_source.delete", "knowledge_source", &sourceID, "Deleted knowledge source")
	w.WriteHeader(http.StatusNoContent)
}

// checkKnowledgeInput validates a knowledge source (after any URL fetch filled
// its content) and writes the 400 response itself when it is rejected.
func (a *API) checkKnowledgeInput(w http.ResponseWriter, r *http.Request, input domain.KnowledgeSource) bool {
	if err := validateKnowledgeSource(input); err != nil {
		a.writeError(w, r, "invalid_knowledge_source", http.StatusBadRequest)
		return false
	}
	if utf8.RuneCountInString(input.Content) > maxKnowledgeContentChars {
		a.writeError(w, r, "knowledge_source_too_large", http.StatusBadRequest)
		return false
	}
	return true
}
