package mcp

import (
	"context"
	"net/http"
	"strings"
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
func PrincipalFromContext(ctx context.Context) interface{} {
	return ctx.Value(principalKey)
}

// WithPrincipal stores the authenticated principal in context.
func WithPrincipal(ctx context.Context, principal interface{}) context.Context {
	return context.WithValue(ctx, principalKey, principal)
}
