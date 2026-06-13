package sqlite

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
)

func TestListTeamPostEngagement(t *testing.T) {
	ctx := context.Background()
	s := memoryStoreAgg(t)
	u, err := s.UpsertOIDCUser(ctx, "pe", "pe@x", "PE")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "pe-team"})
	if err != nil {
		t.Fatal(err)
	}
	masto, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m", Username: "m", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	bsky, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "bluesky", AuthType: domain.AccountAuthTypeAppPassword,
		InstanceURL: "https://b", Username: "b", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}

	newPosted := func(at time.Time, acc domain.SocialAccount, likes int64) {
		t.Helper()
		post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
			Content: "x", ScheduledAt: at, TargetAccounts: []string{acc.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertPostMetrics(ctx, post.ID, acc.ID, map[string]int64{"likes": likes}, ""); err != nil {
			t.Fatal(err)
		}
	}

	mon := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	newPosted(mon, masto, 12)
	newPosted(mon.AddDate(0, 0, 1), bsky, 5)

	// No provider filter: both posts, each with its own engagement total.
	all, err := s.ListTeamPostEngagement(ctx, team.ID, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d rows, want 2: %#v", len(all), all)
	}
	var total int64
	for _, r := range all {
		total += r.Engagement
		if r.ScheduledAt.IsZero() {
			t.Fatalf("scheduled_at not parsed: %#v", r)
		}
	}
	if total != 17 {
		t.Fatalf("total engagement = %d, want 17", total)
	}

	// Provider filter keeps only the mastodon post.
	masts, err := s.ListTeamPostEngagement(ctx, team.ID, 0, "mastodon")
	if err != nil {
		t.Fatal(err)
	}
	if len(masts) != 1 || masts[0].Engagement != 12 {
		t.Fatalf("mastodon rows = %#v, want one row of 12", masts)
	}
	if got := masts[0].ScheduledAt.UTC(); !got.Equal(mon) {
		t.Fatalf("scheduled_at = %v, want %v", got, mon)
	}

	// A provider with no posts yields nothing.
	none, err := s.ListTeamPostEngagement(ctx, team.ID, 0, "friendica")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("friendica rows = %#v, want none", none)
	}
}
