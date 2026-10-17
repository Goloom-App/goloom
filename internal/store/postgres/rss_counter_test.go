package postgres_test

import (
	"context"
	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestRSSCounterFailureRollsBackPostAndMarker(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db, err := pgx.Connect(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(ctx) })
	user, team := newAITestTeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/counter", Name: "counter"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `create or replace function fail_rss_counter() returns trigger language plpgsql as $$ begin raise exception 'injected RSS counter failure'; end; $$; create trigger fail_rss_counter before update of counter_next on rss_feed_configs for each row execute function fail_rss_counter()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(ctx, `drop trigger if exists fail_rss_counter on rss_feed_configs; drop function if exists fail_rss_counter()`); err != nil {
			t.Errorf("cleanup counter trigger: %v", err)
		}
	})
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
	if _, err := db.Exec(ctx, `drop trigger fail_rss_counter on rss_feed_configs`); err != nil {
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
