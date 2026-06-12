package mcp

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store"
)

// Handler serves the MCP protocol over HTTP (SSE + JSON-RPC POST).
type Handler struct {
	handler   http.Handler
	store     store.Store
	auth      *auth.Service
	providers *provider.Registry
	config    config.Config
	logger    *slog.Logger
}

// NewHandler creates a new MCP handler with all tools registered.
func NewHandler(
	logger *slog.Logger,
	store store.Store,
	authSvc *auth.Service,
	providers *provider.Registry,
	cfg config.Config,
) *Handler {
	h := &Handler{
		store:     store,
		auth:      authSvc,
		providers: providers,
		config:    cfg,
		logger:    logger,
	}

	// Create the SSE handler that manages MCP sessions
	sseHandler := mcp.NewSSEHandler(func(r *http.Request) *mcp.Server {
		return h.createServer(r)
	}, nil)

	h.handler = sseHandler
	return h
}

// createServer creates a new MCP server for each request (with tools registered).
func (h *Handler) createServer(r *http.Request) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "goloom",
		Version: "1.0.0",
	}, nil)

	h.registerTools(server)
	return server
}

// ServeHTTP handles both SSE (GET) and JSON-RPC (POST) requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Extract bearer token
	token := ExtractBearerToken(r)
	if token == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}

	// 2. Validate token via store
	principal, err := h.store.LookupAPIToken(r.Context(), token)
	if err != nil {
		h.logger.Debug("mcp auth failed", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 3. Check AI scope
	if principal.Kind != "oidc" {
		if !auth.HasScope(principal.Scopes, auth.ScopeAIReadContext) &&
			!auth.HasScope(principal.Scopes, auth.ScopeAIWriteDrafts) {
			http.Error(w, "scope ai:read:context or ai:write:drafts required", http.StatusForbidden)
			return
		}
	}

	// 4. Store principal in request context for tool handlers
	r = r.WithContext(WithPrincipal(r.Context(), principal))

	// 5. Delegate to SSE handler
	h.handler.ServeHTTP(w, r)
}

// principalFromContext extracts the authenticated principal from context.
func principalFromContext(ctx context.Context) *domain.AuthenticatedPrincipal {
	p, ok := ctx.Value(principalKey).(*domain.AuthenticatedPrincipal)
	if !ok {
		return nil
	}
	return p
}
