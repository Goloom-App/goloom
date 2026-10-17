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
	// Audit persists evidence in the invocation transaction. Nil uses Store.
	Audit func(ctx context.Context, event domain.AuditEvent) error
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
	name    string
	desc    string
	scope   string // "" means read (relies on the connection-level read gate)
	confirm bool   // chat: propose the action instead of executing it
	// confirmIf makes the chat gate conditional: the call is proposed only when
	// it returns true (with a human-readable reason). Ignored when confirm is set.
	confirmIf  confirmFn
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

	// confirmIf, when set, gates the chat call per invocation (see spec).
	confirmIf confirmFn

	inputSchema json.RawMessage
	exec        func(ctx context.Context, d Deps, inv Invocation, raw json.RawMessage) (Result, error)
	registerMCP func(server *mcp.Server, d Deps)
}

// confirmFn decides whether a specific chat invocation must be confirmed by the
// user. It must fail closed: when it cannot tell, it returns true.
type confirmFn func(ctx context.Context, d Deps, args json.RawMessage) (needs bool, reason string)

// NeedsConfirmation reports whether a chat call with these args must be
// proposed to the user instead of executed, plus a short reason for the card.
func (t *Tool) NeedsConfirmation(ctx context.Context, d Deps, args json.RawMessage) (bool, string) {
	if t.Confirm {
		return true, ""
	}
	if t.confirmIf != nil {
		return t.confirmIf(ctx, d, args)
	}
	return false, ""
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
		confirmIf:   s.confirmIf,
		Transports:  s.transports,
		inputSchema: mustSchema[In](s.name),
	}
	invoke := func(ctx context.Context, d Deps, inv Invocation, in In) (out Out, err error) {
		if s.scope == "" {
			return core(ctx, d, inv, in)
		}
		err = d.Store.WithAuditTransaction(ctx, func(ctx context.Context) error {
			out, err = core(ctx, d, inv, in)
			return err
		})
		return out, err
	}
	t.exec = func(ctx context.Context, d Deps, inv Invocation, raw json.RawMessage) (Result, error) {
		var in In
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &in); err != nil {
				return Result{}, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		out, err := invoke(ctx, d, inv, in)
		if err != nil {
			return Result{}, err
		}
		// The marshalled output is the structured payload the chat UI renders
		// (e.g. a created draft) and, by default, the textual result for the
		// model too. A tool whose output implements ToolSummary can override the
		// model-facing text — e.g. to signal that the call is terminal so the
		// agent stops instead of re-calling it.
		marshalled, _ := json.Marshal(out)
		summary := string(marshalled)
		if s, ok := any(out).(interface{ ToolSummary() string }); ok {
			if custom := s.ToolSummary(); custom != "" {
				summary = custom
			}
		}
		return Result{Summary: summary, Payload: marshalled}, nil
	}
	t.registerMCP = func(server *mcp.Server, d Deps) {
		mcp.AddTool(server, &mcp.Tool{Name: s.name, Description: s.desc},
			func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
				inv := Invocation{Transport: TransportMCP}
				if p := PrincipalFromContext(ctx); p != nil {
					inv.Principal = *p
				}
				out, err := invoke(ctx, d, inv, in)
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
// team audit log regardless of transport; failures roll back the invocation.
func (d Deps) recordAudit(ctx context.Context, inv Invocation, teamID, action, targetType, targetID, summary string) error {
	if strings.TrimSpace(teamID) == "" {
		return nil
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
		Metadata:    map[string]string{"channel": string(inv.Transport), "outcome": "success"},
	}
	if strings.TrimSpace(targetID) != "" {
		event.TargetID = &targetID
	}
	if p.Kind == domain.AuditActorToken {
		event.TokenID = p.TokenID
		event.TokenName = p.TokenName
	}
	if d.Audit != nil {
		return d.Audit(ctx, event)
	}
	return d.Store.InsertAuditEvent(ctx, event)
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
