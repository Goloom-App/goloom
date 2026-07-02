package postgres_test

import (
	"context"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func TestPostgres_ListTeamInvitations(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	owner, err := s.UpsertOIDCUser(ctx, "inv-owner-"+uuid.NewString(), "inv-owner@pg.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "inv-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "inv-other-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}

	inv1, _, err := s.CreateTeamInvitation(ctx, team.ID, owner.ID, domain.CreateTeamInvitationInput{Email: "one@pg.test", Role: domain.RoleEditor})
	if err != nil {
		t.Fatal(err)
	}
	inv2, _, err := s.CreateTeamInvitation(ctx, team.ID, owner.ID, domain.CreateTeamInvitationInput{Email: "two@pg.test", Role: domain.RoleViewer})
	if err != nil {
		t.Fatal(err)
	}

	list, err := s.ListTeamInvitations(ctx, team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 invitations, got %d", len(list))
	}
	byID := map[string]domain.TeamInvitation{list[0].ID: list[0], list[1].ID: list[1]}
	got1, ok := byID[inv1.ID]
	if !ok || got1.Email != "one@pg.test" || got1.Role != domain.RoleEditor || got1.TeamID != team.ID || got1.CreatedByUserID != owner.ID {
		t.Fatalf("invitation 1 mismatch: %+v", got1)
	}
	got2, ok := byID[inv2.ID]
	if !ok || got2.Email != "two@pg.test" || got2.Role != domain.RoleViewer {
		t.Fatalf("invitation 2 mismatch: %+v", got2)
	}
	if got1.ExpiresAt.IsZero() || got1.CreatedAt.IsZero() {
		t.Fatalf("timestamps not populated: %+v", got1)
	}

	empty, err := s.ListTeamInvitations(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no invitations for other team, got %d", len(empty))
	}
}

func TestPostgres_DeleteTeamInvitation(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	owner, err := s.UpsertOIDCUser(ctx, "del-owner-"+uuid.NewString(), "del-owner@pg.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "del-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "del-other-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	inv, _, err := s.CreateTeamInvitation(ctx, team.ID, owner.ID, domain.CreateTeamInvitationInput{Email: "gone@pg.test", Role: domain.RoleViewer})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteTeamInvitation(ctx, other.ID, inv.ID); err == nil {
		t.Fatal("expected error when deleting invitation via wrong team")
	}

	if err := s.DeleteTeamInvitation(ctx, team.ID, inv.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, err := s.ListTeamInvitations(ctx, team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected invitation removed, got %d", len(list))
	}

	if err := s.DeleteTeamInvitation(ctx, team.ID, inv.ID); err == nil {
		t.Fatal("expected error when deleting already-removed invitation")
	}
}
