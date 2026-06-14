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
	Name        string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	ExpiresAt   *string  `json:"expires_at,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	TeamID      *string  `json:"team_id,omitempty"`
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
	for _, scope := range normalizedScopes {
		if !auth.IsKnownScope(scope) {
			a.writeError(w, r, "unknown_scope", http.StatusBadRequest)
			return
		}
	}
	var encodedScopes string
	if len(normalizedScopes) > 0 {
		raw, err := json.Marshal(normalizedScopes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		encodedScopes = string(raw)
	}
	tokenTeamID := normalizeOptionalStringPtr(input.TeamID)
	plaintext, meta, err := a.store.CreateUserAPIToken(r.Context(), principal.User.ID, input.Name, expires, encodedScopes, tokenTeamID, strings.TrimSpace(input.Description))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Team-scoped keys are audited under their team so owners can see when a new
	// tool/automation key was minted for the team.
	if tokenTeamID != nil {
		tokenID := meta.ID
		a.recordAudit(r, *tokenTeamID, "api_token.create", "api_token", &tokenID, "Created API key: "+input.Name)
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
	tokenID := r.PathValue("tokenID")

	// Look the token up before revoking so a team-scoped key can be audited
	// under its team (best-effort; never blocks the revoke).
	var revoked *domain.APIToken
	if tokens, listErr := a.store.ListUserAPITokens(r.Context(), principal.User.ID); listErr == nil {
		for i := range tokens {
			if tokens[i].ID == tokenID {
				revoked = &tokens[i]
				break
			}
		}
	}

	if err := a.store.RevokeUserAPIToken(r.Context(), principal.User.ID, tokenID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			a.writeError(w, r, "token_not_found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if revoked != nil && revoked.TeamID != nil {
		id := revoked.ID
		a.recordAudit(r, *revoked.TeamID, "api_token.revoke", "api_token", &id, "Revoked API key: "+revoked.Name)
	}
	w.WriteHeader(http.StatusNoContent)
}
