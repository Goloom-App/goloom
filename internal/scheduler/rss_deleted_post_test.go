package scheduler

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/rss"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
)

func TestImportRSSFeedDoesNotRecreateDeletedPostAfterLease(t *testing.T) {
	for _, aiEnabled := range []bool{true, false} {
		name := "direct"
		if aiEnabled {
			name = "AI"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			enc, err := security.NewEncrypter("scheduler-test-secret-32-bytes!!!")
			if err != nil {
				t.Fatal(err)
			}
			dsn := filepath.Join(t.TempDir(), "rss.sqlite") + "?_pragma=foreign_keys(1)"
			st, err := sqlite.New(ctx, dsn, enc)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { st.Close() })
			db, err := sql.Open("sqlite", dsn)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			user, err := st.UpsertOIDCUser(ctx, "rss-owner", "rss@test", "RSS")
			if err != nil {
				t.Fatal(err)
			}
			team, err := st.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "RSS"})
			if err != nil {
				t.Fatal(err)
			}
			if aiEnabled {
				if _, err := st.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &aiEnabled}); err != nil {
					t.Fatal(err)
				}
				if _, err := st.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{Provider: "openai", Model: "gpt-test", APIKey: "sk-test"}); err != nil {
					t.Fatal(err)
				}
			}
			account, err := st.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, InstanceURL: "https://example.test", Username: "rss", AccessToken: "token"})
			if err != nil {
				t.Fatal(err)
			}
			feed, err := st.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "RSS", IsActive: true, AiEnhanceEnabled: aiEnabled, ContentTemplate: "{title}", OutputMode: domain.AutomationOutputPublishNow, TargetAccountIDs: []string{account.ID}, MaxPostsPerDay: 10})
			if err != nil {
				t.Fatal(err)
			}
			feed.LastFetchedAt = ptrTime(time.Now().UTC().Add(-time.Hour))
			manager := aijobs.NewManager(st, noopRunner{})
			svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
			item := rss.Item{GUID: "deleted-item", Title: "Deliberately removed", Link: "https://example.test/article", Content: "Article", PublishedAt: time.Now().UTC().Add(-time.Minute)}
			fetcher := stubRSSFetcher{items: []rss.Item{item}}
			svc.importRSSFeed(ctx, fetcher, feed)
			manager.Wait()
			if aiEnabled {
				jobs, err := st.ListAIJobs(ctx, team.ID, 10)
				if err != nil || len(jobs) != 1 || jobs[0].Status != domain.AIJobStatusCompleted {
					t.Fatalf("initial AI jobs=%+v err=%v", jobs, err)
				}
				// Materialize successful AI output through the same atomic post/marker
				// transaction used by the completion callback.
				if _, err := st.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: item.Title, Content: "AI article", ScheduledAt: time.Now().UTC(), TargetAccounts: []string{account.ID}, Source: domain.PostSourceAutomation, RSSFeedID: &feed.ID, RSSItemKey: rss.ItemKey(item)}); err != nil {
					t.Fatal(err)
				}
			}
			posts, err := st.ListTeamPosts(ctx, team.ID)
			if err != nil || len(posts) != 1 {
				t.Fatalf("initial posts=%d err=%v", len(posts), err)
			}
			if posts[0].Status != domain.PostStatusPending {
				t.Fatalf("publish_now status=%q", posts[0].Status)
			}
			if err := st.DeleteScheduledPost(ctx, team.ID, posts[0].ID); err != nil {
				t.Fatal(err)
			}
			var linkedPost sql.NullString
			if err := db.QueryRowContext(ctx, "select post_id from rss_imported_items where feed_id = ? and item_key = ?", feed.ID, rss.ItemKey(item)).Scan(&linkedPost); err != nil || linkedPost.Valid {
				t.Fatalf("deleted post link=%v err=%v", linkedPost, err)
			}
			if _, err := db.ExecContext(ctx, "update rss_imported_items set created_at = ? where feed_id = ? and item_key = ?", time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano), feed.ID, rss.ItemKey(item)); err != nil {
				t.Fatal(err)
			}
			svc.importRSSFeed(ctx, fetcher, feed)
			manager.Wait()
			posts, err = st.ListTeamPosts(ctx, team.ID)
			if err != nil || len(posts) != 0 {
				t.Errorf("deleted RSS item recreated posts=%d err=%v", len(posts), err)
			}
			jobs, err := st.ListAIJobs(ctx, team.ID, 10)
			wantJobs := 0
			if aiEnabled {
				wantJobs = 1
			}
			if err != nil || len(jobs) != wantJobs {
				t.Errorf("deleted RSS item queued AI jobs=%d want=%d err=%v", len(jobs), wantJobs, err)
			}
		})
	}
}
