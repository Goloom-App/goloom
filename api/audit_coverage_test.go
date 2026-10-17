package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func auditEntry(t *testing.T, f endpointFixture, action, target string) domain.AuditEvent {
	t.Helper()
	events, err := f.store.ListAuditEvents(context.Background(), domain.AuditFilter{TeamID: f.team.ID, Action: action})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("%s events = %+v, want exactly one", action, events)
	}
	e := events[0]
	if e.ActorUserID != f.user.ID || e.ActorKind != domain.AuditActorToken || e.TeamID != f.team.ID || e.TargetID == nil || *e.TargetID != target {
		t.Fatalf("incorrect attribution: %+v", e)
	}
	if e.Metadata["channel"] != "rest" || e.Metadata["outcome"] != "success" {
		t.Fatalf("missing transport/outcome: %+v", e)
	}
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), "NEVER-AUDIT") {
		t.Fatalf("sensitive body leaked: %s", raw)
	}
	return e
}

func TestAuditReadRequiresReadScope(t *testing.T) {
	f := newEndpointFixture(t)
	token, _, err := f.store.CreateUserAPIToken(context.Background(), f.user.ID, "draft", nil, `["write:draft"]`, nil, f.team.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.bearer = token
	requireStatus(t, f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/audit-log", nil), http.StatusForbidden)
}

func TestAuditRSSAndRecurringCoverage(t *testing.T) {
	f := newEndpointFixture(t)
	base := "/v1/teams/" + f.team.ID
	feed := decodeJSON[domain.RSSFeedConfig](t, f.do(t, http.MethodPost, base+"/rss-feeds", map[string]any{"name": "feed", "feed_url": "https://example.com/NEVER-AUDIT?token=secret", "prompt_hint": "NEVER-AUDIT", "target_account_ids": []string{f.account.ID}}))
	auditEntry(t, f, "rss_feed.create", feed.ID)
	requireStatus(t, f.do(t, http.MethodPatch, base+"/rss-feeds/"+feed.ID, map[string]any{"name": "new"}), http.StatusOK)
	auditEntry(t, f, "rss_feed.update", feed.ID)
	requireStatus(t, f.do(t, http.MethodDelete, base+"/rss-feeds/"+feed.ID, nil), http.StatusNoContent)
	auditEntry(t, f, "rss_feed.delete", feed.ID)
	rec := f.do(t, http.MethodPost, base+"/post-templates", map[string]any{"title": "r", "content": "NEVER-AUDIT", "target_account_ids": []string{f.account.ID}, "recurrence_json": `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`})
	requireStatus(t, rec, http.StatusCreated)
	tmpl := decodeJSON[domain.PostTemplate](t, rec)
	auditEntry(t, f, "recurring.create", tmpl.ID)
	requireStatus(t, f.do(t, http.MethodPatch, base+"/post-templates/"+tmpl.ID, map[string]any{"content": "NEVER-AUDIT update"}), http.StatusOK)
	auditEntry(t, f, "recurring.update", tmpl.ID)
	requireStatus(t, f.do(t, http.MethodPost, base+"/post-templates/"+tmpl.ID+"/skip", map[string]any{"occurrence_at": time.Now().UTC().Add(time.Hour)}), http.StatusNoContent)
	auditEntry(t, f, "recurring.skip", tmpl.ID)
	requireStatus(t, f.do(t, http.MethodDelete, base+"/post-templates/"+tmpl.ID, nil), http.StatusNoContent)
	auditEntry(t, f, "recurring.delete", tmpl.ID)
}

func TestAuditMemberRoleAndNoop(t *testing.T) {
	f := newEndpointFixture(t)
	u, err := f.store.UpsertOIDCUser(context.Background(), "member-audit", "member@test.test", "Member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.AddTeamMember(context.Background(), f.team.ID, domain.AddTeamMemberInput{UserID: u.ID, Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	path := "/v1/teams/" + f.team.ID + "/members"
	requireStatus(t, f.do(t, http.MethodPost, path, map[string]any{"user_id": u.ID, "role": "editor"}), http.StatusCreated)
	e := auditEntry(t, f, "member.role_change", u.ID)
	if e.Metadata["role_before"] != "viewer" || e.Metadata["role_after"] != "editor" {
		t.Fatalf("role metadata: %+v", e)
	}
	requireStatus(t, f.do(t, http.MethodPost, path, map[string]any{"user_id": u.ID, "role": "editor"}), http.StatusCreated)
	auditEntry(t, f, "member.role_change", u.ID)
	count, _ := f.store.CountAuditEvents(context.Background(), domain.AuditFilter{TeamID: f.team.ID, Action: "member.add"})
	if count != 0 {
		t.Fatalf("role update emitted member.add")
	}
}

func TestAuditOtherUserChanges(t *testing.T) {
	f := newEndpointFixture(t)
	base := "/v1/teams/" + f.team.ID
	teamRec := f.do(t, http.MethodPost, "/v1/teams", map[string]any{"name": "New audit team"})
	requireStatus(t, teamRec, http.StatusCreated)
	team := decodeJSON[domain.Team](t, teamRec)
	other := f
	other.team = team
	auditEntry(t, other, "team.create", team.ID)
	requireStatus(t, f.do(t, http.MethodPut, base+"/external-post-monitor", map[string]any{"enabled": true, "retention_days": 30}), http.StatusOK)
	auditEntry(t, f, "external_monitor.update", f.team.ID)
	requireStatus(t, f.do(t, http.MethodPost, base+"/posts/draft", map[string]any{"title": "Draft", "content": "NEVER-AUDIT", "account_ids": []string{f.account.ID}}), http.StatusCreated)
	events, _ := f.store.ListAuditEvents(context.Background(), domain.AuditFilter{TeamID: f.team.ID, Action: "post.create"})
	if len(events) != 1 {
		t.Fatalf("AI draft events: %+v", events)
	}
	auditEntry(t, f, "post.create", *events[0].TargetID)
}

func TestAuditMigrationBothTeams(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	team, err := f.store.CreateTeam(ctx, f.user.ID, domain.CreateTeamInput{Name: "destination"})
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/accounts/"+f.account.ID+"/migrate", map[string]any{"target_team_id": team.ID}), http.StatusOK)
	out := auditEntry(t, f, "account.migrate_out", f.account.ID)
	other := f
	other.team = team
	in := auditEntry(t, other, "account.migrate_in", f.account.ID)
	if out.Metadata["correlation_id"] == "" || out.Metadata["correlation_id"] != in.Metadata["correlation_id"] {
		t.Fatalf("migration correlation mismatch")
	}
}

func TestAuditUntitledPostDoesNotLeakBody(t *testing.T) {
	f := newEndpointFixture(t)
	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/posts", map[string]any{"title": "NEVER-AUDIT", "content": "NEVER-AUDIT", "draft": true, "target_accounts": []string{f.account.ID}})
	requireStatus(t, rec, http.StatusCreated)
	post := decodeJSON[domain.ScheduledPost](t, rec)
	auditEntry(t, f, "post.create", post.ID)
}

func TestInvitationAcceptanceAuditActor(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()
	invitee, err := f.store.UpsertOIDCUser(ctx, "audit-invitee", "invitee@test.test", "Invitee")
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := f.store.CreateTeamInvitation(ctx, f.team.ID, f.user.ID, domain.CreateTeamInvitationInput{Email: invitee.Email, Role: domain.RoleEditor})
	if err != nil {
		t.Fatal(err)
	}
	bearer, _, err := f.store.CreateUserAPIToken(ctx, invitee.ID, "accepting actor", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	f.bearer = bearer
	f.user = invitee
	requireStatus(t, f.do(t, http.MethodPost, "/v1/invitations/accept", map[string]any{"token": token}), http.StatusOK)
	auditEntry(t, f, "invitation.accept", invitee.ID)
	requireStatus(t, f.do(t, http.MethodPost, "/v1/invitations/accept", map[string]any{"token": token}), http.StatusBadRequest)
	auditEntry(t, f, "invitation.accept", invitee.ID)
}

func TestAuditNoopAndRejectedWritesStayQuiet(t *testing.T) {
	f := newEndpointFixture(t)
	base := "/v1/teams/" + f.team.ID
	requireStatus(t, f.do(t, http.MethodPatch, base, map[string]any{"name": f.team.Name}), http.StatusOK)
	requireStatus(t, f.do(t, http.MethodDelete, base+"/members/missing", nil), http.StatusNoContent)
	requireStatus(t, f.do(t, http.MethodPost, base+"/post-templates", map[string]any{"content": "invalid"}), http.StatusBadRequest)
	requireStatus(t, f.do(t, http.MethodPut, base+"/external-post-monitor", map[string]any{"enabled": false}), http.StatusOK)
	events, err := f.store.ListAuditEvents(context.Background(), domain.AuditFilter{TeamID: f.team.ID})
	if err != nil || len(events) != 0 {
		t.Fatalf("noop/rejected success events: %+v %v", events, err)
	}
}
