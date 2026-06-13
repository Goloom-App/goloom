package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSchemaAITables(t *testing.T) {
	ctx := context.Background()
	dsn := postgresDSN(t)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Ensure schema is applied through the existing store initialization path.
	_ = newTestStore(t)

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
		rows, err := pool.Query(ctx, "SELECT 1 FROM "+table+" LIMIT 0")
		if err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
		rows.Close()
	}

	assertColumnExists := func(table, column string) {
		t.Helper()
		var found int
		err := pool.QueryRow(ctx, `
			select 1
			from information_schema.columns
			where table_schema = current_schema()
			  and table_name = $1
			  and column_name = $2
			limit 1
		`, table, column).Scan(&found)
		if err != nil {
			t.Fatalf("column %s.%s missing: %v", table, column, err)
		}
	}

	assertColumnExists("teams", "is_ai_enabled")
	assertColumnExists("api_tokens", "scopes")
	assertColumnExists("api_tokens", "team_id")
	assertColumnExists("api_tokens", "description")
}
