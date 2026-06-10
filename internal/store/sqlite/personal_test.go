package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/security"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestMigrateSQLiteEmbeddedAnnouncements_promotesOrphanChild(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		t.Fatal(err)
	}
	if err := applySQLiteLegacyMigrations(ctx, db); err != nil {
		t.Fatal(err)
	}

	teamID := uuid.NewString()
	userID := uuid.NewString()
	now := nowString()
	if _, err := db.ExecContext(ctx, `
		insert into users (id, subject, email, name, is_admin, created_at, updated_at)
		values (?, ?, ?, ?, 1, ?, ?)`,
		userID, "sub-"+userID, "m@test", "M", now, now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		insert into teams (id, name, description, created_at)
		values (?, ?, '', ?)`,
		teamID, "team-"+teamID, now,
	); err != nil {
		t.Fatal(err)
	}

	missingParentID := uuid.NewString()
	childID := uuid.NewString()
	rec := `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`
	if _, err := db.ExecContext(ctx, `
		insert into post_templates (
			id, team_id, author_user_id, title, content, recurrence_json, visibility,
			media_ids, media_exclude_by_account, target_account_ids, enabled,
			next_materialize_at, counter_next, announces_template_id, created_at, updated_at
		) values (?, ?, ?, ?, ?, ?, 'public', '[]', '{}', '[]', 1, ?, 1, ?, ?, ?)`,
		childID, teamID, userID, "Announce title", "Announce body", rec,
		formatTime(time.Now().UTC().Add(24*time.Hour)), missingParentID, now, now,
	); err != nil {
		t.Fatal(err)
	}

	if err := migrateSQLiteEmbeddedAnnouncements(ctx, db); err != nil {
		t.Fatal(err)
	}

	var announcesID sql.NullString
	var title string
	if err := db.QueryRowContext(ctx, `
		select announces_template_id, title from post_templates where id = ?`, childID,
	).Scan(&announcesID, &title); err != nil {
		t.Fatal(err)
	}
	if announcesID.Valid && strings.TrimSpace(announcesID.String) != "" {
		t.Fatalf("orphan child should be promoted, announces_template_id=%v", announcesID)
	}
	if title != "Announce title" {
		t.Fatalf("title=%q", title)
	}

	enc, err := security.NewEncrypter("sqlite-migrate-test-secret-32bytes")
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(ctx, dsn, enc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	list, err := s.ListPostTemplates(ctx, teamID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != childID {
		t.Fatalf("list=%+v", list)
	}
}
