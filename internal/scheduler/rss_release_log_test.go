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

func TestRSSDispatchFallbackPostCommitReadFailureLogsReleaseAtInfo(t *testing.T) {
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
	if !strings.Contains(logs.String(), `"level":"INFO","msg":"rss import: item already linked or unavailable"`) {
		t.Fatalf("benign release missing info log: %s", logs.String())
	}
	if strings.Contains(logs.String(), `"level":"ERROR","msg":"rss import: release item failed"`) {
		t.Fatalf("benign release logged ERROR: %s", logs.String())
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("committed posts=%d err=%v", len(posts), err)
	}
}
