package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

func TestRSSConsumedMigrationPreservesLegacyMarkers(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db, err := pgx.Connect(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	user, team := newAITestTeam(t, s)
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
	if _, err := db.Exec(ctx, "alter table rss_imported_items drop column if exists consumed_at"); err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		s = newTestStore(t)
		if run == 0 {
			if err := s.DeleteScheduledPost(ctx, team.ID, post.ID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.Exec(ctx, "update rss_imported_items set created_at = $1 where feed_id = $2", time.Now().UTC().Add(-time.Hour), feed.ID); err != nil {
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
