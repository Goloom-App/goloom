package mcp

import (
	"context"
	"log/slog"
	"net/http"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postservice"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Handler serves the MCP protocol over the Streamable HTTP transport
// (single /mcp endpoint, JSON or text/event-stream responses).
type Handler struct {
	handler   http.Handler
	store     store.Store
	auth      *auth.Service
	providers *provider.Registry
	posts     *postservice.Service
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
		posts:     postservice.New(store, providers),
		config:    cfg,
		logger:    logger,
	}

	// Streamable HTTP transport in stateless mode: every request is
	// self-contained and auto-initialized, so we don't depend on the client (or
	// an intermediary proxy) preserving the Mcp-Session-Id across calls — which
	// otherwise makes follow-up calls fail with "method ... is invalid during
	// session initialization". This fits goloom's tools, which are pure
	// request/response with per-request bearer auth and no server->client
	// messages, and it removes the need for session affinity behind a proxy.
	h.handler = mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return h.createServer(r)
	}, &mcp.StreamableHTTPOptions{Logger: logger, Stateless: true})
	return h
}

// createServer creates a new MCP server for each request (with tools registered).
func (h *Handler) createServer(r *http.Request) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "goloom",
		Version: "1.0.0",
	}, nil)

	server.AddReceivingMiddleware(h.loggingMiddleware)
	h.registerTools(server)
	return server
}

// ServeHTTP authenticates the request, then delegates to the Streamable HTTP
// transport (GET opens the optional SSE stream, POST carries JSON-RPC).
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

	// 3. Require at least read access. Unscoped tokens (and browser sessions)
	// pass; per-tool write/delete scopes are checked inside the tool handlers.
	if !auth.PrincipalAllows(principal, auth.ScopeRead) {
		http.Error(w, "scope read required", http.StatusForbidden)
		return
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
