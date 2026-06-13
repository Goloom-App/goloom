package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestTeamAuditLogEndpoint(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	// An action performed via the API token must be recorded and attributed.
	requireStatus(t, f.do(t, http.MethodPatch, "/v1/teams/"+f.team.ID, map[string]any{"name": "Renamed"}), http.StatusOK)

	rec := f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/audit-log", nil)
	requireStatus(t, rec, http.StatusOK)
	resp := decodeJSON[struct {
		Entries []domain.AuditEvent `json:"entries"`
		Total   int                 `json:"total"`
	}](t, rec)
	if resp.Total == 0 || len(resp.Entries) == 0 {
		t.Fatal("expected an audit entry for the team update")
	}

	var found *domain.AuditEvent
	for i := range resp.Entries {
		if resp.Entries[i].Action == "team.update" {
			found = &resp.Entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("no team.update event recorded: %+v", resp.Entries)
	}
	if found.ActorUserID != f.user.ID {
		t.Errorf("actor user = %q, want %q", found.ActorUserID, f.user.ID)
	}
	// The fixture authenticates with a named API token ("endpoints").
	if found.ActorKind != domain.AuditActorToken {
		t.Errorf("actor kind = %q, want api_token", found.ActorKind)
	}
	if found.TokenName == nil || *found.TokenName != "endpoints" {
		t.Errorf("token name = %v, want 'endpoints'", found.TokenName)
	}

	// A non-owner (viewer) must not be able to read the audit log.
	viewer, err := f.store.UpsertOIDCUser(ctx, "viewer-"+f.user.ID, "viewer@example.test", "Viewer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AddTeamMember(ctx, f.team.ID, domain.AddTeamMemberInput{UserID: viewer.ID, Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	viewerToken, _, err := f.store.CreateUserAPIToken(ctx, viewer.ID, "viewer-token", nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/teams/"+f.team.ID+"/audit-log", nil)
	req.Header.Set("Authorization", "Bearer "+viewerToken)
	rec2 := httptest.NewRecorder()
	f.handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("viewer audit-log access = %d, want 403", rec2.Code)
	}
}

func TestTeamAuditLogTokenLifecycle(t *testing.T) {
	f := newEndpointFixture(t)

	// Mint a team-scoped API key, then revoke it.
	rec := f.do(t, http.MethodPost, "/v1/me/api-tokens", map[string]any{"name": "team-bot", "team_id": f.team.ID})
	requireStatus(t, rec, http.StatusCreated)
	created := decodeJSON[struct {
		APIToken domain.APIToken `json:"api_token"`
	}](t, rec)
	requireStatus(t, f.do(t, http.MethodDelete, "/v1/me/api-tokens/"+created.APIToken.ID, nil), http.StatusNoContent)

	// The revoke must appear in the team audit log.
	rec = f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/audit-log?action=api_token.revoke", nil)
	requireStatus(t, rec, http.StatusOK)
	resp := decodeJSON[struct {
		Entries []domain.AuditEvent `json:"entries"`
	}](t, rec)
	if len(resp.Entries) == 0 || resp.Entries[0].Action != "api_token.revoke" {
		t.Fatalf("expected api_token.revoke audit event, got %+v", resp.Entries)
	}
}
