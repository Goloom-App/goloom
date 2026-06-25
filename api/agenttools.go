package api

import (
	"context"

	"git.f4mily.net/goloom/internal/agenttools"
	"git.f4mily.net/goloom/internal/domain"
)

// agentDeps builds the shared dependency set the agent tool catalog needs. The
// in-app chat assistant uses it to expose the same tools the MCP server does.
func (a *API) agentDeps() agenttools.Deps {
	return agenttools.Deps{
		Store:     a.store,
		Auth:      a.auth,
		Posts:     a.posts,
		Providers: a.providers,
		Logger:    a.log,
		Audit: func(ctx context.Context, event domain.AuditEvent) {
			if err := a.store.InsertAuditEvent(ctx, event); err != nil {
				a.log.Error("agent audit insert failed", "team_id", event.TeamID, "action", event.Action, "error", err)
			}
		},
	}
}
