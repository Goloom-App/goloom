package sqlite_test

import (
	"context"
	"encoding/json"
	"git.f4mily.net/goloom/internal/domain"
	"testing"
	"time"
)

func TestRSSReservationLease(t *testing.T) {
	ctx := context.Background()
	s, db := newAIStoreWithRawDB(t)
	user, team := makeAITeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/lease", Name: "lease"})
	if err != nil {
		t.Fatal(err)
	}
	age := func(key string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, "update rss_imported_items set created_at = ? where feed_id = ? and item_key = ?", time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano), feed.ID, key); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"orphan", "active", "baseline", "linked"} {
		if key == "baseline" {
			err = s.RecordRSSImportedItem(ctx, feed.ID, key, "")
		} else {
			_, err = s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, key)
		}
		if err != nil {
			t.Fatal(err)
		}
		age(key)
	}
	payload, _ := json.Marshal(map[string]any{"params": map[string]any{"rss_automation": map[string]string{"feed_id": feed.ID, "item_key": "active"}}})
	job, err := s.CreateAIJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Origin: domain.AIJobOriginAutomation, Status: domain.AIJobStatusProcessing, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "linked", Content: "linked", Draft: true, RSSFeedID: &feed.ID, RSSItemKey: "linked"})
	if err != nil {
		t.Fatal(err)
	}
	_ = post
	for _, key := range []string{"baseline", "linked", "active"} {
		if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, key); err != nil || won {
			t.Fatalf("protected %s claimed=%v err=%v", key, won, err)
		}
	}
	for attempt := 2; attempt <= 3; attempt++ {
		if imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, "orphan"); err != nil || imported {
			t.Fatalf("stale orphan skipped=%v err=%v", imported, err)
		}
		if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "orphan"); err != nil || !won {
			t.Fatalf("orphan attempt %d claimed=%v err=%v", attempt, won, err)
		}
		if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "orphan"); err != nil || won {
			t.Fatalf("fresh orphan claimed=%v err=%v", won, err)
		}
		age("orphan")
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "orphan"); err != nil || won {
		t.Fatalf("exhausted orphan claimed=%v err=%v", won, err)
	}
	// Simulates crash after cancel/completion claims terminal state, before marker release.
	if _, err := s.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusFailed, nil, "cancelled"); err != nil {
		t.Fatal(err)
	}
	if imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, "active"); err != nil || imported {
		t.Fatalf("terminal job stale marker skipped=%v err=%v", imported, err)
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "active"); err != nil || !won {
		t.Fatalf("terminal job marker claimed=%v err=%v", won, err)
	}
}
