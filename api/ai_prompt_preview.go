package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"git.f4mily.net/goloom/internal/ai"
	"git.f4mily.net/goloom/internal/aicontext"
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

	aiContext, err := aicontext.Build(r.Context(), a.store, teamID, aicontext.Full())
	if err != nil {
		a.writeStoreError(w, r, err, "")
		return
	}

	enrichedParams, err := enrichAIJobParams(r.Context(), ensureJSONObject(input.Params))
	if err != nil {
		a.writeError(w, r, "invalid_ai_prompt_params", http.StatusBadRequest)
		return
	}

	aiContext, err = aicontext.Select(aiContext, enrichedParams)
	if err != nil {
		code := "invalid_ai_prompt_params"
		if errors.Is(err, aicontext.ErrKnowledgeSourceUnavailable) {
			code = "knowledge_source_unavailable"
		}
		a.writeError(w, r, code, http.StatusBadRequest)
		return
	}
	systemPrompt, generationPrompt, err := ai.PrepareVoicePrompt(aiContext, enrichedParams)
	if err != nil {
		a.writeError(w, r, "invalid_ai_prompt_params", http.StatusBadRequest)
		return
	}

	auth.WriteJSON(w, http.StatusOK, aiPromptPreviewResponse{
		SystemPrompt:     systemPrompt,
		GenerationPrompt: generationPrompt,
	})
}
