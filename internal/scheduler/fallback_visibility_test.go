package scheduler

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/rss"
)

func TestRecurringDispatchFailureCreatesVisibleFallback(t *testing.T) {
	for _, announcement := range []bool{false, true} {
		t.Run(map[bool]string{false: "main", true: "announcement"}[announcement], func(t *testing.T) {
			ctx := context.Background()
			s, manager, team, user := newRecurringAIFixture(t, false)
			enabled := true
			if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
				t.Fatal(err)
			}
			acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", InstanceURL: "https://example.test", Username: "fallback", AccessToken: "tok", AuthType: domain.AccountAuthTypeOAuthToken})
			if err != nil {
				t.Fatal(err)
			}
			tpl, err := s.CreatePostTemplate(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostTemplateInput{Title: "Template", Content: "Template content", TargetAccountIDs: []string{acc.ID}, RecurrenceJSON: `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`, Enabled: &enabled})
			if err != nil {
				t.Fatal(err)
			}
			tpl.AiEnhanceEnabled = true
			tpl.AnnouncementEnabled = true
			tpl.AiEnhanceAnnouncement = true
			tpl.AnnouncementContent = "Announcement content"
			tpl.AnnouncementCounterNext = 1
			svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
			at := time.Now().UTC().Add(72 * time.Hour)
			if announcement {
				err = svc.materializeAnnouncement(ctx, &tpl, at, true)
			} else {
				err = svc.createScheduledPostFromTemplate(ctx, &tpl, at, at, domain.TemplatePostRoleMain)
			}
			if err != nil {
				t.Fatal(err)
			}
			posts, err := s.ListTeamPosts(ctx, team.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(posts) != 1 {
				t.Fatalf("dispatch failure lost template: posts=%d", len(posts))
			}
			if posts[0].AIFallbackReason == nil || *posts[0].AIFallbackReason != "dispatch_failed" {
				t.Fatalf("fallback reason=%v", posts[0].AIFallbackReason)
			}
		})
	}
}

func TestRSSDispatchFailureCreatesVisibleFallback(t *testing.T) {
	ctx := context.Background()
	s, manager, team, _ := newRecurringAIFixture(t, false)
	enabled := true
	if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", InstanceURL: "https://example.test", Username: "fallback", AccessToken: "tok", AuthType: domain.AccountAuthTypeOAuthToken})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{FeedURL: "https://example.test/feed", Name: "Fallback", TargetAccountIDs: []string{acc.ID}, AiEnhanceEnabled: true, ContentTemplate: "{title}"})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
	created, err := svc.createPostFromRSSItem(ctx, feed, rss.Item{GUID: "fallback-item", Title: "Template content", PublishedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("dispatch failure lost RSS item")
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 {
		t.Fatalf("posts=%d", len(posts))
	}
	if posts[0].AIFallbackReason == nil || *posts[0].AIFallbackReason != "dispatch_failed" {
		t.Fatalf("fallback reason=%v", posts[0].AIFallbackReason)
	}
	created, err = svc.createPostFromRSSItem(ctx, feed, rss.Item{GUID: "fallback-item", Title: "Template content"})
	if err != nil || created {
		t.Fatalf("fallback imported twice: created=%v err=%v", created, err)
	}
}
