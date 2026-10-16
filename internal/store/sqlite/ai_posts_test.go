package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestListRecentPostedPosts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := makeAITeam(t, s)
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m.example", Username: "recentu", AccessToken: "tok",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}

	base := time.Now().UTC().Add(-time.Hour)
	mk := func(content string, at time.Time, post bool) domain.ScheduledPost {
		p, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
			Content: content, ScheduledAt: at, TargetAccounts: []string{acc.ID},
		})
		if err != nil {
			t.Fatalf("CreateScheduledPost: %v", err)
		}
		if post {
			if err := s.MarkPostResult(ctx, p.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
				t.Fatalf("MarkPostResult: %v", err)
			}
		}
		return p
	}
	oldest := mk("oldest", base.Add(-2*time.Minute), true)
	newest := mk("newest", base, true)
	mk("not posted", base.Add(-time.Minute), false)

	got, err := s.ListRecentPostedPosts(ctx, team.ID, 10)
	if err != nil {
		t.Fatalf("ListRecentPostedPosts: %v", err)
	}
	if len(got) != 2 || got[0].ID != newest.ID || got[1].ID != oldest.ID {
		t.Fatalf("want [newest oldest] posted only, got %+v", got)
	}
	for _, p := range got {
		if len(p.TargetAccounts) != 1 || p.TargetAccounts[0] != acc.ID {
			t.Fatalf("post %s targets = %v, want [%s]", p.ID, p.TargetAccounts, acc.ID)
		}
	}

	limited, err := s.ListRecentPostedPosts(ctx, team.ID, 1)
	if err != nil || len(limited) != 1 || limited[0].ID != newest.ID {
		t.Fatalf("limit 1: err=%v got=%+v", err, limited)
	}

	empty, err := s.ListRecentPostedPosts(ctx, "00000000-0000-0000-0000-000000000000", 10)
	if err != nil {
		t.Fatalf("unknown team: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("unknown team: want none, got %d", len(empty))
	}
}

func TestListUpcomingPosts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := makeAITeam(t, s)
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m.example", Username: "upcomingu", AccessToken: "tok",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}
	now := time.Now().UTC()

	mk := func(content string, at time.Time, draft bool) domain.ScheduledPost {
		p, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
			Content: content, ScheduledAt: at, TargetAccounts: []string{acc.ID}, Draft: draft,
		})
		if err != nil {
			t.Fatalf("CreateScheduledPost: %v", err)
		}
		return p
	}
	later := mk("later", now.Add(3*time.Hour), false)
	soon := mk("soon draft", now.Add(time.Hour), true)
	past := mk("past", now.Add(-time.Hour), false)
	_ = past
	posted := mk("posted", now.Add(2*time.Hour), false)
	if err := s.MarkPostResult(ctx, posted.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
		t.Fatalf("MarkPostResult: %v", err)
	}

	got, err := s.ListUpcomingPosts(ctx, team.ID, now, 10)
	if err != nil {
		t.Fatalf("ListUpcomingPosts: %v", err)
	}
	if len(got) != 2 || got[0].ID != soon.ID || got[1].ID != later.ID {
		t.Fatalf("want [soon later], got %+v", got)
	}
	for _, p := range got {
		if len(p.TargetAccounts) != 1 || p.TargetAccounts[0] != acc.ID {
			t.Fatalf("post %s targets = %v, want [%s]", p.ID, p.TargetAccounts, acc.ID)
		}
	}

	limited, err := s.ListUpcomingPosts(ctx, team.ID, now, 1)
	if err != nil || len(limited) != 1 || limited[0].ID != soon.ID {
		t.Fatalf("limit 1: err=%v got=%+v", err, limited)
	}
}
