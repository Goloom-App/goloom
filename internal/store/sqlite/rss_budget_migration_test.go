package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
)

func TestRSSBudgetDateMigration(t *testing.T) {
	ctx := context.Background()
	enc, err := security.NewEncrypter("sqlite-integration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	dsn := filepath.Join(t.TempDir(), "budget.sqlite") + "?_pragma=foreign_keys(1)"
	s, err := sqlite.New(ctx, dsn, enc)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	user, team := makeAITeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/budget-migration", Name: "migration"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "reserved"); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRSSImportedItem(ctx, feed.ID, "baseline", ""); err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "linked", Content: "linked", Draft: true, RSSFeedID: &feed.ID, RSSItemKey: "linked"})
	if err != nil {
		t.Fatal(err)
	}
	var exists int
	if err := db.QueryRowContext(ctx, "select count(*) from pragma_table_info('rss_imported_items') where name='budget_date'").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists != 0 {
		if _, err := db.ExecContext(ctx, "alter table rss_imported_items drop column budget_date"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "update rss_imported_items set created_at = '2026-10-16T23:30:00-03:00' where feed_id = ?", feed.ID); err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		s.Close()
		s, err = sqlite.New(ctx, dsn, enc)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"reserved", "linked", "baseline"} {
			var got *string
			if err := db.QueryRowContext(ctx, "select budget_date from rss_imported_items where feed_id = ? and item_key = ?", feed.ID, key).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if key == "baseline" {
				if got != nil {
					t.Errorf("baseline charged: %s", *got)
				}
			} else if got == nil || *got != "2026-10-17" {
				t.Errorf("run %d key %s budget=%v, want UTC 2026-10-17", run, key, got)
			}
		}
		if run == 0 {
			if err := s.DeleteScheduledPost(ctx, team.ID, post.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "update rss_imported_items set created_at = '2026-10-18T00:00:00Z' where feed_id = ?", feed.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
}
