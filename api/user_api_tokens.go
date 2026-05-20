package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

type createAPITokenRequest struct {
	Name      string  `json:"name"`
	ExpiresAt *string `json:"expires_at,omitempty"`
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
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(input.Name) == domain.WebSessionAPITokenName {
		a.writeError(w, r, "token_name_reserved", http.StatusBadRequest)
		return
	}
	var expires *time.Time
	if input.ExpiresAt != nil && *input.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *input.ExpiresAt)
		if err != nil {
			a.writeError(w, r, "expires_at_rfc3339", http.StatusBadRequest)
			return
		}
		if !t.After(time.Now().UTC()) {
			a.writeError(w, r, "expires_at_future", http.StatusBadRequest)
			return
		}
		expires = &t
	}
	plaintext, meta, err := a.store.CreateUserAPIToken(r.Context(), principal.User.ID, input.Name, expires)
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
			a.writeError(w, r, "token_not_found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
