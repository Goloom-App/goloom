package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
)

// handleCreateSessionFromToken exchanges a valid bearer token (bootstrap/recovery
// or API token) for a browser session cookie. This powers the token/recovery
// login path so human sessions are cookie-based like the OIDC flow.
func (a *API) handleCreateSessionFromToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		a.writeError(w, r, "invalid_json_body", http.StatusBadRequest)
		return
	}
	token := strings.TrimSpace(input.Token)
	if token == "" {
		a.writeError(w, r, "token_required", http.StatusBadRequest)
		return
	}

	principal, err := a.store.LookupAPIToken(r.Context(), token)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sessionToken, _, err := a.store.CreateSessionAPIToken(r.Context(), principal.User.ID, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	csrf, err := auth.NewCSRFToken()
	if err != nil {
		a.writeError(w, r, "failed_build_session", http.StatusInternalServerError)
		return
	}
	a.auth.WriteSessionCookies(w, sessionToken, csrf)
	auth.WriteJSON(w, http.StatusOK, map[string]any{"user": principal.User})
}

// handleLogout revokes the current browser session and clears its cookies.
func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	if principal, err := a.auth.CurrentPrincipal(r); err == nil && principal.TokenID != nil {
		// Best-effort: delete the __web_session row so the token can't be reused.
		_ = a.store.RevokeUserAPIToken(r.Context(), principal.User.ID, *principal.TokenID)
	}
	a.auth.ClearSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}
