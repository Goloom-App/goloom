package api

import (
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

// requireAuth applies the API's default token policy after authentication and
// before route-specific role/feature checks. New routes inherit it.
func (a *API) requireAuth(next http.Handler) http.Handler {
	return a.auth.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := auth.PrincipalFromContext(r.Context())
		teamID := r.PathValue("teamID")
		if teamID != "" {
			allowed, err := a.auth.PrincipalHasTeamAccess(r.Context(), *principal, teamID, domain.RoleViewer, domain.RoleEditor, domain.RoleOwner)
			if err != nil {
				a.writeStoreError(w, r, err, "")
				return
			}
			if !allowed {
				a.writeError(w, r, "forbidden", http.StatusForbidden)
				return
			}
		}
		if !tokenRouteAllows(*principal, r) {
			a.writeError(w, r, "scope_required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func tokenRouteAllows(principal domain.AuthenticatedPrincipal, r *http.Request) bool {
	pattern := r.Pattern
	_, path, _ := strings.Cut(pattern, " ")
	// Admin access and user-wide push subscriptions require full global authority.
	if strings.HasPrefix(path, "/v1/admin/") || path == "/v1/me/push-subscriptions" || strings.HasPrefix(path, "/v1/me/push-subscriptions/") {
		return auth.PrincipalCanManageAPIToken(principal, nil, nil)
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		// These responses aggregate global/user-wide data with no team projection.
		switch path {
		case "/v1/users", "/v1/teams", "/v1/me/review-counts", "/v1/me/team-notification-prefs", "/v1/me/push-subscriptions":
			if principal.TokenTeamID != nil {
				return false
			}
		}
		return auth.PrincipalAllows(principal, auth.ScopeRead)
	case http.MethodOptions:
		return true
	}
	// Privilege management cannot be delegated to a scoped token. Unscoped
	// team-bound tokens retain authority only within their bound team.
	if pattern == "PATCH /v1/teams/{teamID}" ||
		pattern == "POST /v1/teams" ||
		strings.Contains(pattern, "/members") || strings.Contains(pattern, "/invitations") ||
		strings.Contains(pattern, "/ai-service-config") || strings.Contains(pattern, "/accounts") {
		var teamID *string
		if id := r.PathValue("teamID"); id != "" && !strings.HasSuffix(pattern, "/migrate") {
			teamID = &id
		}
		return auth.PrincipalCanManageAPIToken(principal, nil, teamID)
	}
	switch pattern {
	case "POST /v1/me/api-tokens", "DELETE /v1/me/api-tokens/{tokenID}":
		// #160 checks exact delegated authority, ownership and expiry in handlers.
		return true
	case "POST /v1/auth/logout", "POST /v1/teams/{teamID}/posts/validate",
		"POST /v1/teams/{teamID}/ai/prompt-preview", "POST /v1/teams/{teamID}/ai/chat":
		return auth.PrincipalAllows(principal, auth.ScopeRead)
	case "POST /v1/teams/{teamID}/posts", "PATCH /v1/teams/{teamID}/posts/{postID}",
		"POST /v1/teams/{teamID}/posts/{postID}/cancel":
		// Handlers enforce the scope for the actual post target state.
		return auth.PrincipalAllows(principal, auth.ScopeWriteDraft) || auth.PrincipalAllows(principal, auth.ScopeWriteSchedule)
	case "DELETE /v1/teams/{teamID}/posts/{postID}":
		return auth.PrincipalAllows(principal, auth.ScopeDeleteDraft) || auth.PrincipalAllows(principal, auth.ScopeDeleteSchedule)
	case "POST /v1/teams/{teamID}/posts/draft", "POST /v1/teams/{teamID}/ai-trigger",
		"POST /v1/teams/{teamID}/ai-jobs/{jobID}/cancel":
		return auth.PrincipalAllows(principal, auth.ScopeWriteDraft)
	case "POST /v1/teams/{teamID}/ai/confirm-action":
		// Tools re-check their precise action scope, including deletions.
		return auth.PrincipalAllows(principal, auth.ScopeWriteDraft) || auth.PrincipalAllows(principal, auth.ScopeWriteSchedule) ||
			auth.PrincipalAllows(principal, auth.ScopeDeleteDraft) || auth.PrincipalAllows(principal, auth.ScopeDeleteSchedule)
	}
	if r.Method == http.MethodDelete {
		return auth.PrincipalAllows(principal, auth.ScopeDelete)
	}
	return auth.PrincipalAllows(principal, auth.ScopeWrite)
}
