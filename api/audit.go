package api

import (
	"net/http"
	"strconv"
	"strings"

	"git.f4mily.net/goloom/internal/auth"
	"git.f4mily.net/goloom/internal/domain"
)

// recordAudit writes a best-effort team audit event for the current principal.
// It never fails the caller's action: on error it logs and returns. Call it
// after a mutation has succeeded, before writing the response (so the request
// context is still alive). targetID/summary describe the affected object.
func (a *API) recordAudit(r *http.Request, teamID, action, targetType string, targetID *string, summary string) {
	if strings.TrimSpace(teamID) == "" {
		return
	}
	principal := auth.PrincipalFromContext(r.Context())
	if principal == nil {
		return
	}
	event := domain.AuditEvent{
		TeamID:      teamID,
		ActorUserID: principal.User.ID,
		ActorName:   principal.User.Name,
		ActorEmail:  principal.User.Email,
		ActorKind:   principal.Kind,
		TokenID:     principal.TokenID,
		TokenName:   principal.TokenName,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		Summary:     summary,
	}
	if err := a.store.InsertAuditEvent(r.Context(), event); err != nil {
		a.log.Error("audit event insert failed", "team_id", teamID, "action", action, "error", err)
	}
}

// auditTitle renders a post title for an audit summary, with a placeholder when
// the post has no title.
func auditTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "(untitled)"
	}
	return title
}

// handleListTeamAuditLog returns the team's audit events (owner-only route).
func (a *API) handleListTeamAuditLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := domain.AuditFilter{
		TeamID:      r.PathValue("teamID"),
		ActorUserID: strings.TrimSpace(q.Get("actor")),
		Action:      strings.TrimSpace(q.Get("action")),
	}
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 500 {
			filter.Limit = n
		}
	}
	if raw := strings.TrimSpace(q.Get("offset")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	events, err := a.store.ListAuditEvents(r.Context(), filter)
	if err != nil {
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}
	total, err := a.store.CountAuditEvents(r.Context(), filter)
	if err != nil {
		a.writeError(w, r, "internal_error", http.StatusInternalServerError)
		return
	}
	auth.WriteJSON(w, http.StatusOK, map[string]any{
		"entries": events,
		"total":   total,
	})
}
