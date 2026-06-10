package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func newSchemaAITestDB(t *testing.T) (*sqlite.Store, *sql.DB) {
	t.Helper()
	enc, err := security.NewEncrypter("sqlite-integration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	s, err := sqlite.New(ctx, dsn, enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return s, db
}

func TestSchemaAITables(t *testing.T) {
	ctx := context.Background()
	_, db := newSchemaAITestDB(t)
	for _, table := range []string{
		"team_profiles",
		"campaign_formats",
		"style_examples",
		"knowledge_sources",
		"ai_jobs",
		"ai_service_configs",
		"rss_feed_configs",
		"proactive_trigger_settings",
	} {
		rows, err := db.QueryContext(ctx, "SELECT 1 FROM "+table+" LIMIT 0")
		if err != nil {
			t.Fatalf("table %s: %v", table, err)
		}
		_ = rows.Close()
	}
}

func TestSchemaAITeamsColumn(t *testing.T) {
	ctx := context.Background()
	s, db := newSchemaAITestDB(t)
	owner, err := s.UpsertOIDCUser(ctx, "schema-ai-team-owner", "owner@example.com", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{Name: "ai-team-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	var enabled int
	if err := db.QueryRowContext(ctx, "select is_ai_enabled from teams where id = ?", team.ID).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 {
		t.Fatalf("expected default is_ai_enabled=0, got %d", enabled)
	}
}
