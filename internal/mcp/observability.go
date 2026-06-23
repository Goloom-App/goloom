package mcp

import (
	"context"
	"strings"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// loggingMiddleware records every tool invocation at info level so operators can
// see agent activity in the admin log view (component "mcp", derived from this
// file's source path). Without it the MCP package emitted no info logs at all.
func (h *Handler) loggingMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method != "tools/call" {
			return next(ctx, method, req)
		}
		tool := ""
		if p, ok := req.GetParams().(*mcp.CallToolParams); ok {
			tool = p.Name
		}
		actor := mcpActor(ctx)
		start := time.Now()

		res, err := next(ctx, method, req)

		ms := time.Since(start).Milliseconds()
		if err != nil {
			h.logger.Error("mcp tool call failed", "tool", tool, "actor", actor, "duration_ms", ms, "error", err)
			return res, err
		}
		// A tool that returns a structured error sets IsError on the result.
		if r, ok := res.(*mcp.CallToolResult); ok && r.IsError {
			h.logger.Warn("mcp tool call rejected", "tool", tool, "actor", actor, "duration_ms", ms)
		} else {
			h.logger.Info("mcp tool call", "tool", tool, "actor", actor, "duration_ms", ms)
		}
		return res, err
	}
}

// audit records a team audit event for an MCP write action, mirroring the REST
// API's recordAudit so agent actions show up in the team audit log. Best-effort:
// a failure is logged but does not fail the tool call.
func (h *Handler) audit(ctx context.Context, teamID, action, targetType, targetID, summary string) {
	if strings.TrimSpace(teamID) == "" {
		return
	}
	principal := principalFromContext(ctx)
	if principal == nil {
		return
	}
	event := domain.AuditEvent{
		TeamID:      teamID,
		ActorUserID: principal.User.ID,
		ActorName:   principal.User.Name,
		ActorEmail:  principal.User.Email,
		ActorKind:   principal.Kind,
		Action:      action,
		TargetType:  targetType,
		Summary:     summary,
	}
	if strings.TrimSpace(targetID) != "" {
		event.TargetID = &targetID
	}
	if principal.Kind == domain.AuditActorToken {
		event.TokenID = principal.TokenID
		event.TokenName = principal.TokenName
	}
	if err := h.store.InsertAuditEvent(ctx, event); err != nil {
		h.logger.Error("mcp audit insert failed", "team_id", teamID, "action", action, "error", err)
	}
}

// mcpActor returns a short identifier for the calling principal for log lines.
func mcpActor(ctx context.Context) string {
	p := principalFromContext(ctx)
	if p == nil {
		return "unknown"
	}
	if p.TokenName != nil && strings.TrimSpace(*p.TokenName) != "" {
		return *p.TokenName
	}
	if p.User.Email != "" {
		return p.User.Email
	}
	return p.User.ID
}
