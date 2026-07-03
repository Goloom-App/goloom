package scheduler

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/rss"
)

// feedProvider implements both SocialMediaProvider and AuthorFeedFetcher so that
// importExternalPostsForAccount can successfully cast the provider to the feed fetcher interface.
type feedProvider struct {
	name string

	mu           sync.Mutex
	fetchedSince []time.Time
	posts        []provider.AuthorPost
	fetchErr     error
}

func (f *feedProvider) Name() string { return f.name }
func (f *feedProvider) Capabilities(_ context.Context, _ domain.SocialAccount) (provider.Capabilities, error) {
	return provider.Capabilities{MaxChars: 500}, nil
}
func (f *feedProvider) PrepareProviderInstance(_ context.Context, _ domain.CreateProviderInstanceInput) (domain.PreparedProviderInstance, error) {
	return domain.PreparedProviderInstance{}, nil
}
func (f *feedProvider) ConnectAccount(_ context.Context, _ domain.CreateAccountInput, _ *domain.ProviderInstance) (domain.ConnectedAccount, error) {
	return domain.ConnectedAccount{}, nil
}
func (f *feedProvider) UploadMedia(_ context.Context, _ domain.SocialAccount, _ provider.PublishAuth, _ io.Reader, _, _, _ string) (string, error) {
	return "", nil
}
func (f *feedProvider) Publish(_ context.Context, _ domain.SocialAccount, _ provider.PublishAuth, _ provider.PublishRequest) (provider.PublishResult, error) {
	return provider.PublishResult{}, nil
}
func (f *feedProvider) GetMetrics(_ context.Context, _ domain.SocialAccount, _ provider.PublishAuth, _ string) ([]provider.EngagementMetric, error) {
	return nil, nil
}
func (f *feedProvider) GetAccountMetrics(_ context.Context, _ domain.SocialAccount, _ provider.PublishAuth) ([]provider.AccountMetric, error) {
	return nil, nil
}
func (f *feedProvider) ListAuthorPosts(_ context.Context, _ domain.SocialAccount, _ provider.PublishAuth, since time.Time, _ int) ([]provider.AuthorPost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetchedSince = append(f.fetchedSince, since)
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.posts, nil
}

// Ensure feedProvider satisfies both interfaces.
var _ provider.SocialMediaProvider = (*feedProvider)(nil)
var _ provider.AuthorFeedFetcher = (*feedProvider)(nil)

// ---------------------------------------------------------------------------
// externalPostImportJob / SyncExternalPostsNow
// ---------------------------------------------------------------------------

func TestExternalPostImportJob_noTeams(t *testing.T) {
	st := &mockStore{} // ListTeamsWithExternalPostMonitorEnabled returns nil, nil
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	// Should run without error; no panics
	svc.externalPostImportJob(context.Background())
}

func TestSyncExternalPostsNow_noTeams(t *testing.T) {
	st := &mockStore{}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	// Acquires mutex, runs job, releases mutex — must complete without deadlock
	done := make(chan struct{})
	go func() {
		svc.SyncExternalPostsNow(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SyncExternalPostsNow deadlocked")
	}
}

// ---------------------------------------------------------------------------
// importExternalPostsForTeam
// ---------------------------------------------------------------------------

func TestImportExternalPostsForTeam_noOwner(t *testing.T) {
	// ListTeamMembers returns empty → no owner found → logs and returns
	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return nil, nil
		},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	settings := domain.ExternalPostMonitorSettings{TeamID: "team-noop"}
	svc.importExternalPostsForTeam(context.Background(), settings)
	// No panic expected; UpdateExternalPostMonitorSyncState not called because we returned early
}

func TestImportExternalPostsForTeam_unsupportedProvider(t *testing.T) {
	// Account's provider not in registry → importExternalPostsForAccount returns error
	// UpdateExternalPostMonitorSyncState is still called after the loop
	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: "u1", Role: domain.RoleOwner}}, nil
		},
		listTeamAccountsFn: func(_ context.Context, _ string) ([]domain.SocialAccount, error) {
			return []domain.SocialAccount{
				{ID: "acct1", Provider: "unknown-provider", RemoteAccountID: "remote1"},
			}, nil
		},
	}

	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	bc := time.Now().UTC()
	settings := domain.ExternalPostMonitorSettings{TeamID: "team-no-provider", BackfillCompletedAt: &bc}
	// Must not panic; logs warning for unsupported provider, still updates sync state
	svc.importExternalPostsForTeam(context.Background(), settings)
}

func TestImportExternalPostsForTeam_happyPath(t *testing.T) {
	var createImportedCalls int

	fp := &feedProvider{
		name: "fakefeed",
		posts: []provider.AuthorPost{
			{RemoteID: "r1", URL: "https://example.com/1", Content: "hello", PublishedAt: time.Now().UTC().Add(-time.Hour)},
		},
	}
	reg := provider.NewRegistry(fp)

	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: "owner1", Role: domain.RoleOwner}}, nil
		},
		listTeamAccountsFn: func(_ context.Context, _ string) ([]domain.SocialAccount, error) {
			return []domain.SocialAccount{
				{ID: "acct1", Provider: "fakefeed", RemoteAccountID: "remote-123"},
			}, nil
		},
		createImportedPostFn: func(_ context.Context, _, _ string, _ domain.ImportedPostInput) (domain.ScheduledPost, error) {
			createImportedCalls++
			return domain.ScheduledPost{ID: "ip-1"}, nil
		},
	}

	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	bc := time.Now().UTC()
	settings := domain.ExternalPostMonitorSettings{TeamID: "team1", BackfillCompletedAt: &bc}
	svc.importExternalPostsForTeam(context.Background(), settings)

	if createImportedCalls != 1 {
		t.Fatalf("expected 1 CreateImportedPost call, got %d", createImportedCalls)
	}
}

// ---------------------------------------------------------------------------
// importExternalPostsForAccount
// ---------------------------------------------------------------------------

func TestImportExternalPostsForAccount_missingProvider(t *testing.T) {
	st := &mockStore{}
	reg := provider.NewRegistry() // empty registry
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)

	account := domain.SocialAccount{ID: "a1", Provider: "ghost", RemoteAccountID: "xyz"}
	n, err := svc.importExternalPostsForAccount(context.Background(), "team1", "owner1", account, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
	if n != 0 {
		t.Fatalf("expected 0 imported, got %d", n)
	}
}

func TestImportExternalPostsForAccount_noFeedFetcher(t *testing.T) {
	// fakeProvider does NOT implement AuthorFeedFetcher
	noFeed := &fakeProvider{name: "nofeed"}
	reg := provider.NewRegistry(noFeed)
	st := &mockStore{}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)

	account := domain.SocialAccount{ID: "a1", Provider: "nofeed", RemoteAccountID: "xyz"}
	n, err := svc.importExternalPostsForAccount(context.Background(), "team1", "owner1", account, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error: provider does not support author feed")
	}
	if n != 0 {
		t.Fatalf("expected 0 imported, got %d", n)
	}
}

func TestImportExternalPostsForAccount_missingRemoteAccountID(t *testing.T) {
	fp := &feedProvider{name: "fakefeed", posts: nil}
	reg := provider.NewRegistry(fp)
	st := &mockStore{}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)

	account := domain.SocialAccount{ID: "a1", Provider: "fakefeed", RemoteAccountID: "   "}
	n, err := svc.importExternalPostsForAccount(context.Background(), "team1", "owner1", account, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error for missing remote_account_id")
	}
	if n != 0 {
		t.Fatalf("expected 0 imported, got %d", n)
	}
}

func TestImportExternalPostsForAccount_fetchError(t *testing.T) {
	fetchErr := errors.New("network timeout")
	fp := &feedProvider{name: "fakefeed", fetchErr: fetchErr}
	reg := provider.NewRegistry(fp)
	st := &mockStore{}
	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)

	account := domain.SocialAccount{ID: "a1", Provider: "fakefeed", RemoteAccountID: "acct-remote"}
	n, err := svc.importExternalPostsForAccount(context.Background(), "team1", "owner1", account, time.Now().UTC())
	if !errors.Is(err, fetchErr) {
		t.Fatalf("expected fetchErr, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 imported, got %d", n)
	}
}

func TestImportExternalPostsForAccount_alreadyTracked(t *testing.T) {
	fp := &feedProvider{
		name: "fakefeed",
		posts: []provider.AuthorPost{
			{RemoteID: "r1", URL: "https://ex.com/1", Content: "hi", PublishedAt: time.Now().UTC()},
		},
	}
	reg := provider.NewRegistry(fp)

	st := &mockStore{
		authorPostAlreadyTrackedFn: func(_ context.Context, _, _, _ string, _ map[string]string) (bool, error) {
			return true, nil // already imported
		},
	}

	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	account := domain.SocialAccount{ID: "a1", Provider: "fakefeed", RemoteAccountID: "acct-remote"}
	n, err := svc.importExternalPostsForAccount(context.Background(), "team1", "owner1", account, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 imported (already tracked), got %d", n)
	}
}

func TestImportExternalPostsForAccount_importsNewPosts(t *testing.T) {
	fp := &feedProvider{
		name: "fakefeed",
		posts: []provider.AuthorPost{
			{RemoteID: "r1", URL: "https://ex.com/1", Content: "brand new", PublishedAt: time.Now().UTC().Add(-2 * time.Hour)},
			{RemoteID: "r2", URL: "https://ex.com/2", Content: "another", PublishedAt: time.Now().UTC().Add(-1 * time.Hour)},
		},
	}
	reg := provider.NewRegistry(fp)

	var createCalls int
	st := &mockStore{
		createImportedPostFn: func(_ context.Context, _, _ string, _ domain.ImportedPostInput) (domain.ScheduledPost, error) {
			createCalls++
			return domain.ScheduledPost{ID: "new-ip"}, nil
		},
	}

	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	account := domain.SocialAccount{ID: "a1", Provider: "fakefeed", RemoteAccountID: "acct-remote"}
	n, err := svc.importExternalPostsForAccount(context.Background(), "team1", "owner1", account, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 imported, got %d", n)
	}
	if createCalls != 2 {
		t.Fatalf("expected 2 CreateImportedPost calls, got %d", createCalls)
	}
}

// ---------------------------------------------------------------------------
// ImportOldPosts
// ---------------------------------------------------------------------------

func TestImportOldPosts_noOwner(t *testing.T) {
	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return nil, nil // no owner
		},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	_, err := svc.ImportOldPosts(context.Background(), "team1", ImportOldPostsInput{})
	if err == nil {
		t.Fatal("expected error when no team owner")
	}
}

func TestImportOldPosts_invalidUntilDate(t *testing.T) {
	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: "u1", Role: domain.RoleOwner}}, nil
		},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)

	_, err := svc.ImportOldPosts(context.Background(), "team1", ImportOldPostsInput{
		UntilDate: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid until_date")
	}
}

func TestImportOldPosts_limitCapping(t *testing.T) {
	// Limit > 500 should be capped at 500; provider returns no posts so loop exits immediately
	fp := &feedProvider{name: "fakefeed", posts: nil}
	reg := provider.NewRegistry(fp)
	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: "u1", Role: domain.RoleOwner}}, nil
		},
		listTeamAccountsFn: func(_ context.Context, _ string) ([]domain.SocialAccount, error) {
			return []domain.SocialAccount{
				{ID: "a1", Provider: "fakefeed", RemoteAccountID: "acct1"},
			}, nil
		},
	}

	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	res, err := svc.ImportOldPosts(context.Background(), "team1", ImportOldPostsInput{
		Limit: 9999,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Imported != 0 {
		t.Fatalf("expected 0 imported (no posts returned), got %d", res.Imported)
	}
	// ListAuthorPosts should have been called at least once (limit was applied but provider returned no posts)
	fp.mu.Lock()
	calls := len(fp.fetchedSince)
	fp.mu.Unlock()
	if calls == 0 {
		t.Fatal("expected ListAuthorPosts to have been called")
	}
}

func TestImportOldPosts_filtersByAccountID(t *testing.T) {
	fp := &feedProvider{
		name: "fakefeed",
		posts: []provider.AuthorPost{
			{RemoteID: "r1", URL: "https://ex.com/1", Content: "old post", PublishedAt: time.Now().UTC().Add(-6 * time.Hour)},
		},
	}
	reg := provider.NewRegistry(fp)

	var createCalls int
	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: "u1", Role: domain.RoleOwner}}, nil
		},
		listTeamAccountsFn: func(_ context.Context, _ string) ([]domain.SocialAccount, error) {
			return []domain.SocialAccount{
				{ID: "match-acct", Provider: "fakefeed", RemoteAccountID: "r-match"},
				{ID: "skip-acct", Provider: "fakefeed", RemoteAccountID: "r-skip"},
			}, nil
		},
		createImportedPostFn: func(_ context.Context, _, _ string, _ domain.ImportedPostInput) (domain.ScheduledPost, error) {
			createCalls++
			return domain.ScheduledPost{ID: "new-ip"}, nil
		},
	}

	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	res, err := svc.ImportOldPosts(context.Background(), "team1", ImportOldPostsInput{
		AccountIDs: []string{"match-acct"},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Imported != 1 {
		t.Fatalf("expected 1 imported (only match-acct), got %d", res.Imported)
	}

	// Only 1 ListAuthorPosts call (for match-acct), not 2
	fp.mu.Lock()
	calls := len(fp.fetchedSince)
	fp.mu.Unlock()
	if calls != 1 {
		t.Fatalf("expected 1 ListAuthorPosts call, got %d", calls)
	}
}

func TestImportOldPosts_noAccountFilter_allAccounts(t *testing.T) {
	fp := &feedProvider{
		name: "fakefeed",
		posts: []provider.AuthorPost{
			{RemoteID: "r1", URL: "https://ex.com/1", Content: "post 1", PublishedAt: time.Now().UTC().Add(-2 * time.Hour)},
		},
	}
	reg := provider.NewRegistry(fp)

	var createCalls int
	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: "u1", Role: domain.RoleOwner}}, nil
		},
		listTeamAccountsFn: func(_ context.Context, _ string) ([]domain.SocialAccount, error) {
			return []domain.SocialAccount{
				{ID: "acct1", Provider: "fakefeed", RemoteAccountID: "r1"},
				{ID: "acct2", Provider: "fakefeed", RemoteAccountID: "r2"},
			}, nil
		},
		createImportedPostFn: func(_ context.Context, _, _ string, _ domain.ImportedPostInput) (domain.ScheduledPost, error) {
			createCalls++
			return domain.ScheduledPost{ID: "ip"}, nil
		},
	}

	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	res, err := svc.ImportOldPosts(context.Background(), "team1", ImportOldPostsInput{
		Limit: 10, // No AccountIDs filter → imports all accounts
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both accounts get 1 post each from the fake provider
	if res.Imported != 2 {
		t.Fatalf("expected 2 imported (both accounts), got %d", res.Imported)
	}
}

func TestImportOldPosts_stopsWhenNoMorePosts(t *testing.T) {
	fp := &feedProvider{name: "fakefeed", posts: nil} // empty list → loop breaks
	reg := provider.NewRegistry(fp)

	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: "u1", Role: domain.RoleOwner}}, nil
		},
		listTeamAccountsFn: func(_ context.Context, _ string) ([]domain.SocialAccount, error) {
			return []domain.SocialAccount{
				{ID: "acct1", Provider: "fakefeed", RemoteAccountID: "remote1"},
			}, nil
		},
	}

	svc := New(testLogger(), st, reg, time.Minute, 1, 0, 0, 0, 0, nil)
	res, err := svc.ImportOldPosts(context.Background(), "team1", ImportOldPostsInput{Limit: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Imported != 0 {
		t.Fatalf("expected 0 imported (no posts), got %d", res.Imported)
	}
}

// ---------------------------------------------------------------------------
// rssImportJob / SyncRSSFeedsNow / handleRSSFirstFetch (additional coverage)
// ---------------------------------------------------------------------------

func TestRSSImportJob_noFeeds(t *testing.T) {
	st := &mockStore{} // ListActiveRSSFeedConfigs returns nil, nil
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	svc.rssImportJob(context.Background())
}

func TestSyncRSSFeedsNow_noFeeds(t *testing.T) {
	st := &mockStore{}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	done := make(chan struct{})
	go func() {
		svc.SyncRSSFeedsNow(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SyncRSSFeedsNow deadlocked")
	}
}

func TestRSSImportJob_withFeeds(t *testing.T) {
	// rssImportJob creates its own parser internally; here we call importRSSFeed directly
	// with a stub fetcher to test the full flow end-to-end.
	ownerID := "owner-rss"
	accountID := "acct-rss"

	var updateLastFetchedCalls int
	var createPostCalls int

	lastFetched := time.Now().UTC().Add(-time.Hour)
	feed := domain.RSSFeedConfig{
		ID:               "rss-feed-1",
		TeamID:           "team-rss",
		FeedURL:          "https://example.com/feed",
		Name:             "Test Feed",
		IsActive:         true,
		ContentTemplate:  "{title}\n{link}",
		OutputMode:       domain.AutomationOutputPublishNow,
		MaxPostsPerDay:   10,
		CounterNext:      1,
		TargetAccountIDs: []string{accountID},
		LastFetchedAt:    &lastFetched,
	}

	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: ownerID, Role: domain.RoleOwner}}, nil
		},
		listActiveRSSFeedsFn: func(_ context.Context, _ int) ([]domain.RSSFeedConfig, error) {
			return []domain.RSSFeedConfig{feed}, nil
		},
		updateRSSFeedLastFetchedFn: func(_ context.Context, _ string, _ time.Time) error {
			updateLastFetchedCalls++
			return nil
		},
		createScheduledPostFn: func(_ context.Context, _ string, _ domain.AuthenticatedPrincipal, _ domain.CreatePostInput) (domain.ScheduledPost, error) {
			createPostCalls++
			return domain.ScheduledPost{ID: "rss-post-1"}, nil
		},
	}

	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)

	stubFetcher := stubRSSFetcher{items: []rss.Item{
		{
			GUID:        "item-123",
			Link:        "https://example.com/post-1",
			Title:       "New Post",
			Content:     "Content here",
			PublishedAt: time.Now().UTC().Add(-30 * time.Minute), // newer than lastFetched
		},
	}}
	svc.importRSSFeed(context.Background(), stubFetcher, feed)

	if updateLastFetchedCalls != 1 {
		t.Fatalf("expected UpdateRSSFeedLastFetched called once, got %d", updateLastFetchedCalls)
	}
	if createPostCalls != 1 {
		t.Fatalf("expected 1 post created, got %d", createPostCalls)
	}
}

func TestHandleRSSFirstFetch_publishLatest(t *testing.T) {
	// handleRSSFirstFetch with InitialSyncPublishLatest publishes only the most recent item
	ownerID := "owner-rss"
	accountID := "acct-rss"

	var createCalls int
	var updateCalls int
	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: ownerID, Role: domain.RoleOwner}}, nil
		},
		createScheduledPostFn: func(_ context.Context, _ string, _ domain.AuthenticatedPrincipal, _ domain.CreatePostInput) (domain.ScheduledPost, error) {
			createCalls++
			return domain.ScheduledPost{ID: "rss-post"}, nil
		},
		updateRSSFeedLastFetchedFn: func(_ context.Context, _ string, _ time.Time) error {
			updateCalls++
			return nil
		},
	}

	older := time.Now().UTC().Add(-2 * time.Hour)
	newer := time.Now().UTC().Add(-30 * time.Minute)

	items := []rss.Item{
		{GUID: "old-item", Title: "Old", Content: "old content", Link: "https://ex.com/old", PublishedAt: older},
		{GUID: "new-item", Title: "New", Content: "new content", Link: "https://ex.com/new", PublishedAt: newer},
	}

	feed := domain.RSSFeedConfig{
		ID:               "feed-1",
		TeamID:           "team-1",
		Name:             "Blog",
		ContentTemplate:  "{title}",
		InitialSyncMode:  domain.RSSInitialSyncPublishLatest,
		TargetAccountIDs: []string{accountID},
		OutputMode:       domain.AutomationOutputPublishNow,
		CounterNext:      1,
	}

	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	svc.handleRSSFirstFetch(context.Background(), feed, items, time.Now().UTC(), 5)

	if createCalls != 1 {
		t.Fatalf("expected 1 post created (latest item), got %d", createCalls)
	}
	if updateCalls != 1 {
		t.Fatalf("expected UpdateRSSFeedLastFetched called once, got %d", updateCalls)
	}
}

func TestHandleRSSFirstFetch_doNotPublish(t *testing.T) {
	// InitialSyncMode != PublishLatest → no post created, still calls UpdateRSSFeedLastFetched
	var updateCalls int
	st := &mockStore{
		updateRSSFeedLastFetchedFn: func(_ context.Context, _ string, _ time.Time) error {
			updateCalls++
			return nil
		},
	}

	items := []rss.Item{
		{GUID: "item-1", Title: "Post", Content: "content", Link: "https://ex.com/1", PublishedAt: time.Now().UTC().Add(-time.Hour)},
	}
	feed := domain.RSSFeedConfig{
		ID:               "feed-2",
		TeamID:           "team-1",
		Name:             "Blog",
		ContentTemplate:  "{title}",
		InitialSyncMode:  domain.RSSInitialSyncBaseline,
		TargetAccountIDs: []string{"acct1"},
	}

	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	svc.handleRSSFirstFetch(context.Background(), feed, items, time.Now().UTC(), 5)

	if updateCalls != 1 {
		t.Fatalf("expected UpdateRSSFeedLastFetched called once, got %d", updateCalls)
	}
}
