package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/jackc/pgx/v5"
)

// Only an unfinished automation job belonging to the feed's team may hold its
// lease. User-controlled metadata and unrelated jobs must not pin retries.
func TestRSSLeaseRequiresMatchingAutomationJob(t *testing.T) {
	for _, tc := range []struct {
		name     string
		origin   string
		status   domain.AIJobStatus
		foreign  bool
		metadata string
		held     bool
	}{
		{"pending automation", domain.AIJobOriginAutomation, domain.AIJobStatusPending, false, "matching", true},
		{"processing automation", domain.AIJobOriginAutomation, domain.AIJobStatusProcessing, false, "matching", true},
		{"user metadata", domain.AIJobOriginUser, domain.AIJobStatusPending, false, "matching", false},
		{"foreign team", domain.AIJobOriginAutomation, domain.AIJobStatusProcessing, true, "matching", false},
		{"completed job", domain.AIJobOriginAutomation, domain.AIJobStatusCompleted, false, "matching", false},
		{"failed job", domain.AIJobOriginAutomation, domain.AIJobStatusFailed, false, "matching", false},
		{"different feed", domain.AIJobOriginAutomation, domain.AIJobStatusPending, false, "feed", false},
		{"different item", domain.AIJobOriginAutomation, domain.AIJobStatusPending, false, "item", false},
		{"missing metadata", domain.AIJobOriginAutomation, domain.AIJobStatusPending, false, "missing", false},
		{"wrong metadata shape", domain.AIJobOriginAutomation, domain.AIJobStatusPending, false, "scalar", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := newTestStore(t)
			db, err := pgx.Connect(ctx, postgresDSN(t))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close(ctx)
			user, team := newAITestTeam(t, s)
			otherUser, otherTeam := newAITestTeam(t, s)
			feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/lease-edges", Name: "lease edges"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := s.DeleteRSSFeedConfig(ctx, team.ID, feed.ID); err != nil {
					t.Error(err)
				}
			})
			won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item")
			if err != nil || !won {
				t.Fatalf("initial claim=%v err=%v", won, err)
			}
			if _, err := db.Exec(ctx, "update rss_imported_items set created_at = $1 where feed_id = $2 and item_key = $3", time.Now().UTC().Add(-time.Hour), feed.ID, "item"); err != nil {
				t.Fatal(err)
			}
			metadata := any(map[string]string{"feed_id": feed.ID, "item_key": "item"})
			switch tc.metadata {
			case "feed":
				metadata = map[string]string{"feed_id": otherTeam.ID, "item_key": "item"}
			case "item":
				metadata = map[string]string{"feed_id": feed.ID, "item_key": "another"}
			case "missing":
				metadata = nil
			case "scalar":
				metadata = "untrusted metadata"
			}
			payload, err := json.Marshal(map[string]any{"params": map[string]any{"rss_automation": metadata}})
			if err != nil {
				t.Fatal(err)
			}
			job := domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Origin: tc.origin, Status: tc.status, Type: domain.AIJobTypeVoiceEngine, Payload: payload}
			if tc.foreign {
				job.TeamID = otherTeam.ID
				job.AuthorUserID = otherUser.ID
			}
			if _, err := s.CreateAIJob(ctx, job); err != nil {
				t.Fatal(err)
			}
			imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, "item")
			if err != nil || imported != tc.held {
				t.Fatalf("lease held=%v want=%v err=%v", imported, tc.held, err)
			}
			won, err = s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item")
			if err != nil || won == tc.held {
				t.Fatalf("reclaim=%v held=%v err=%v", won, tc.held, err)
			}
			// A reclaim starts a fresh lease; blocked claims never burn an attempt.
			attempts, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "item")
			want := 2
			if tc.held {
				want = 1
			}
			if err != nil || attempts != want {
				t.Fatalf("attempts=%d want=%d err=%v", attempts, want, err)
			}
		})
	}
}

func TestRSSRetryRollbackAndCancelledWritesPreserveBudget(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, team := newAITestTeam(t, s)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/rollback", Name: "rollback"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.DeleteRSSFeedConfig(ctx, team.ID, feed.ID); err != nil {
			t.Error(err)
		}
	})
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil || !won {
		t.Fatal(won, err)
	}
	abort := errors.New("audit write failed")
	err = s.WithAuditTransaction(ctx, func(txctx context.Context) error {
		if n, err := s.ReleaseRSSItemAttempt(txctx, team.ID, feed.ID, "item"); err != nil || n != 1 {
			t.Fatalf("release=%d err=%v", n, err)
		}
		if won, err := s.ClaimRSSItemAttempt(txctx, team.ID, feed.ID, "item"); err != nil || !won {
			t.Fatalf("retry=%v err=%v", won, err)
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("rollback error=%v", err)
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil || won {
		t.Fatalf("rolled-back release opened lease: claim=%v err=%v", won, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if won, err := s.ClaimRSSItemAttempt(cancelled, team.ID, feed.ID, "new"); err == nil || won {
		t.Fatalf("cancelled claim=%v err=%v", won, err)
	}
	if _, err := s.ReleaseRSSItemAttempt(cancelled, team.ID, feed.ID, "item"); err == nil {
		t.Fatal("cancelled release succeeded")
	}
	if _, err := s.RSSItemAlreadyImported(cancelled, feed.ID, "item"); err == nil {
		t.Fatal("cancelled lookup succeeded")
	}
	if err := s.RecordRSSImportedItem(cancelled, feed.ID, "new", ""); err == nil {
		t.Fatal("cancelled marker write succeeded")
	}
	if n, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil || n != 1 {
		t.Fatalf("rollback/cancellation spent retry budget: attempts=%d err=%v", n, err)
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "new"); err != nil || !won {
		t.Fatalf("cancelled write left marker: claim=%v err=%v", won, err)
	}
}
