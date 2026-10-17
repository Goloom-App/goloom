package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/rss"
	"git.f4mily.net/goloom/internal/store"
)

type rssRetryInsertFailureStore struct {
	store.Store
	fail     bool
	attempts int
}

func (s *rssRetryInsertFailureStore) CreateScheduledPost(ctx context.Context, teamID string, p domain.AuthenticatedPrincipal, in domain.CreatePostInput) (domain.ScheduledPost, error) {
	s.attempts++
	if s.fail {
		return domain.ScheduledPost{}, errors.New("injected RSS insertion failure")
	}
	return s.Store.CreateScheduledPost(ctx, teamID, p, in)
}

func TestRSSDispatchFailureRetainsBoundedAttemptsAndAudit(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(map[bool]string{false: "transient", true: "permanent"}[permanent], func(t *testing.T) {
			ctx := context.Background()
			s, manager, team, _ := newRecurringAIFixture(t, false)
			enabled := true
			if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
				t.Fatal(err)
			}
			feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "retry", AiEnhanceEnabled: true, ContentTemplate: "{title}", OutputMode: domain.AutomationOutputDraft})
			if err != nil {
				t.Fatal(err)
			}
			wrapped := &rssRetryInsertFailureStore{Store: s, fail: true}
			svc := New(testLogger(), wrapped, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
			item := rss.Item{GUID: "retry-audit", Title: "Template", PublishedAt: time.Now().UTC()}
			for poll := 0; poll < 5; poll++ {
				if !permanent && poll == 1 {
					wrapped.fail = false
				}
				created, err := svc.createPostFromRSSItem(ctx, feed, item)
				if poll == 0 || (permanent && poll < domain.MaxRSSItemAttempts) {
					if err == nil || created {
						t.Fatalf("poll %d: expected insertion failure: created=%v err=%v", poll, created, err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
			}
			wantAttempts, wantPosts, wantExhausted := 2, 1, 0
			if permanent {
				wantAttempts, wantPosts, wantExhausted = domain.MaxRSSItemAttempts, 0, 1
			}
			posts, err := s.ListTeamPosts(ctx, team.ID)
			if err != nil {
				t.Fatal(err)
			}
			if wrapped.attempts != wantAttempts || len(posts) != wantPosts {
				t.Fatalf("attempts=%d posts=%d want=%d/%d", wrapped.attempts, len(posts), wantAttempts, wantPosts)
			}
			after, err := s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.ExhaustedItemsCount != wantExhausted || after.CounterNext != feed.CounterNext+wantPosts {
				t.Fatalf("counter=%d exhausted=%d want=%d/%d", after.CounterNext, after.ExhaustedItemsCount, feed.CounterNext+wantPosts, wantExhausted)
			}
			events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.automation_create"})
			if err != nil || len(events) != wantPosts {
				t.Fatalf("events=%v err=%v", events, err)
			}
			if wantPosts > 0 && (events[0].Metadata["outcome"] != "fallback" || posts[0].AIFallbackReason == nil || *posts[0].AIFallbackReason != "dispatch_failed") {
				t.Fatalf("fallback evidence missing: post=%+v event=%+v", posts[0], events[0])
			}
			jobs, err := s.ListAIJobs(ctx, team.ID, 10)
			if err != nil || len(jobs) != 0 {
				t.Fatalf("failed dispatch leaked jobs=%v err=%v", jobs, err)
			}
		})
	}
}
