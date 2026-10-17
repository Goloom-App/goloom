package sqlite_test

import (
	"context"
	"git.f4mily.net/goloom/internal/domain"
	"testing"
)

func TestRSSCounterFailureRollsBackPostAndMarker(t *testing.T) {
	ctx := context.Background()
	s, db := newAIStoreWithRawDB(t)
	user, team := makeAITeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/counter", Name: "counter"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `create trigger fail_rss_counter before update of counter_next on rss_feed_configs begin select raise(abort, 'injected RSS counter failure'); end`); err != nil {
		t.Fatal(err)
	}
	input := domain.CreatePostInput{Title: "one", Content: "one", Draft: true, RSSFeedID: &feed.ID, RSSItemKey: "item"}
	if _, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, input); err == nil {
		t.Fatal("counter failure committed post")
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("rollback posts=%v err=%v", posts, err)
	}
	after, err := s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
	if err != nil || after.CounterNext != feed.CounterNext {
		t.Fatalf("rollback counter=%d err=%v", after.CounterNext, err)
	}
	if _, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil {
		t.Fatalf("rollback left linked marker: %v", err)
	}
	if _, err := db.ExecContext(ctx, `drop trigger fail_rss_counter`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, input); err != nil {
		t.Fatal(err)
	}
	after, err = s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
	if err != nil || after.CounterNext != feed.CounterNext+1 {
		t.Fatalf("healed counter=%d err=%v", after.CounterNext, err)
	}
	if _, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, input); err == nil {
		t.Fatal("duplicate committed")
	}
	after, err = s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
	if err != nil || after.CounterNext != feed.CounterNext+1 {
		t.Fatalf("duplicate incremented counter=%d err=%v", after.CounterNext, err)
	}
}
