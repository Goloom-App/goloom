package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestRSSImportedItemRejectsForeignPost(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, team := makeAITeam(t, s)
	other, err := s.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, other.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "foreign", Content: "foreign", Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRSSImportedItem(ctx, feed.ID, "item", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRSSImportedItemPostID(ctx, team.ID, feed.ID, "item", post.ID); err == nil {
		t.Error("foreign-team post linked to RSS item")
	}
}

func TestRSSUpdatesRequireTeam(t *testing.T) {
	ctx := context.Background()
	s, db := newAIStoreWithRawDB(t)
	user, team := makeAITeam(t, s)
	_, other := makeAITeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "own", Content: "own", Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRSSImportedItem(ctx, feed.ID, "item", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.IncrementRSSFeedCounter(ctx, other.ID, feed.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign counter update: %v", err)
	}
	if err := s.UpdateRSSImportedItemPostID(ctx, other.ID, feed.ID, "item", post.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("foreign item update: %v", err)
	}
	after, err := s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
	if err != nil || after.CounterNext != feed.CounterNext {
		t.Fatalf("foreign counter update mutated feed: %+v err=%v", after, err)
	}
	var linked sql.NullString
	if err := db.QueryRowContext(ctx, "select post_id from rss_imported_items where feed_id = ? and item_key = ?", feed.ID, "item").Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if linked.Valid {
		t.Fatalf("foreign item update mutated post link: %s", linked.String)
	}
	if err := s.IncrementRSSFeedCounter(ctx, team.ID, feed.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRSSImportedItemPostID(ctx, team.ID, feed.ID, "item", post.ID); err != nil {
		t.Fatal(err)
	}
	after, err = s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
	if err != nil || after.CounterNext != feed.CounterNext+1 {
		t.Fatalf("own counter update: %+v err=%v", after, err)
	}
	if err := db.QueryRowContext(ctx, "select post_id from rss_imported_items where feed_id = ? and item_key = ?", feed.ID, "item").Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if !linked.Valid || linked.String != post.ID {
		t.Fatalf("own post link: %v", linked)
	}
}
