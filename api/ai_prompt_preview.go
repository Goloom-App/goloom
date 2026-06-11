package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
)

type aiPromptPreviewRequest struct {
	Params json.RawMessage `json:"params"`
}

type aiPromptPreviewResponse struct {
	SystemPrompt     string `json:"system_prompt"`
	GenerationPrompt string `json:"generation_prompt"`
}

func (a *API) handleAIPromptPreview(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("teamID")
	var input aiPromptPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}

	config, err := a.store.GetAIServiceConfig(r.Context(), teamID)
	if err != nil {
		a.writeError(w, r, "ai_service_not_configured", http.StatusUnprocessableEntity)
		return
	}

	aiContext, err := a.store.GetTeamAIContext(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	contextRaw, err := json.Marshal(aiContext)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	enrichedParams, err := enrichAIJobParams(r.Context(), ensureJSONObject(input.Params))
	if err != nil {
		a.writeError(w, r, err.Error(), http.StatusBadRequest)
		return
	}

	payload, err := json.Marshal(map[string]any{
		"context": json.RawMessage(contextRaw),
		"params":  enrichedParams,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	serviceURL := strings.TrimRight(strings.TrimSpace(config.ServiceURL), "/")
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, serviceURL+"/api/v1/prompt-preview", bytes.NewReader(payload))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.writeError(w, r, "ai_service_unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if resp.StatusCode != http.StatusOK {
		a.writeError(w, r, "ai_service_unavailable", http.StatusBadGateway)
		return
	}

	var preview aiPromptPreviewResponse
	if err := json.Unmarshal(body, &preview); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, preview)
}
