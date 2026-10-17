package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
)

func TestRSSConsumedMigrationPreservesLegacyMarkers(t *testing.T) {
	ctx := context.Background()
	enc, err := security.NewEncrypter("sqlite-integration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	dsn := filepath.Join(t.TempDir(), "legacy.sqlite") + "?_pragma=foreign_keys(1)"
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
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/migration", Name: "migration"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "linked"); err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "legacy AI", Content: "legacy AI", Draft: true, RSSFeedID: &feed.ID, RSSItemKey: "linked"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRSSImportedItem(ctx, feed.ID, "baseline", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "retry"); err != nil {
		t.Fatal(err)
	}
	var hasConsumed int
	if err := db.QueryRowContext(ctx, "select count(*) from pragma_table_info('rss_imported_items') where name = 'consumed_at'").Scan(&hasConsumed); err != nil {
		t.Fatal(err)
	}
	if hasConsumed != 0 {
		if _, err := db.ExecContext(ctx, "alter table rss_imported_items drop column consumed_at"); err != nil {
			t.Fatal(err)
		}
	}
	for run := 0; run < 2; run++ {
		s.Close()
		s, err = sqlite.New(ctx, dsn, enc)
		if err != nil {
			t.Fatal(err)
		}
		if run == 0 {
			if err := s.DeleteScheduledPost(ctx, team.ID, post.ID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.ExecContext(ctx, "update rss_imported_items set created_at = ? where feed_id = ?", time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano), feed.ID); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"linked", "baseline"} {
			if _, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, key); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("migration run %d legacy %s release err=%v", run, key, err)
			}
			if imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, key); err != nil || !imported {
				t.Errorf("migration run %d legacy %s imported=%v err=%v", run, key, imported, err)
			}
			if claimed, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, key); err != nil || claimed {
				t.Errorf("migration run %d legacy %s claimed=%v err=%v", run, key, claimed, err)
			}
		}
		if imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, "retry"); err != nil || imported {
			t.Errorf("migration run %d retry imported=%v err=%v", run, imported, err)
		}
		if claimed, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "retry"); err != nil || !claimed {
			t.Errorf("migration run %d retry claimed=%v err=%v", run, claimed, err)
		}
	}
}
