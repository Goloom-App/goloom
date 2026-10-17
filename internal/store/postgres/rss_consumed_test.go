package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestRSSConsumedItemSurvivesPostDeletion(t *testing.T) {
	for _, attempts := range []int{0, 1, domain.MaxRSSItemAttempts} {
		for _, link := range []string{"transaction", "record", "update"} {
			t.Run(fmt.Sprintf("attempts=%d/link=%s", attempts, link), func(t *testing.T) {
				ctx := context.Background()
				s := newTestStore(t)
				db, err := pgx.Connect(ctx, postgresDSN(t))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close(ctx)
				user, team := newAITestTeam(t, s)
				feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/consumed", Name: "consumed", IsActive: true})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := s.DeleteRSSFeedConfig(ctx, team.ID, feed.ID); err != nil {
						t.Error(err)
					}
				})
				for attempt := 0; attempt < attempts; attempt++ {
					if attempt > 0 {
						if _, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil {
							t.Fatal(err)
						}
					}
					if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil || !won {
						t.Fatalf("claim=%v err=%v", won, err)
					}
				}
				input := domain.CreatePostInput{Title: "consumed", Content: "consumed", Draft: true, RSSFeedID: &feed.ID}
				if link == "transaction" {
					input.RSSItemKey = "item"
				}
				post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, input)
				if err != nil {
					t.Fatal(err)
				}
				switch link {
				case "record":
					err = s.RecordRSSImportedItem(ctx, feed.ID, "item", post.ID)
				case "update":
					if attempts == 0 {
						if err := s.RecordRSSImportedItem(ctx, feed.ID, "item", ""); err != nil {
							t.Fatal(err)
						}
					}
					err = s.UpdateRSSImportedItemPostID(ctx, team.ID, feed.ID, "item", post.ID)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := s.DeleteScheduledPost(ctx, team.ID, post.ID); err != nil {
					t.Fatal(err)
				}
				var linkedPost *string
				if err := db.QueryRow(ctx, "select post_id from rss_imported_items where feed_id = $1 and item_key = 'item'", feed.ID).Scan(&linkedPost); err != nil || linkedPost != nil {
					t.Fatalf("deleted post link=%v err=%v", linkedPost, err)
				}
				if _, err := db.Exec(ctx, "update rss_imported_items set created_at = $1 where feed_id = $2 and item_key = 'item'", time.Now().UTC().Add(-time.Hour), feed.ID); err != nil {
					t.Fatal(err)
				}
				if imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, "item"); err != nil || !imported {
					t.Errorf("deleted consumed item eligible for import: imported=%v err=%v", imported, err)
				}
				current, err := s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
				if err != nil || current.ExhaustedItemsCount != 0 {
					t.Errorf("consumed item counted as exhausted: count=%d err=%v", current.ExhaustedItemsCount, err)
				}
				// Both API and scheduler feed lists must omit consumed items from exhaustion.
				teamFeeds, err := s.ListRSSFeedConfigs(ctx, team.ID)
				if err != nil || len(teamFeeds) != 1 || teamFeeds[0].ExhaustedItemsCount != 0 {
					t.Errorf("team feed exhaustion=%+v err=%v", teamFeeds, err)
				}
				activeFeeds, err := s.ListActiveRSSFeedConfigs(ctx, 0)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, activeFeed := range activeFeeds {
					if activeFeed.ID == feed.ID {
						found = true
						if activeFeed.ExhaustedItemsCount != 0 {
							t.Errorf("active feed exhaustion=%d", activeFeed.ExhaustedItemsCount)
						}
					}
				}
				if !found {
					t.Error("consumed feed missing from active feeds")
				}

				if _, err := s.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, "item"); !errors.Is(err, domain.ErrNotFound) {
					t.Errorf("consumed item released: %v", err)
				}
				if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "item"); err != nil || won {
					t.Errorf("consumed item reclaimed=%v err=%v", won, err)
				}
				input.RSSItemKey = "item"
				if _, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, input); err == nil {
					t.Error("deleted consumed post recreated")
				}
				posts, err := s.ListTeamPosts(ctx, team.ID)
				if err != nil || len(posts) != 0 {
					t.Errorf("posts after deletion=%d err=%v", len(posts), err)
				}
			})
		}
	}
}
