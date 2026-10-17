package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

func TestRSSDailyBudgetReservationLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, team := newAITestTeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/budget", Name: "budget", MaxPostsPerDay: 1})
	if err != nil {
		t.Fatal(err)
	}
	count := func(want int) {
		t.Helper()
		got, err := s.CountRSSFeedPostsToday(ctx, feed.ID)
		if err != nil || got != want {
			t.Fatalf("daily budget=%d want=%d err=%v", got, want, err)
		}
	}
	if err := s.RecordRSSImportedItem(ctx, feed.ID, "baseline", ""); err != nil {
		t.Fatal(err)
	}
	count(0)
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "first"); err != nil || !won {
		t.Fatal(won, err)
	}
	count(1)
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "second"); won || !errors.Is(err, domain.ErrRSSDailyLimit) {
		t.Fatalf("full budget claim=%v err=%v", won, err)
	}
	// Reaching the daily limit must still allow the poll timestamp to advance.
	fetchedAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := s.UpdateRSSFeedLastFetched(ctx, feed.ID, fetchedAt); err != nil {
		t.Fatal(err)
	}
	current, err := s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
	if err != nil || current.LastFetchedAt == nil || !current.LastFetchedAt.Equal(fetchedAt) {
		t.Fatalf("last fetched=%v want=%v err=%v", current.LastFetchedAt, fetchedAt, err)
	}
	count(1)
	if _, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "first"); err != nil {
		t.Fatal(err)
	}
	count(0)
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "second"); err != nil || !won {
		t.Fatal(won, err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "saved", Content: "saved", Draft: true, RSSFeedID: &feed.ID, RSSItemKey: "second"})
	if err != nil {
		t.Fatal(err)
	}
	count(1)
	if err := s.DeleteScheduledPost(ctx, team.ID, post.ID); err != nil {
		t.Fatal(err)
	}
	count(1)
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "third"); won || !errors.Is(err, domain.ErrRSSDailyLimit) {
		t.Fatalf("consumed budget claim=%v err=%v", won, err)
	}
}

func TestRSSDailyBudgetConcurrentClaims(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/concurrent-budget", Name: "budget", MaxPostsPerDay: 1})
	if err != nil {
		t.Fatal(err)
	}
	const workers = 12
	start := make(chan struct{})
	results := make(chan bool, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			<-start
			won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, fmt.Sprintf("item-%d", i))
			if err != nil && !errors.Is(err, domain.ErrRSSDailyLimit) {
				t.Errorf("unexpected claim error: %v", err)
			}
			results <- won
		})
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for won := range results {
		if won {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent winners=%d want=1", winners)
	}
}

func TestRSSDailyBudgetLeaseAndReservationDay(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db, err := pgx.Connect(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	user, team := newAITestTeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/lease-budget", Name: "budget", MaxPostsPerDay: 1})
	if err != nil {
		t.Fatal(err)
	}
	count := func(want int) {
		t.Helper()
		got, err := s.CountRSSFeedPostsToday(ctx, feed.ID)
		if err != nil || got != want {
			t.Fatalf("daily budget=%d want=%d err=%v", got, want, err)
		}
	}
	claim := func(key string) {
		t.Helper()
		if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, key); err != nil || !won {
			t.Fatalf("claim=%v err=%v", won, err)
		}
	}
	claim("first")
	payload, _ := json.Marshal(map[string]any{"params": map[string]any{"rss_automation": map[string]string{"feed_id": feed.ID, "item_key": "first"}}})
	job, err := s.CreateAIJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Origin: domain.AIJobOriginAutomation, Status: domain.AIJobStatusPending, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `update rss_imported_items set created_at = now() - interval '1 hour' where feed_id = $1 and item_key = 'first'`, feed.ID); err != nil {
		t.Fatal(err)
	}
	count(1) // Live pending jobs protect expired leases.
	if won, err := s.MarkAIJobProcessing(ctx, job.ID); err != nil || !won {
		t.Fatal(won, err)
	}
	count(1)
	if won, err := s.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusFailed, nil, "failed"); err != nil || !won {
		t.Fatal(won, err)
	}
	count(0) // Expired no-post failures return capacity.
	claim("first")
	count(1)
	if _, err := db.Exec(ctx, `update rss_imported_items set budget_date = (clock_timestamp() at time zone 'UTC')::date - 1 where feed_id = $1 and item_key = 'first'`, feed.ID); err != nil {
		t.Fatal(err)
	}
	count(0) // Simulate midnight: yesterday's reservation stays on yesterday.
	claim("second")
	_, err = s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "late", Content: "late", Draft: true, RSSFeedID: &feed.ID, RSSItemKey: "first", ScheduledAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	count(1) // Late completion does not consume today's second slot.
	if _, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "second"); err != nil {
		t.Fatal(err)
	}
	count(0)
	// Legacy posts without a marker still consume capacity.
	_, err = s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "legacy", Content: "legacy", Draft: true, RSSFeedID: &feed.ID})
	if err != nil {
		t.Fatal(err)
	}
	count(1)
}

func TestRSSDailyBudgetCancellationPreservesReservation(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/cancel-budget", Name: "budget", MaxPostsPerDay: 1})
	if err != nil {
		t.Fatal(err)
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "active"); err != nil || !won {
		t.Fatal(won, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	for name, operation := range map[string]func() error{
		"count":   func() error { _, err := s.CountRSSFeedPostsToday(cancelled, feed.ID); return err },
		"dedupe":  func() error { _, err := s.RSSItemAlreadyImported(cancelled, feed.ID, "active"); return err },
		"claim":   func() error { _, err := s.ClaimRSSItemAttempt(cancelled, team.ID, feed.ID, "new"); return err },
		"release": func() error { _, err := s.ReleaseRSSItemAttempt(cancelled, team.ID, feed.ID, "active"); return err },
		"record":  func() error { return s.RecordRSSImportedItem(cancelled, feed.ID, "new", "") },
	} {
		t.Run(name, func(t *testing.T) {
			if err := operation(); !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v want cancellation", err)
			}
		})
	}
	if count, err := s.CountRSSFeedPostsToday(ctx, feed.ID); err != nil || count != 1 {
		t.Fatalf("budget after cancellation=%d want=1 err=%v", count, err)
	}
	if imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, "new"); err != nil || imported {
		t.Fatalf("cancelled write left marker=%v err=%v", imported, err)
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "new"); won || !errors.Is(err, domain.ErrRSSDailyLimit) {
		t.Fatalf("cancelled release freed capacity=%v err=%v", won, err)
	}
}
