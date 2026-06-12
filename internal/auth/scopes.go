package auth

import (
	"context"
	"net/http"
	"slices"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
)

const (
	ScopeAIReadContext = "ai:read:context"
	ScopeAIWriteDrafts = "ai:write:drafts"
	ScopeAITriggerJobs = "ai:trigger:jobs"
	ScopeAIChat        = "ai:chat"
)

func PrincipalFromContext(ctx context.Context) *domain.AuthenticatedPrincipal {
	principal, ok := security.PrincipalFromContext[domain.AuthenticatedPrincipal](ctx)
	if !ok {
		return nil
	}
	return &principal
}

// RequireScope returns middleware that checks the principal has ALL required scopes.
func RequireScope(scopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal := PrincipalFromContext(r.Context())
			if principal == nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if principal.Kind == "oidc" {
				next.ServeHTTP(w, r)
				return
			}
			for _, required := range scopes {
				if !hasScope(principal.Scopes, required) {
					writeError(w, r, "ai_scope_required", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func hasScope(scopes []string, required string) bool {
	return slices.Contains(scopes, required)
}

// HasScope checks if the given scopes slice contains the required scope.
// This is exported for use by other packages (e.g., MCP server).
func HasScope(scopes []string, required string) bool {
	return slices.Contains(scopes, required)
}

func writeError(w http.ResponseWriter, _ *http.Request, key string, status int) {
	http.Error(w, key, status)
}
