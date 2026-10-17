package postgres_test

import (
	"context"
	"errors"
	"git.f4mily.net/goloom/internal/domain"
	"sync"
	"testing"
)

func TestRSSItemAttempts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, team := newAITestTeam(t, s)
	_, other := newAITestTeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/retry", Name: "retry"})
	if err != nil {
		t.Fatal(err)
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, other.ID, feed.ID, "item"); err != nil || won {
		t.Fatalf("foreign claim=%v err=%v", won, err)
	}
	if _, err := s.ReleaseRSSItemAttempt(ctx, other.ID, feed.ID, "item"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign release=%v", err)
	}
	if err := s.RecordRSSImportedItem(ctx, feed.ID, "baseline", ""); err != nil {
		t.Fatal(err)
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "baseline"); err != nil || won {
		t.Fatalf("baseline claimed: %v %v", won, err)
	}
	for attempt := 1; attempt <= domain.MaxRSSItemAttempts; attempt++ {
		var wg sync.WaitGroup
		wins := make(chan bool, 2)
		errs := make(chan error, 2)
		for range 2 {
			wg.Go(func() { won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item"); wins <- won; errs <- err })
		}
		wg.Wait()
		close(wins)
		close(errs)
		n := 0
		for won := range wins {
			if won {
				n++
			}
		}
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		if n != 1 {
			t.Fatalf("attempt %d: winners=%d want=1", attempt, n)
		}
		if imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, "item"); err != nil || !imported {
			t.Fatalf("active item imported=%v err=%v", imported, err)
		}
		used, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "item")
		if err != nil || used != attempt {
			t.Fatalf("release attempts=%d err=%v want=%d", used, err, attempt)
		}
		imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, "item")
		if err != nil || imported != (attempt == domain.MaxRSSItemAttempts) {
			t.Fatalf("released item imported=%v err=%v attempt=%d", imported, err, attempt)
		}
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil || won {
		t.Fatalf("exhausted claim=%v err=%v", won, err)
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "linked"); err != nil || !won {
		t.Fatal(won, err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "saved", Content: "saved", Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordRSSImportedItem(ctx, feed.ID, "linked", post.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "linked"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("linked item release=%v", err)
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "linked"); err != nil || won {
		t.Fatalf("linked claim=%v err=%v", won, err)
	}
}

func TestRSSPostInsertLinksItemAtomically(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, team := newAITestTeam(t, s)
	_, other := newAITestTeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/atomic", Name: "atomic"})
	if err != nil {
		t.Fatal(err)
	}
	input := domain.CreatePostInput{Title: "one", Content: "one", Draft: true, RSSFeedID: &feed.ID, RSSItemKey: "item"}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, input)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
	if err != nil || after.CounterNext != feed.CounterNext+1 {
		t.Fatalf("atomic counter=%d want=%d err=%v", after.CounterNext, feed.CounterNext+1, err)
	}
	if _, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "item"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("committed post released: %v", err)
	}
	if _, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, input); err == nil {
		t.Fatal("duplicate RSS post committed")
	}
	input.RSSItemKey = "foreign"
	if _, err := s.CreateScheduledPost(ctx, other.ID, domain.AuthenticatedPrincipal{User: user}, input); err == nil {
		t.Fatal("foreign RSS feed linked")
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil || len(posts) != 1 || posts[0].ID != post.ID {
		t.Fatalf("posts=%v err=%v", posts, err)
	}
	posts, err = s.ListTeamPosts(ctx, other.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("foreign post did not roll back: posts=%v err=%v", posts, err)
	}
}
