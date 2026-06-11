package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

func (a *API) handleListKnowledgeSources(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListKnowledgeSources(r.Context(), r.PathValue("teamID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

	if err := input.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	created, err := a.store.CreateKnowledgeSource(r.Context(), teamID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, created)
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

	if err := input.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	updated, err := a.store.UpdateKnowledgeSource(r.Context(), teamID, sourceID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, updated)
}

func (a *API) handleDeleteKnowledgeSource(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteKnowledgeSource(r.Context(), r.PathValue("teamID"), r.PathValue("sourceID")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func fetchURLText(ctx context.Context, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("source_url is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "goloom-knowledge-fetch/1.0")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("url fetch failed")
	}

	return extractReadableTextFromHTML(string(body)), nil
}
