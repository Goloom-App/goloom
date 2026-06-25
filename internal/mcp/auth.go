package mcp

import (
	"net/http"
	"strings"
)

// ExtractBearerToken extracts the bearer token from the Authorization header.
func ExtractBearerToken(r *http.Request) string {
	authz := r.Header.Get("Authorization")
	if len(authz) > 7 && strings.EqualFold(authz[:7], "bearer ") {
		return strings.TrimSpace(authz[7:])
	}
	return ""
}
