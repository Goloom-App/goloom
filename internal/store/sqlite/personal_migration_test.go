package sqlite

import (
	"context"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"github.com/google/uuid"
)

func migrationTestStore(t *testing.T) *Store {
	t.Helper()
	enc, err := security.NewEncrypter("personal-migration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	s, err := New(ctx, dsn, enc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// seedLegacyPersonalTeam inserts a pre-migration personal workspace the way
// the removed EnsurePersonalTeam used to create it.
func seedLegacyPersonalTeam(t *testing.T, s *Store, userID, name string) string {
	t.Helper()
	ctx := context.Background()
	teamID := uuid.NewString()
	now := nowString()
	if _, err := s.db.ExecContext(ctx, `
		insert into teams (id, name, description, is_personal, personal_for_user_id, created_at)
		values (?, ?, '', 1, ?, ?)`,
		teamID, name, userID, now,
	); err != nil {
		t.Fatalf("seed personal team: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		insert into team_memberships (user_id, team_id, role, created_at)
		values (?, ?, ?, ?)`,
		userID, teamID, domain.RoleOwner, now,
	); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return teamID
}

func TestSQLite_UpsertOIDCUser_noAutoTeam(t *testing.T) {
	ctx := context.Background()
	s := migrationTestStore(t)
	u, err := s.UpsertOIDCUser(ctx, "no-auto-team", "noteam@x", "No Team")
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

func TestSQLite_MigratePersonalWorkspaces(t *testing.T) {
	ctx := context.Background()
	s := migrationTestStore(t)

	alice, err := s.UpsertOIDCUser(ctx, "mig-alice", "alice@example.test", "Alice Example")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := s.UpsertOIDCUser(ctx, "mig-bob", "bob@example.test", "")
	if err != nil {
		t.Fatal(err)
	}

	aliceTeamID := seedLegacyPersonalTeam(t, s, alice.ID, "Personal · "+alice.ID[:8])
	bobTeamID := seedLegacyPersonalTeam(t, s, bob.ID, "Personal · "+bob.ID[:8])

	if err := s.MigratePersonalWorkspaces(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Auto-generated names are replaced with the user's display name (or the
	// email local part when the name is empty); the personal markers are gone.
	aliceTeam, err := s.GetTeamByID(ctx, aliceTeamID)
	if err != nil {
		t.Fatal(err)
	}
	if aliceTeam.Name != "Alice Example" {
		t.Fatalf("alice team name = %q, want %q", aliceTeam.Name, "Alice Example")
	}
	bobTeam, err := s.GetTeamByID(ctx, bobTeamID)
	if err != nil {
		t.Fatal(err)
	}
	if bobTeam.Name != "bob" {
		t.Fatalf("bob team name = %q, want %q", bobTeam.Name, "bob")
	}

	var personalCount int
	if err := s.db.QueryRowContext(ctx, `select count(*) from teams where is_personal = 1 or personal_for_user_id is not null`).Scan(&personalCount); err != nil {
		t.Fatal(err)
	}
	if personalCount != 0 {
		t.Fatalf("expected no personal-flagged teams after migration, got %d", personalCount)
	}

	// Memberships survive and the former personal team behaves like any team.
	teams, err := s.ListTeamsForUser(ctx, alice.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(teams) != 1 || teams[0].ID != aliceTeamID {
		t.Fatalf("alice teams after migration: %+v", teams)
	}

	// Running it again is a no-op.
	if err := s.MigratePersonalWorkspaces(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestSQLite_MigratePersonalWorkspaces_nameCollision(t *testing.T) {
	ctx := context.Background()
	s := migrationTestStore(t)

	carol, err := s.UpsertOIDCUser(ctx, "mig-carol", "carol@example.test", "Carol")
	if err != nil {
		t.Fatal(err)
	}
	// A regular team already owns the preferred name.
	if _, err := s.CreateTeam(ctx, carol.ID, domain.CreateTeamInput{Name: "Carol"}); err != nil {
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
	if team.Name != "Carol 2" {
		t.Fatalf("collision rename = %q, want %q", team.Name, "Carol 2")
	}

	// Custom names (not the auto-generated pattern) are kept as-is.
	dave, err := s.UpsertOIDCUser(ctx, "mig-dave", "dave@example.test", "Dave")
	if err != nil {
		t.Fatal(err)
	}
	customID := seedLegacyPersonalTeam(t, s, dave.ID, "Daves eigener Name")
	if err := s.MigratePersonalWorkspaces(ctx); err != nil {
		t.Fatal(err)
	}
	custom, err := s.GetTeamByID(ctx, customID)
	if err != nil {
		t.Fatal(err)
	}
	if custom.Name != "Daves eigener Name" {
		t.Fatalf("custom name changed to %q", custom.Name)
	}
}
