package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/rss"
)

type pendingRSSRunner struct{ release chan struct{} }

func (r pendingRSSRunner) RunJob(ctx context.Context, _ domain.AIJob, _ domain.AIServiceConfig, _ domain.AIContext) (json.RawMessage, error) {
	select {
	case <-r.release:
		return json.RawMessage(`{"content":"AI article"}`), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestRSSDailyLimitIncludesPendingAIAndCancellationReleasesCapacity(t *testing.T) {
	ctx := context.Background()
	st, _, team, _ := newRecurringAIFixture(t, true)
	account, err := st.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, InstanceURL: "https://example.test", Username: "rss", AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := st.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "RSS", IsActive: true, AiEnhanceEnabled: true, ContentTemplate: "{title}", OutputMode: domain.AutomationOutputPublishNow, TargetAccountIDs: []string{account.ID}, MaxPostsPerDay: 1})
	if err != nil {
		t.Fatal(err)
	}
	feed.LastFetchedAt = ptrTime(time.Now().UTC().Add(-time.Hour))
	release := make(chan struct{})
	manager := aijobs.NewManager(st, pendingRSSRunner{release: release})
	defer func() { close(release); manager.Wait() }()
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
	first := rss.Item{GUID: "first", Title: "First", PublishedAt: time.Now().UTC().Add(-time.Minute)}
	second := rss.Item{GUID: "second", Title: "Second", PublishedAt: time.Now().UTC()}
	svc.importRSSFeed(ctx, stubRSSFetcher{items: []rss.Item{first}}, feed)
	svc.importRSSFeed(ctx, stubRSSFetcher{items: []rss.Item{second}}, feed)
	jobs, err := st.ListAIJobs(ctx, team.ID, 10)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("pending AI jobs=%d, want 1; err=%v", len(jobs), err)
	}
	if won, err := st.ClaimAIJobCompletion(ctx, jobs[0].ID, domain.AIJobStatusFailed, nil, "cancelled"); err != nil || !won {
		t.Fatalf("cancel won=%v err=%v", won, err)
	}
	manager.Cancel(jobs[0].ID)
	manager.Wait()
	if _, err := st.ReleaseRSSItemAttempt(ctx, team.ID, feed.ID, rss.ItemKey(first)); err != nil {
		t.Fatal(err)
	}
	svc.importRSSFeed(ctx, stubRSSFetcher{items: []rss.Item{second}}, feed)
	jobs, err = st.ListAIJobs(ctx, team.ID, 10)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("jobs after cancellation=%d, want 2; err=%v", len(jobs), err)
	}
	posts, err := st.ListTeamPosts(ctx, team.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("pending jobs created posts=%d err=%v", len(posts), err)
	}
}

func TestRSSDailyLimitDirectImportsRemainCapped(t *testing.T) {
	ctx := context.Background()
	st, _, team, _ := newRecurringAIFixture(t, false)
	account, err := st.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, InstanceURL: "https://example.test", Username: "rss", AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := st.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "RSS", IsActive: true, ContentTemplate: "{title}", OutputMode: domain.AutomationOutputPublishNow, TargetAccountIDs: []string{account.ID}, MaxPostsPerDay: 1})
	if err != nil {
		t.Fatal(err)
	}
	feed.LastFetchedAt = ptrTime(time.Now().UTC().Add(-time.Hour))
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	svc.importRSSFeed(ctx, stubRSSFetcher{items: []rss.Item{{GUID: "first", Title: "First", PublishedAt: time.Now().UTC().Add(-time.Minute)}, {GUID: "second", Title: "Second", PublishedAt: time.Now().UTC()}}}, feed)
	svc.importRSSFeed(ctx, stubRSSFetcher{items: []rss.Item{{GUID: "third", Title: "Third", PublishedAt: time.Now().UTC()}}}, feed)
	posts, err := st.ListTeamPosts(ctx, team.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("direct posts=%d, want 1; err=%v", len(posts), err)
	}
	if posts[0].Content != "First" {
		t.Fatalf("direct content=%q", posts[0].Content)
	}
}
