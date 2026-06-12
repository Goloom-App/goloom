package api

import (
	"encoding/json"
	"net/http"

	"git.f4mily.net/goloom/internal/ai"
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

	aiContext, err := a.store.GetTeamAIContext(r.Context(), teamID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	enrichedParams, err := enrichAIJobParams(r.Context(), ensureJSONObject(input.Params))
	if err != nil {
		a.writeError(w, r, err.Error(), http.StatusBadRequest)
		return
	}

	platform := "mastodon"
	var platformProbe struct {
		Platform string `json:"platform"`
	}
	if err := json.Unmarshal(enrichedParams, &platformProbe); err == nil && platformProbe.Platform != "" {
		platform = platformProbe.Platform
	}

	generationPrompt, err := ai.BuildGenerationPromptFromParams(aiContext, enrichedParams, platform)
	if err != nil {
		a.writeError(w, r, err.Error(), http.StatusBadRequest)
		return
	}

	auth.WriteJSON(w, http.StatusOK, aiPromptPreviewResponse{
		SystemPrompt:     ai.BuildSystemPrompt(aiContext),
		GenerationPrompt: generationPrompt,
	})
}
