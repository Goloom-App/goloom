package postgres

import (
	"context"
	"os"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"github.com/google/uuid"
)

func migrationTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_URL to run postgres integration tests")
	}
	enc, err := security.NewEncrypter("personal-migration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, err := New(ctx, dsn, enc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedLegacyPersonalTeam(t *testing.T, s *Store, userID, name string) string {
	t.Helper()
	ctx := context.Background()
	teamID := uuid.NewString()
	if _, err := s.pool.Exec(ctx, `
		insert into teams (id, name, description, is_personal, personal_for_user_id)
		values ($1, $2, '', true, $3)`,
		teamID, name, userID,
	); err != nil {
		t.Fatalf("seed personal team: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `
		insert into team_memberships (user_id, team_id, role)
		values ($1, $2, $3)`,
		userID, teamID, domain.RoleOwner,
	); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return teamID
}

func TestPostgres_UpsertOIDCUser_noAutoTeam(t *testing.T) {
	ctx := context.Background()
	s := migrationTestStore(t)
	u, err := s.UpsertOIDCUser(ctx, "no-auto-team-"+uuid.NewString(), "noteam-"+uuid.NewString()+"@pg.test", "No Team")
	if err != nil {
		t.Fatal(err)
	}
	teams, err := s.ListTeamsForUser(ctx, u.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 0 {
		t.Fatalf("expected no auto-created team, got %+v", teams)
	}
}

func TestPostgres_MigratePersonalWorkspaces(t *testing.T) {
	ctx := context.Background()
	s := migrationTestStore(t)

	aliceName := "Alice Mig " + uuid.NewString()[:8]
	alice, err := s.UpsertOIDCUser(ctx, "mig-alice-"+uuid.NewString(), "alice-mig-"+uuid.NewString()[:8]+"@pg.test", aliceName)
	if err != nil {
		t.Fatal(err)
	}
	bobLocal := "bobmig" + uuid.NewString()[:8]
	bob, err := s.UpsertOIDCUser(ctx, "mig-bob-"+uuid.NewString(), bobLocal+"@pg.test", "")
	if err != nil {
		t.Fatal(err)
	}

	aliceTeamID := seedLegacyPersonalTeam(t, s, alice.ID, "Personal · "+alice.ID[:8])
	bobTeamID := seedLegacyPersonalTeam(t, s, bob.ID, "Personal · "+bob.ID[:8])

	if err := s.MigratePersonalWorkspaces(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	aliceTeam, err := s.GetTeamByID(ctx, aliceTeamID)
	if err != nil {
		t.Fatal(err)
	}
	if aliceTeam.Name != aliceName {
		t.Fatalf("alice team name = %q, want %q", aliceTeam.Name, aliceName)
	}
	bobTeam, err := s.GetTeamByID(ctx, bobTeamID)
	if err != nil {
		t.Fatal(err)
	}
	if bobTeam.Name != bobLocal {
		t.Fatalf("bob team name = %q, want %q", bobTeam.Name, bobLocal)
	}

	var personalCount int
	if err := s.pool.QueryRow(ctx, `select count(*) from teams where is_personal or personal_for_user_id is not null`).Scan(&personalCount); err != nil {
		t.Fatal(err)
	}
	if personalCount != 0 {
		t.Fatalf("expected no personal-flagged teams after migration, got %d", personalCount)
	}

	teams, err := s.ListTeamsForUser(ctx, alice.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 || teams[0].ID != aliceTeamID {
		t.Fatalf("alice teams after migration: %+v", teams)
	}

	if err := s.MigratePersonalWorkspaces(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestPostgres_MigratePersonalWorkspaces_nameCollision(t *testing.T) {
	ctx := context.Background()
	s := migrationTestStore(t)

	carolName := "Carol Mig " + uuid.NewString()[:8]
	carol, err := s.UpsertOIDCUser(ctx, "mig-carol-"+uuid.NewString(), "carol-mig-"+uuid.NewString()[:8]+"@pg.test", carolName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTeam(ctx, carol.ID, domain.CreateTeamInput{Name: carolName}); err != nil {
		t.Fatal(err)
	}
	personalID := seedLegacyPersonalTeam(t, s, carol.ID, "Personal · "+carol.ID[:8])

	if err := s.MigratePersonalWorkspaces(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	team, err := s.GetTeamByID(ctx, personalID)
	if err != nil {
		t.Fatal(err)
	}
	if team.Name != carolName+" 2" {
		t.Fatalf("collision rename = %q, want %q", team.Name, carolName+" 2")
	}

	daveCustom := "Daves eigener Name " + uuid.NewString()[:8]
	dave, err := s.UpsertOIDCUser(ctx, "mig-dave-"+uuid.NewString(), "dave-mig-"+uuid.NewString()[:8]+"@pg.test", "Dave")
	if err != nil {
		t.Fatal(err)
	}
	customID := seedLegacyPersonalTeam(t, s, dave.ID, daveCustom)
	if err := s.MigratePersonalWorkspaces(ctx); err != nil {
		t.Fatal(err)
	}
	custom, err := s.GetTeamByID(ctx, customID)
	if err != nil {
		t.Fatal(err)
	}
	if custom.Name != daveCustom {
		t.Fatalf("custom name changed to %q", custom.Name)
	}
}
