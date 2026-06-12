package mcp

import (
	"context"
	"net/http"
	"strings"

	"git.f4mily.net/goloom/internal/domain"
)

type contextKey string

const principalKey contextKey = "principal"

// ExtractBearerToken extracts the bearer token from the Authorization header.
func ExtractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

// PrincipalFromContext extracts the authenticated principal from context.
func PrincipalFromContext(ctx context.Context) *domain.AuthenticatedPrincipal {
	p, _ := ctx.Value(principalKey).(*domain.AuthenticatedPrincipal)
	return p
}

// WithPrincipal stores the authenticated principal in context. It stores a
// pointer so the tool handlers' principalFromContext assertion always matches.
func WithPrincipal(ctx context.Context, principal domain.AuthenticatedPrincipal) context.Context {
	return context.WithValue(ctx, principalKey, &principal)
}
