package scheduler

import (
	"context"
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
