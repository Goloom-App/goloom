package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestKnowledgeSelectionLegacyMigration(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacy := strings.ReplaceAll(schemaSQL, "    knowledge_source_ids text not null default 'null',\n", "")
	if _, err := db.ExecContext(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `insert into teams(id,name,created_at) values('team','Team','2026-01-01'); insert into campaign_formats(id,team_id,name) values('format','team','Format'); insert into rss_feed_configs(id,team_id,feed_url,name) values('feed','team','https://example.org/rss','Feed')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := applySQLiteLegacyMigrations(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"campaign_formats", "rss_feed_configs"} {
		var got string
		if err := db.QueryRowContext(ctx, "select knowledge_source_ids from "+table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != "null" {
			t.Fatalf("%s legacy selection %q must inherit", table, got)
		}
	}
}
