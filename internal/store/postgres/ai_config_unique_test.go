package postgres_test

import (
	"context"
	"errors"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestPostgres_AIServiceConfig_UpsertKeepsIDAndKey(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

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

func TestPostgres_AIServiceConfig_TeamIDUnique(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)
	if _, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{Provider: "openai"}); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `insert into ai_service_configs (team_id) values ($1)`, team.ID); err == nil {
		t.Fatal("expected unique violation on second config row for the same team")
	}
}

func TestPostgres_AIServiceConfig_MigrationDedupesKeepingNewest(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)

	conn, err := pgx.Connect(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	// Simulate a pre-constraint database (constraint on fresh DBs, index after migration).
	_, _ = conn.Exec(ctx, `alter table ai_service_configs drop constraint if exists ai_service_configs_team_id_key`)
	_, _ = conn.Exec(ctx, `drop index if exists ai_service_configs_team_id_key`)
	newID := uuid.NewString()
	for _, row := range []struct{ id, model, at string }{
		{uuid.NewString(), "old-model", "2024-01-01T00:00:00Z"},
		{newID, "new-model", "2025-01-01T00:00:00Z"},
		{uuid.NewString(), "mid-model", "2024-06-01T00:00:00Z"},
	} {
		if _, err := conn.Exec(ctx, `insert into ai_service_configs (id, team_id, model, created_at) values ($1, $2, $3, $4)`, row.id, team.ID, row.model, row.at); err != nil {
			t.Fatal(err)
		}
	}

	enc, err := security.NewEncrypter("postgres-integration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := postgres.New(ctx, postgresDSN(t), enc) // re-applies schema
	if err != nil {
		t.Fatalf("migration on duplicated data: %v", err)
	}
	defer s2.Close()
	got, err := s2.GetAIServiceConfig(ctx, team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != newID || got.Model != "new-model" {
		t.Fatalf("expected newest row kept, got %+v", got)
	}
	if _, err := conn.Exec(ctx, `insert into ai_service_configs (team_id) values ($1)`, team.ID); err == nil {
		t.Fatal("unique index missing after migration")
	}
}

func TestPostgres_NotFoundSentinel(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)
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
