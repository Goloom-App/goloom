package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestAIJobOriginLegacyMigration(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacy := strings.Replace(schemaSQL, "    origin text not null default 'user',\n", "", 1)
	if _, err = db.ExecContext(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `insert into users(id,subject,email,name,created_at,updated_at) values('u','u','u','u','2026-01-01','2026-01-01'); insert into teams(id,name,created_at) values('t','t','2026-01-01'); insert into ai_jobs(id,team_id,author_user_id,job_type,payload,created_at,updated_at) values('j','t','u','voice_engine','{"rss_automation":{}}','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = applySQLiteLegacyMigrations(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	var origin string
	if err = db.QueryRowContext(ctx, `select origin from ai_jobs where id='j'`).Scan(&origin); err != nil {
		t.Fatal(err)
	}
	if origin != "user" {
		t.Fatalf("legacy origin=%q, want user", origin)
	}
}
