package sqlite

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/hashtag"
)

func TestHashtagPerformanceAndBackfill(t *testing.T) {
	ctx := context.Background()
	s := memoryStoreAgg(t)
	u, err := s.UpsertOIDCUser(ctx, "ht", "ht@x", "HT")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ht-team", Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "x", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}

	newPostedPost := func(content string, likes int64) string {
		t.Helper()
		post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
			Content: content, ScheduledAt: time.Now().UTC(), TargetAccounts: []string{acc.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
			t.Fatal(err)
		}
		if err := s.MarkPostTargetResult(ctx, post.ID, acc.ID, domain.PostStatusPosted, "https://x/p", "", nil, ""); err != nil {
			t.Fatal(err)
		}
		if err := s.UpsertPostMetrics(ctx, post.ID, acc.ID, map[string]int64{"likes": likes}, ""); err != nil {
			t.Fatal(err)
		}
		return post.ID
	}

	p1 := newPostedPost("Release! #GoLoom #OpenSource", 10)
	p2 := newPostedPost("Update #goloom", 6)

	// Publish-time path for p1/p2.
	if err := s.ReplacePostHashtags(ctx, p1, acc.ID, hashtag.Extract("Release! #GoLoom #OpenSource")); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplacePostHashtags(ctx, p2, acc.ID, hashtag.Extract("Update #goloom")); err != nil {
		t.Fatal(err)
	}

	rows, err := s.ListTeamHashtagPerformance(ctx, team.ID, 90, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d hashtags, want 2: %#v", len(rows), rows)
	}
	// #goloom: 2 uses, 16 engagement → score 16/5 = 3.2 beats #opensource 10/4 = 2.5.
	if rows[0].Tag != "goloom" || rows[0].Uses != 2 || rows[0].TotalEngagement != 16 {
		t.Fatalf("unexpected top tag: %#v", rows[0])
	}
	if rows[0].Display != "goloom" && rows[0].Display != "GoLoom" {
		t.Fatalf("unexpected display: %q", rows[0].Display)
	}
	if rows[1].Tag != "opensource" || rows[1].Uses != 1 || rows[1].TotalEngagement != 10 {
		t.Fatalf("unexpected second tag: %#v", rows[1])
	}
	if rows[0].AvgEngagement != 8 {
		t.Fatalf("avg engagement = %v, want 8", rows[0].AvgEngagement)
	}

	// Backfill path: a third post without publish-time rows is picked up.
	newPostedPost("Backfill #Nachzügler", 3)
	if err := s.BackfillPostHashtags(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ListTeamHashtagPerformance(ctx, team.ID, 90, "mastodon", 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range rows {
		if r.Tag == "nachzügler" {
			found = true
		}
	}
	if !found {
		t.Fatalf("backfilled tag missing: %#v", rows)
	}

	// Provider filter that matches nothing.
	rows, err = s.ListTeamHashtagPerformance(ctx, team.ID, 90, "bluesky", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected no rows for bluesky filter, got %#v", rows)
	}
}

func TestGetTeamEngagementHeatmap(t *testing.T) {
	ctx := context.Background()
	s := memoryStoreAgg(t)
	u, err := s.UpsertOIDCUser(ctx, "hm", "hm@x", "HM")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "hm-team", Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "x", AccessToken: "t",
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}
	scheduledAt := time.Now().UTC().Add(-24 * time.Hour)
	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content: "x", ScheduledAt: scheduledAt, TargetAccounts: []string{acc.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPostMetrics(ctx, post.ID, acc.ID, map[string]int64{"likes": 7}, ""); err != nil {
		t.Fatal(err)
	}

	buckets, err := s.GetTeamEngagementHeatmap(ctx, team.ID, 90)
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 1 {
		t.Fatalf("got %d buckets, want 1: %#v", len(buckets), buckets)
	}
	b := buckets[0]
	wantWeekday := int(scheduledAt.Weekday())
	if b.WeekdayUTC != wantWeekday || b.HourUTC != scheduledAt.Hour() || b.Score != 7 {
		t.Fatalf("bucket = %#v, want weekday %d hour %d score 7", b, wantWeekday, scheduledAt.Hour())
	}
}
