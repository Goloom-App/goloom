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
	Name      string   `json:"name"`
	ExpiresAt *string  `json:"expires_at,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	TeamID    *string  `json:"team_id,omitempty"`
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
	normalizedScopes := normalizeTokenScopes(input.Scopes)
	var encodedScopes string
	if len(normalizedScopes) > 0 {
		raw, err := json.Marshal(normalizedScopes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		encodedScopes = string(raw)
	}
	plaintext, meta, err := a.store.CreateUserAPIToken(r.Context(), principal.User.ID, input.Name, expires, encodedScopes, normalizeOptionalStringPtr(input.TeamID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	auth.WriteJSON(w, http.StatusCreated, map[string]any{
		"token":     plaintext,
		"api_token": meta,
	})
}

func normalizeTokenScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(scopes))
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeOptionalStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
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
