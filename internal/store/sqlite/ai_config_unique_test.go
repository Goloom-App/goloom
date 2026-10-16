package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func TestSQLite_AIServiceConfig_UpsertKeepsIDAndKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)

	first, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{Provider: "openai", Model: "a", APIKey: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{Provider: "anthropic", Model: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("id changed on upsert: %q -> %q", first.ID, second.ID)
	}
	if second.APIKey != "k1" || !second.APIKeySet {
		t.Fatalf("empty api key must keep stored key, got %q set=%v", second.APIKey, second.APIKeySet)
	}
	if second.Provider != "anthropic" || second.Model != "b" {
		t.Fatalf("fields not updated: %+v", second)
	}
}

func TestSQLite_AIServiceConfig_TeamIDUnique(t *testing.T) {
	ctx := context.Background()
	s, db := newAIStoreWithRawDB(t)
	_, team := makeAITeam(t, s)
	if _, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{Provider: "openai"}); err != nil {
		t.Fatal(err)
	}
	_, err := db.ExecContext(ctx, `insert into ai_service_configs (id, team_id) values (?, ?)`, uuid.NewString(), team.ID)
	if err == nil {
		t.Fatal("expected unique violation on second config row for the same team")
	}
}

func TestSQLite_AIServiceConfig_MigrationDedupesKeepingNewest(t *testing.T) {
	ctx := context.Background()
	enc, err := security.NewEncrypter("sqlite-integration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	s, err := sqlite.New(ctx, dsn, enc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	_, team := makeAITeam(t, s)

	// Simulate a pre-constraint database: drop the index, insert duplicates.
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(ctx, `drop index if exists ux_ai_service_configs_team`); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, model, at string }{
		{"old", "old-model", "2024-01-01T00:00:00.000000000Z"},
		{"new", "new-model", "2025-01-01T00:00:00.000000000Z"},
		{"mid", "mid-model", "2024-06-01T00:00:00.000000000Z"},
	} {
		if _, err := db.ExecContext(ctx, `insert into ai_service_configs (id, team_id, model, created_at) values (?, ?, ?, ?)`, row.id, team.ID, row.model, row.at); err != nil {
			t.Fatal(err)
		}
	}

	s3, err := sqlite.New(ctx, dsn, enc) // re-runs migrations
	if err != nil {
		t.Fatalf("migration on duplicated data: %v", err)
	}
	t.Cleanup(func() { s3.Close() })
	got, err := s3.GetAIServiceConfig(ctx, team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "new" || got.Model != "new-model" {
		t.Fatalf("expected newest row kept, got %+v", got)
	}
	if _, err := db.ExecContext(ctx, `insert into ai_service_configs (id, team_id) values ('dup', ?)`, team.ID); err == nil {
		t.Fatal("unique index missing after migration")
	}
}

func TestSQLite_NotFoundSentinel(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := makeAITeam(t, s)
	if _, err := s.GetTeamProfile(ctx, team.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetTeamProfile: want ErrNotFound, got %v", err)
	}
	if _, err := s.GetProactiveTriggerSettings(ctx, team.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetProactiveTriggerSettings: want ErrNotFound, got %v", err)
	}
	if _, err := s.GetAIServiceConfig(ctx, team.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetAIServiceConfig: want ErrNotFound, got %v", err)
	}
	if _, err := s.GetCampaignFormatByID(ctx, team.ID, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetCampaignFormatByID: want ErrNotFound, got %v", err)
	}
	if _, err := s.GetKnowledgeSourceByID(ctx, team.ID, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetKnowledgeSourceByID: want ErrNotFound, got %v", err)
	}
}
