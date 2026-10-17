package postgres_test

import (
	"context"
	"net/url"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

func TestRSSBudgetDateMigration(t *testing.T) {
	ctx := context.Background()
	dsn, err := url.Parse(postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	query := dsn.Query()
	query.Set("timezone", "America/Los_Angeles")
	dsn.RawQuery = query.Encode()
	t.Setenv("TEST_POSTGRES_URL", dsn.String())
	s := newTestStore(t)
	db, err := pgx.Connect(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	user, team := newAITestTeam(t, s)
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
	if _, err := db.Exec(ctx, "alter table rss_imported_items drop column if exists budget_date"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "update rss_imported_items set created_at = '2026-10-16T23:30:00-03:00' where feed_id = $1", feed.ID); err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		s = newTestStore(t)
		for _, key := range []string{"reserved", "linked", "baseline"} {
			var got *string
			if err := db.QueryRow(ctx, "select budget_date::text from rss_imported_items where feed_id = $1 and item_key = $2", feed.ID, key).Scan(&got); err != nil {
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
			if _, err := db.Exec(ctx, "update rss_imported_items set created_at = '2026-10-18T00:00:00Z' where feed_id = $1", feed.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
}
