package mcp

import (
	"context"
	"log/slog"
	"net/http"

	"git.f4mily.net/goloom/internal/agenttools"
	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postservice"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Handler serves the MCP protocol over the Streamable HTTP transport
// (single /mcp endpoint, JSON or text/event-stream responses). The tools
// themselves live in the agenttools catalog, shared with the in-app chat
// assistant; this handler only authenticates the request and registers that
// catalog onto a per-request MCP server.
type Handler struct {
	handler http.Handler
	store   store.Store
	logger  *slog.Logger
	deps    agenttools.Deps
}

// NewHandler creates a new MCP handler with all tools registered.
func NewHandler(
	logger *slog.Logger,
	dataStore store.Store,
	authSvc *auth.Service,
	providers *provider.Registry,
	_ config.Config,
) *Handler {
	h := &Handler{
		store:  dataStore,
		logger: logger,
		deps: agenttools.Deps{
			Store:     dataStore,
			Auth:      authSvc,
			Posts:     postservice.New(dataStore, providers),
			Providers: providers,
			Logger:    logger,
			// Mirror the REST API's audit so agent actions show up in the team
			// audit log. Best-effort: a failure is logged, never fatal.
			Audit: func(ctx context.Context, event domain.AuditEvent) {
				if err := dataStore.InsertAuditEvent(ctx, event); err != nil {
					logger.Error("mcp audit insert failed", "team_id", event.TeamID, "action", event.Action, "error", err)
				}
			},
		},
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
func (h *Handler) createServer(_ *http.Request) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "goloom",
		Version: "1.0.0",
	}, nil)

	server.AddReceivingMiddleware(h.loggingMiddleware)
	agenttools.RegisterMCP(server, h.deps)
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

	// 4. Store principal in context for the agent tools to read.
	r = r.WithContext(agenttools.WithPrincipal(r.Context(), principal))

	// 5. Delegate to the Streamable HTTP handler.
	h.handler.ServeHTTP(w, r)
}
