package scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/rss"
)

type stubRSSFetcher struct {
	items []rss.Item
	err   error
}

func (s stubRSSFetcher) Fetch(_ context.Context, _ string) ([]rss.Item, error) {
	return s.items, s.err
}

func TestRSSOutputSchedule(t *testing.T) {
	past := time.Now().UTC().Add(-2 * time.Hour)
	future := time.Now().UTC().Add(2 * time.Hour)

	at, draft := rssOutputSchedule(domain.AutomationOutputDraft, past)
	if !draft || at.Before(time.Now().UTC().Add(-time.Minute)) {
		t.Fatalf("draft mode past: at=%v draft=%v", at, draft)
	}

	at, draft = rssOutputSchedule(domain.AutomationOutputScheduled, future)
	if draft || !at.Equal(future) {
		t.Fatalf("scheduled future: at=%v draft=%v", at, draft)
	}

	at, draft = rssOutputSchedule(domain.AutomationOutputPublishNow, past)
	if draft {
		t.Fatalf("publish now should not be draft")
	}
}

func TestFilterRSSItemsSince(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []rss.Item{
		{GUID: "old", PublishedAt: since.Add(-time.Hour)},
		{GUID: "new", PublishedAt: since.Add(time.Hour)},
	}
	got := filterRSSItemsSince(items, since)
	if len(got) != 1 || got[0].GUID != "new" {
		t.Fatalf("filterRSSItemsSince = %+v", got)
	}
}

func TestImportRSSFeed_createsPublishNowPost(t *testing.T) {
	teamID := "team-1"
	feedID := "feed-1"
	ownerID := "owner-1"
	accountID := "acct-1"
	published := time.Now().UTC().Add(-time.Minute)

	feed := domain.RSSFeedConfig{
		ID:               feedID,
		TeamID:           teamID,
		FeedURL:          "https://example.com/feed.xml",
		Name:             "Blog",
		IsActive:         true,
		ContentTemplate:  "{title}\n{link}",
		OutputMode:       domain.AutomationOutputPublishNow,
		MaxPostsPerDay:   10,
		CounterNext:      1,
		TargetAccountIDs: []string{accountID},
		LastFetchedAt:    ptrTime(time.Now().UTC().Add(-time.Hour)),
	}

	st := &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: ownerID, Role: domain.RoleOwner}}, nil
		},
		createScheduledPostFn: func(_ context.Context, gotTeamID string, _ domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
			if gotTeamID != teamID {
				t.Fatalf("teamID = %q", gotTeamID)
			}
			if input.Draft {
				t.Fatalf("expected publish_now, got draft")
			}
			if input.Source != domain.PostSourceAutomation {
				t.Fatalf("source = %q", input.Source)
			}
			if input.RSSFeedID == nil || *input.RSSFeedID != feedID {
				t.Fatalf("rss feed id = %v", input.RSSFeedID)
			}
			if input.Content == "" {
				t.Fatal("expected rendered content")
			}
			return domain.ScheduledPost{ID: "post-1", Status: domain.PostStatusPending}, nil
		},
	}

	svc := New(testLogger(), st, nil, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.importRSSFeed(context.Background(), stubRSSFetcher{items: []rss.Item{{
		GUID:        "item-1",
		Link:        "https://example.com/post",
		Title:       "Hello",
		Content:     "Summary",
		PublishedAt: published,
	}}}, feed)
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func testRSSFeed(lastFetchedAt *time.Time) domain.RSSFeedConfig {
	return domain.RSSFeedConfig{
		ID:               "feed-1",
		TeamID:           "team-1",
		FeedURL:          "https://example.com/feed.xml",
		Name:             "Blog",
		IsActive:         true,
		ContentTemplate:  "{title}\n{link}",
		OutputMode:       domain.AutomationOutputPublishNow,
		MaxPostsPerDay:   10,
		CounterNext:      1,
		TargetAccountIDs: []string{"acct-1"},
		LastFetchedAt:    lastFetchedAt,
	}
}

func rssOwnerStore() *mockStore {
	return &mockStore{
		listTeamMembersFn: func(_ context.Context, _ string) ([]domain.TeamMembership, error) {
			return []domain.TeamMembership{{UserID: "owner-1", Role: domain.RoleOwner}}, nil
		},
	}
}

// A static-site feed is often deployed hours after the declared pubDate of its
// newest item; by then last_fetched_at has already moved past that pubDate.
// Novelty must come from the dedupe table, not from the timestamp comparison.
func TestImportRSSFeed_importsItemAppearingAfterItsPubDate(t *testing.T) {
	published := time.Now().UTC().Add(-2 * time.Hour)
	feed := testRSSFeed(ptrTime(time.Now().UTC().Add(-5 * time.Minute)))

	st := rssOwnerStore()
	createCalls := 0
	st.createScheduledPostFn = func(_ context.Context, _ string, _ domain.AuthenticatedPrincipal, _ domain.CreatePostInput) (domain.ScheduledPost, error) {
		createCalls++
		return domain.ScheduledPost{ID: "post-1", Status: domain.PostStatusPending}, nil
	}

	svc := New(testLogger(), st, nil, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.importRSSFeed(context.Background(), stubRSSFetcher{items: []rss.Item{{
		GUID:        "late-item",
		Link:        "https://example.com/late",
		Title:       "Late",
		Content:     "Deployed after its pubDate",
		PublishedAt: published,
	}}}, feed)

	if createCalls != 1 {
		t.Fatalf("expected the late-deployed item to be imported, got %d posts", createCalls)
	}
}

func TestImportRSSFeed_skipsItemsBeyondLookback(t *testing.T) {
	// Items whose pubDate lies further back than the lookback window (7 days)
	// are backlog, not news — they must not be imported.
	published := time.Now().UTC().Add(-10 * 24 * time.Hour)
	feed := testRSSFeed(ptrTime(time.Now().UTC().Add(-5 * time.Minute)))

	st := rssOwnerStore()
	createCalls := 0
	st.createScheduledPostFn = func(_ context.Context, _ string, _ domain.AuthenticatedPrincipal, _ domain.CreatePostInput) (domain.ScheduledPost, error) {
		createCalls++
		return domain.ScheduledPost{ID: "post-1", Status: domain.PostStatusPending}, nil
	}

	svc := New(testLogger(), st, nil, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.importRSSFeed(context.Background(), stubRSSFetcher{items: []rss.Item{{
		GUID:        "backlog-item",
		Link:        "https://example.com/backlog",
		Title:       "Backlog",
		Content:     "Too old",
		PublishedAt: published,
	}}}, feed)

	if createCalls != 0 {
		t.Fatalf("expected backlog item to be skipped, got %d posts", createCalls)
	}
}

func TestImportRSSFeed_alreadyImportedDoesNotConsumeDailyBudget(t *testing.T) {
	feed := testRSSFeed(ptrTime(time.Now().UTC().Add(-5 * time.Minute)))
	feed.MaxPostsPerDay = 1

	st := rssOwnerStore()
	st.rssItemAlreadyImportedFn = func(_ context.Context, _ string, itemKey string) (bool, error) {
		return itemKey == "seen-item", nil
	}
	var createdContents []string
	st.createScheduledPostFn = func(_ context.Context, _ string, _ domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
		createdContents = append(createdContents, input.Content)
		return domain.ScheduledPost{ID: "post-1", Status: domain.PostStatusPending}, nil
	}

	svc := New(testLogger(), st, nil, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.importRSSFeed(context.Background(), stubRSSFetcher{items: []rss.Item{
		{
			GUID:        "seen-item",
			Link:        "https://example.com/seen",
			Title:       "Seen",
			Content:     "Already imported",
			PublishedAt: time.Now().UTC().Add(-3 * time.Hour),
		},
		{
			GUID:        "fresh-item",
			Link:        "https://example.com/fresh",
			Title:       "Fresh",
			Content:     "New episode",
			PublishedAt: time.Now().UTC().Add(-2 * time.Hour),
		},
	}}, feed)

	if len(createdContents) != 1 || !strings.Contains(createdContents[0], "Fresh") {
		t.Fatalf("expected only the fresh item to be created, got %v", createdContents)
	}
}

func TestHandleRSSFirstFetch_baselinesAllItems(t *testing.T) {
	// First fetch in baseline mode must mark every current item as imported so
	// the lookback window cannot flood the team with the feed's backlog later.
	feed := testRSSFeed(nil)
	feed.InitialSyncMode = domain.RSSInitialSyncBaseline

	st := rssOwnerStore()
	createCalls := 0
	st.createScheduledPostFn = func(_ context.Context, _ string, _ domain.AuthenticatedPrincipal, _ domain.CreatePostInput) (domain.ScheduledPost, error) {
		createCalls++
		return domain.ScheduledPost{ID: "post-1", Status: domain.PostStatusPending}, nil
	}

	svc := New(testLogger(), st, nil, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.importRSSFeed(context.Background(), stubRSSFetcher{items: []rss.Item{
		{GUID: "item-a", Link: "https://example.com/a", Title: "A", PublishedAt: time.Now().UTC().Add(-2 * time.Hour)},
		{GUID: "item-b", Link: "https://example.com/b", Title: "B", PublishedAt: time.Now().UTC().Add(-1 * time.Hour)},
	}}, feed)

	if createCalls != 0 {
		t.Fatalf("baseline first fetch must not create posts, got %d", createCalls)
	}
	recorded := map[string]bool{}
	for _, call := range st.recordRSSImportedItemCalls {
		recorded[call.itemKey] = true
	}
	if !recorded["item-a"] || !recorded["item-b"] {
		t.Fatalf("expected both items baselined, got %+v", st.recordRSSImportedItemCalls)
	}
}

func TestHandleRSSFirstFetch_publishLatestBaselinesRest(t *testing.T) {
	feed := testRSSFeed(nil)
	feed.InitialSyncMode = domain.RSSInitialSyncPublishLatest

	st := rssOwnerStore()
	var createdContents []string
	st.createScheduledPostFn = func(_ context.Context, _ string, _ domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
		createdContents = append(createdContents, input.Content)
		return domain.ScheduledPost{ID: "post-1", Status: domain.PostStatusPending}, nil
	}

	svc := New(testLogger(), st, nil, time.Minute, 1, 0, 0, 0, 0, nil)
	svc.importRSSFeed(context.Background(), stubRSSFetcher{items: []rss.Item{
		{GUID: "old-item", Link: "https://example.com/old", Title: "Old", PublishedAt: time.Now().UTC().Add(-2 * time.Hour)},
		{GUID: "new-item", Link: "https://example.com/new", Title: "New", PublishedAt: time.Now().UTC().Add(-1 * time.Hour)},
	}}, feed)

	if len(createdContents) != 1 || !strings.Contains(createdContents[0], "New") {
		t.Fatalf("expected only the latest item published, got %v", createdContents)
	}
	recorded := map[string]bool{}
	for _, call := range st.recordRSSImportedItemCalls {
		recorded[call.itemKey] = true
	}
	if !recorded["old-item"] {
		t.Fatalf("expected older item baselined, got %+v", st.recordRSSImportedItemCalls)
	}
}
