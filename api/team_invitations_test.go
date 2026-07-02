package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func TestTeamInvitationLifecycle(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	rec := f.do(t, http.MethodPost, "/v1/teams/"+f.team.ID+"/invitations", map[string]any{
		"email": "invitee@example.test",
		"role":  "editor",
	})
	requireStatus(t, rec, http.StatusCreated)
	created := decodeJSON[struct {
		Invitation domain.TeamInvitation `json:"invitation"`
		Token      string                `json:"token"`
	}](t, rec)
	if created.Token == "" || created.Invitation.Email != "invitee@example.test" {
		t.Fatalf("create response: %+v", created)
	}

	rec = f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/invitations", nil)
	requireStatus(t, rec, http.StatusOK)
	list := decodeJSON[struct {
		Items []domain.TeamInvitation `json:"items"`
	}](t, rec)
	if len(list.Items) != 1 || list.Items[0].ID != created.Invitation.ID {
		t.Fatalf("list response: %+v", list)
	}

	rec = f.do(t, http.MethodDelete, "/v1/teams/"+f.team.ID+"/invitations/"+created.Invitation.ID, nil)
	requireStatus(t, rec, http.StatusNoContent)

	rec = f.do(t, http.MethodGet, "/v1/teams/"+f.team.ID+"/invitations", nil)
	requireStatus(t, rec, http.StatusOK)
	list = decodeJSON[struct {
		Items []domain.TeamInvitation `json:"items"`
	}](t, rec)
	if len(list.Items) != 0 {
		t.Fatalf("expected empty invitation list, got %+v", list)
	}

	rec = f.do(t, http.MethodDelete, "/v1/teams/"+f.team.ID+"/invitations/"+created.Invitation.ID, nil)
	requireStatus(t, rec, http.StatusNotFound)

	events, err := f.store.ListAuditEvents(ctx, domain.AuditFilter{TeamID: f.team.ID})
	if err != nil {
		t.Fatal(err)
	}
	var haveCreate, haveRevoke bool
	for _, e := range events {
		switch e.Action {
		case "invitation.create":
			haveCreate = true
		case "invitation.revoke":
			haveRevoke = true
		}
	}
	if !haveCreate || !haveRevoke {
		t.Fatalf("expected invitation.create and invitation.revoke audit events, got %+v", events)
	}
}

func TestTeamInvitationsRequireOwner(t *testing.T) {
	f := newEndpointFixture(t)
	ctx := context.Background()

	// A viewer member must not manage invitations (routes are owner-only).
	viewer, err := f.store.UpsertOIDCUser(ctx, "inv-viewer-"+uuid.NewString(), "viewer@example.test", "Viewer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AddTeamMember(ctx, f.team.ID, domain.AddTeamMemberInput{UserID: viewer.ID, Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	viewerBearer, _, err := f.store.CreateUserAPIToken(ctx, viewer.ID, "viewer-token", nil, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	do := func(method, path string) int {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+viewerBearer)
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := do(http.MethodGet, "/v1/teams/"+f.team.ID+"/invitations"); code != http.StatusForbidden {
		t.Fatalf("viewer list status = %d, want 403", code)
	}
	if code := do(http.MethodDelete, "/v1/teams/"+f.team.ID+"/invitations/"+uuid.NewString()); code != http.StatusForbidden {
		t.Fatalf("viewer delete status = %d, want 403", code)
	}
}
