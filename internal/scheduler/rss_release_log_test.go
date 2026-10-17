package scheduler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/rss"
	"git.f4mily.net/goloom/internal/store"
)

type rssCommittedReadErrorStore struct{ store.Store }

func (s rssCommittedReadErrorStore) CreateScheduledPost(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
	if _, err := s.Store.CreateScheduledPost(ctx, teamID, principal, input); err != nil {
		return domain.ScheduledPost{}, err
	}
	return domain.ScheduledPost{}, fmt.Errorf("injected post-commit read failure")
}

func TestRSSDispatchFallbackResponseReadFailureRollsBackAndReleases(t *testing.T) {
	ctx := context.Background()
	s, manager, team, _ := newRecurringAIFixture(t, false)
	enabled := true
	if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "Fallback", AiEnhanceEnabled: true, ContentTemplate: "{title}", OutputMode: domain.AutomationOutputDraft})
	if err != nil {
		t.Fatal(err)
	}
	logger, logs := captureLogger()
	svc := New(logger, rssCommittedReadErrorStore{s}, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
	if _, err := svc.createPostFromRSSItem(ctx, feed, rss.Item{GUID: "fallback-item", Title: "Template content", PublishedAt: time.Now().UTC()}); err == nil {
		t.Fatal("expected injected read failure")
	}
	if !strings.Contains(logs.String(), `"msg":"rss import: item attempt failed"`) {
		t.Fatalf("failed attempt missing log: %s", logs.String())
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("rolled-back posts=%d err=%v", len(posts), err)
	}
	if won, err := s.ClaimRSSItemAttempt(ctx, team.ID, feed.ID, "fallback-item"); err != nil || !won {
		t.Fatalf("retry unavailable: %v %v", won, err)
	}
	after, err := s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
	if err != nil || after.CounterNext != feed.CounterNext {
		t.Fatalf("counter=%d err=%v", after.CounterNext, err)
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.automation_create"})
	if err != nil || len(events) != 0 {
		t.Fatalf("uncommitted post audit=%v err=%v", events, err)
	}
}
