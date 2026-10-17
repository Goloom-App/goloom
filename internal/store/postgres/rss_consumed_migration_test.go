package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
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

func TestRSSConsumedMigrationPreservesInFlightJobs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   domain.AIJobStatus
		origin   string
		mismatch string
		release  bool
	}{
		{name: "pending completion", status: domain.AIJobStatusPending, origin: domain.AIJobOriginAutomation},
		{name: "processing completion", status: domain.AIJobStatusProcessing, origin: domain.AIJobOriginAutomation},
		{name: "processing release", status: domain.AIJobStatusProcessing, origin: domain.AIJobOriginAutomation, release: true},
		{name: "no job", mismatch: "no job"},
		{name: "user job", status: domain.AIJobStatusPending, origin: domain.AIJobOriginUser},
		{name: "completed job", status: domain.AIJobStatusCompleted, origin: domain.AIJobOriginAutomation},
		{name: "failed job", status: domain.AIJobStatusFailed, origin: domain.AIJobOriginAutomation},
		{name: "foreign team", status: domain.AIJobStatusPending, origin: domain.AIJobOriginAutomation, mismatch: "team"},
		{name: "different feed", status: domain.AIJobStatusPending, origin: domain.AIJobOriginAutomation, mismatch: "feed"},
		{name: "different item", status: domain.AIJobStatusPending, origin: domain.AIJobOriginAutomation, mismatch: "item"},
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
			account, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
				Provider: "mastodon", Username: "rss", AuthType: domain.AccountAuthTypeOAuthToken,
				InstanceURL: "https://example.test", AccessToken: "test",
			})
			if err != nil {
				t.Fatal(err)
			}
			feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/inflight", Name: "inflight", TargetAccountIDs: []string{account.ID}})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.RecordRSSImportedItem(ctx, feed.ID, "item", ""); err != nil {
				t.Fatal(err)
			}
			jobTeam, jobFeed, jobItem := team.ID, feed.ID, "item"
			switch tc.mismatch {
			case "team":
				_, other := newAITestTeam(t, s)
				jobTeam = other.ID
			case "feed":
				other, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/other", Name: "other"})
				if err != nil {
					t.Fatal(err)
				}
				jobFeed = other.ID
			case "item":
				jobItem = "other"
			}
			var job domain.AIJob
			if tc.mismatch != "no job" {
				payload, err := json.Marshal(map[string]any{"params": map[string]any{
					"target_account_ids": []string{account.ID},
					"rss_automation":     map[string]any{"feed_id": jobFeed, "item_key": jobItem, "output_mode": "draft", "fallback_content": "fallback"},
				}})
				if err != nil {
					t.Fatal(err)
				}
				job, err = s.CreateAIJob(ctx, domain.AIJob{TeamID: jobTeam, AuthorUserID: user.ID, Origin: tc.origin, Status: tc.status, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
				if err != nil {
					t.Fatal(err)
				}
			}
			// Recreate the pre-retry schema: all existing markers acquire attempts=0.
			for _, column := range []string{"attempts", "retry_available", "consumed_at"} {
				if _, err := db.Exec(ctx, "alter table rss_imported_items drop column "+column); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(ctx, "update rss_imported_items set created_at = $1 where feed_id = $2", time.Now().UTC().Add(-time.Hour), feed.ID); err != nil {
				t.Fatal(err)
			}
			active := tc.origin == domain.AIJobOriginAutomation && (tc.status == domain.AIJobStatusPending || tc.status == domain.AIJobStatusProcessing) && tc.mismatch == ""
			var lease string
			for run := 0; run < 2; run++ {
				if active && run == 1 {
					// Move the lease within its valid window so refresh detection
					// does not depend on clock resolution or a slow test.
					if _, err := db.Exec(ctx, "update rss_imported_items set created_at = created_at - interval '1 minute' where feed_id = $1", feed.ID); err != nil {
						t.Fatal(err)
					}
					if err := db.QueryRow(ctx, "select created_at::text from rss_imported_items where feed_id = $1", feed.ID).Scan(&lease); err != nil {
						t.Fatal(err)
					}
				}
				s = newTestStore(t)
				var attempts int
				var consumed, retry, fresh bool
				var created string
				if err := db.QueryRow(ctx, "select attempts, consumed_at is not null, retry_available, created_at::text, created_at > now() - interval '5 minutes' from rss_imported_items where feed_id = $1", feed.ID).Scan(&attempts, &consumed, &retry, &created, &fresh); err != nil {
					t.Fatal(err)
				}
				if active {
					if attempts != 1 || consumed || retry || !fresh {
						t.Fatalf("migration run %d active reservation: attempts=%d consumed=%v retry=%v fresh=%v", run, attempts, consumed, retry, fresh)
					}
					if run == 1 && created != lease {
						t.Fatal("second migration refreshed active reservation lease")
					}
					lease = created
				} else if attempts != 0 || !consumed {
					t.Fatalf("migration run %d tombstone: attempts=%d consumed=%v", run, attempts, consumed)
				}
				if imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, "item"); err != nil || !imported {
					t.Fatalf("imported=%v err=%v", imported, err)
				}
				if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil || won {
					t.Fatalf("reserved/consumed item claimed=%v err=%v", won, err)
				}
			}
			if !active {
				if _, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "item"); !errors.Is(err, domain.ErrNotFound) {
					t.Fatalf("tombstone released: %v", err)
				}
				return
			}
			if tc.release {
				if won, err := s.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusFailed, nil, "cancelled"); err != nil || !won {
					t.Fatalf("claim completion=%v err=%v", won, err)
				}
				if attempts, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil || attempts != 1 {
					t.Fatalf("release attempts=%d err=%v", attempts, err)
				}
				if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil || !won {
					t.Fatalf("retry claimed=%v err=%v", won, err)
				}
				return
			}
			instance := api.New(nil, s, nil, provider.NewRegistry(provider.NewMastodonProvider(provider.MastodonRegistrationConfig{})), config.Config{}, nil, nil, nil, nil)
			instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"generated"}`), "")
			// A duplicate callback must not create a second post.
			instance.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"generated"}`), "")
			after, err := s.GetAIJobByIDGlobal(ctx, job.ID)
			if err != nil || after.Status != domain.AIJobStatusCompleted {
				t.Fatalf("completion status=%s error=%s err=%v", after.Status, after.ErrorMessage, err)
			}
			var count int
			if err := db.QueryRow(ctx, "select count(*) from scheduled_posts where rss_feed_id = $1", feed.ID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("posts=%d err=%v", count, err)
			}
			var consumed bool
			if err := db.QueryRow(ctx, "select consumed_at is not null and post_id is not null from rss_imported_items where feed_id = $1", feed.ID).Scan(&consumed); err != nil || !consumed {
				t.Fatalf("completion consumed=%v err=%v", consumed, err)
			}
			if _, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "item"); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("consumed marker released: %v", err)
			}
		})
	}
}
