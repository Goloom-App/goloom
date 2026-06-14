package auth

import (
	"context"
	"net/http"
	"slices"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
)

// API token scopes. A token may also be bound to a single team (TokenTeamID);
// that restriction is enforced separately in PrincipalHasTeamAccess.
//
// read is independent; write is a superset of write:draft/write:schedule and
// delete a superset of delete:draft/delete:schedule.
const (
	ScopeRead           = "read"
	ScopeWrite          = "write"
	ScopeWriteDraft     = "write:draft"
	ScopeWriteSchedule  = "write:schedule"
	ScopeDelete         = "delete"
	ScopeDeleteDraft    = "delete:draft"
	ScopeDeleteSchedule = "delete:schedule"
)

// AllScopes lists every scope a token may be granted, used to validate requests.
var AllScopes = []string{
	ScopeRead,
	ScopeWrite,
	ScopeWriteDraft,
	ScopeWriteSchedule,
	ScopeDelete,
	ScopeDeleteDraft,
	ScopeDeleteSchedule,
}

// IsKnownScope reports whether scope is one of the recognized token scopes.
func IsKnownScope(scope string) bool {
	return slices.Contains(AllScopes, scope)
}

func PrincipalFromContext(ctx context.Context) *domain.AuthenticatedPrincipal {
	principal, ok := security.PrincipalFromContext[domain.AuthenticatedPrincipal](ctx)
	if !ok {
		return nil
	}
	return &principal
}

// ScopeSatisfied reports whether a token carrying tokenScopes is allowed to
// perform an action requiring `required`, honoring the write/delete hierarchy.
func ScopeSatisfied(tokenScopes []string, required string) bool {
	for _, granted := range tokenScopes {
		if granted == required {
			return true
		}
		if granted == ScopeWrite && (required == ScopeWriteDraft || required == ScopeWriteSchedule) {
			return true
		}
		if granted == ScopeDelete && (required == ScopeDeleteDraft || required == ScopeDeleteSchedule) {
			return true
		}
	}
	return false
}

// PrincipalAllows reports whether the principal may perform an action requiring
// the given scope. Browser/OIDC principals and unscoped tokens (empty scope
// list, the backward-compatible default) are always allowed; only tokens with an
// explicit scope list are restricted.
func PrincipalAllows(principal domain.AuthenticatedPrincipal, required string) bool {
	if principal.Kind != "api_token" {
		return true
	}
	if len(principal.Scopes) == 0 {
		return true
	}
	return ScopeSatisfied(principal.Scopes, required)
}

// RequireTokenScope returns middleware that enforces the given scope for API
// token callers. It is the route-level counterpart to PrincipalAllows for
// actions whose required scope is fixed by the route (handlers whose scope
// depends on request data call PrincipalAllows directly instead).
func RequireTokenScope(required string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := PrincipalFromContext(r.Context())
			if principal == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if !PrincipalAllows(*principal, required) {
				writeError(w, r, "scope_required", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeError(w http.ResponseWriter, _ *http.Request, key string, status int) {
	http.Error(w, key, status)
}
