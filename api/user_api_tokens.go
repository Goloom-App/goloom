package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"git.f4mily.net/goloom/internal/auth"
)

type createAPITokenRequest struct {
	Name string `json:"name"`
}

func (a *API) handleListMyAPITokens(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	tokens, err := a.store.ListUserAPITokens(r.Context(), principal.User.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{"items": sliceOrEmpty(tokens)})
}

func (a *API) handleCreateMyAPIToken(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var input createAPITokenRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	plaintext, meta, err := a.store.CreateUserAPIToken(r.Context(), principal.User.ID, input.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, map[string]any{
		"token":     plaintext,
		"api_token": meta,
	})
}

func (a *API) handleRevokeMyAPIToken(w http.ResponseWriter, r *http.Request) {
	principal, err := a.auth.CurrentPrincipal(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	err = a.store.RevokeUserAPIToken(r.Context(), principal.User.ID, r.PathValue("tokenID"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "token not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
