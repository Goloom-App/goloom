// Package agenttools is the single source of truth for the actions the Goloom
// agent can perform. Each tool is defined exactly once as a transport-agnostic
// core function plus metadata; thin adapters expose the same tools over the MCP
// server (for external API-token clients) and over the in-app chat assistant.
//
// Defining tools once keeps the two surfaces from drifting apart: a tool added
// here is automatically available to both, and its JSON schema is generated from
// the typed input struct so MCP and the chat LLM see identical contracts.
package agenttools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/postservice"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store"
)

// Deps are the shared services every agent tool depends on. The same Deps back
// both the MCP adapter and the chat adapter, so business logic is never
// duplicated between transports.
type Deps struct {
	Store     store.Store
	Auth      *auth.Service
	Posts     *postservice.Service
	Providers *provider.Registry
	Logger    *slog.Logger
	// Audit records a team audit event for a write action. Best-effort: a nil
	// sink (or one that fails) must never fail the tool call.
	Audit func(ctx context.Context, event domain.AuditEvent)
}

// Transport identifies which surface a tool is exposed on.
type Transport string

const (
	TransportMCP  Transport = "mcp"
	TransportChat Transport = "chat"
)

var (
	transportsShared   = []Transport{TransportMCP, TransportChat}
	transportsMCPOnly  = []Transport{TransportMCP}
	transportsChatOnly = []Transport{TransportChat}
)

// Invocation carries the per-call context that the core functions need but that
// is not part of the tool's typed input: who is calling, over which transport,
// and (chat only) what the user is currently looking at.
type Invocation struct {
	Principal   domain.AuthenticatedPrincipal
	Transport   Transport
	ViewContext json.RawMessage
}

// Result is what a tool returns to a caller. Summary is the textual result fed
// back to the chat model; Payload is an optional structured object forwarded to
// the chat UI (e.g. a created draft for the preview card, or a confirmation
// request for a write that needs the user's go-ahead).
type Result struct {
	Summary string
	Payload json.RawMessage
}

// coreFn is the transport-agnostic implementation of a tool.
type coreFn[In, Out any] func(ctx context.Context, d Deps, inv Invocation, in In) (Out, error)

// spec is the static metadata for a tool.
type spec struct {
	name       string
	desc       string
	scope      string // "" means read (relies on the connection-level read gate)
	confirm    bool   // chat: propose the action instead of executing it
	transports []Transport
}

// Tool is a type-erased registry entry. It keeps a generated JSON schema, a raw
// executor used by the chat adapter, and a typed registration closure used by
// the MCP adapter.
type Tool struct {
	Name        string
	Description string
	Scope       string
	Confirm     bool
	Transports  []Transport

	inputSchema json.RawMessage
	exec        func(ctx context.Context, d Deps, inv Invocation, raw json.RawMessage) (Result, error)
	registerMCP func(server *mcp.Server, d Deps)
}

// Exposes reports whether the tool is available on the given transport.
func (t *Tool) Exposes(tr Transport) bool {
	for _, x := range t.Transports {
		if x == tr {
			return true
		}
	}
	return false
}

// define builds a registry entry from a typed core function. The input JSON
// schema is generated from In with the same library the MCP SDK uses, so the
// MCP and chat contracts stay identical and stay in sync with the Go types.
func define[In, Out any](s spec, core coreFn[In, Out]) *Tool {
	t := &Tool{
		Name:        s.name,
		Description: s.desc,
		Scope:       s.scope,
		Confirm:     s.confirm,
		Transports:  s.transports,
		inputSchema: mustSchema[In](s.name),
	}
	t.exec = func(ctx context.Context, d Deps, inv Invocation, raw json.RawMessage) (Result, error) {
		var in In
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &in); err != nil {
				return Result{}, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		out, err := core(ctx, d, inv, in)
		if err != nil {
			return Result{}, err
		}
		summary, _ := json.Marshal(out)
		return Result{Summary: string(summary)}, nil
	}
	t.registerMCP = func(server *mcp.Server, d Deps) {
		mcp.AddTool(server, &mcp.Tool{Name: s.name, Description: s.desc},
			func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
				inv := Invocation{Transport: TransportMCP}
				if p := PrincipalFromContext(ctx); p != nil {
					inv.Principal = *p
				}
				out, err := core(ctx, d, inv, in)
				if err != nil {
					var zero Out
					return nil, zero, err
				}
				return nil, out, nil
			})
	}
	return t
}

func mustSchema[In any](name string) json.RawMessage {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("agenttools: schema for %q: %v", name, err))
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("agenttools: marshal schema for %q: %v", name, err))
	}
	return raw
}

// ===== Auth helpers (shared by every team-scoped tool) =====

// requireScope enforces a token scope for a write/delete action. Read tools rely
// on the connection-level gate. Unscoped tokens and browser sessions pass.
func requireScope(inv Invocation, scope string) error {
	if !auth.PrincipalAllows(inv.Principal, scope) {
		return fmt.Errorf("forbidden: scope %q required", scope)
	}
	return nil
}

// requireTeam checks that the principal may act on the team with one of the
// given roles.
func requireTeam(ctx context.Context, d Deps, inv Invocation, teamID string, roles ...domain.TeamRole) error {
	allowed, err := d.Auth.PrincipalHasTeamAccess(ctx, inv.Principal, teamID, roles...)
	if err != nil || !allowed {
		return fmt.Errorf("forbidden")
	}
	return nil
}

// recordAudit mirrors the REST API's audit so agent write actions show up in the
// team audit log regardless of transport. Best-effort.
func (d Deps) recordAudit(ctx context.Context, inv Invocation, teamID, action, targetType, targetID, summary string) {
	if d.Audit == nil || strings.TrimSpace(teamID) == "" {
		return
	}
	p := inv.Principal
	event := domain.AuditEvent{
		TeamID:      teamID,
		ActorUserID: p.User.ID,
		ActorName:   p.User.Name,
		ActorEmail:  p.User.Email,
		ActorKind:   p.Kind,
		Action:      action,
		TargetType:  targetType,
		Summary:     summary,
	}
	if strings.TrimSpace(targetID) != "" {
		event.TargetID = &targetID
	}
	if p.Kind == domain.AuditActorToken {
		event.TokenID = p.TokenID
		event.TokenName = p.TokenName
	}
	d.Audit(ctx, event)
}

// ===== Principal context (shared between adapters) =====

type contextKey string

const principalKey contextKey = "agenttools.principal"

// WithPrincipal stores the authenticated principal in context (as a pointer so
// the assertion in PrincipalFromContext always matches).
func WithPrincipal(ctx context.Context, principal domain.AuthenticatedPrincipal) context.Context {
	return context.WithValue(ctx, principalKey, &principal)
}

// PrincipalFromContext extracts the authenticated principal from context.
func PrincipalFromContext(ctx context.Context) *domain.AuthenticatedPrincipal {
	p, _ := ctx.Value(principalKey).(*domain.AuthenticatedPrincipal)
	return p
}
