package scheduler

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
)

func TestProcessPostRejectsCrossTeamTargets(t *testing.T) {
	for _, scenario := range []string{"foreign-only", "mixed", "foreign-already-posted", "same-team", "same-team-retry"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			st, _, team, user := newRecurringAIFixture(t, false)
			other, err := st.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "other"})
			if err != nil {
				t.Fatal(err)
			}
			var own, foreign domain.SocialAccount
			for i, teamID := range []string{team.ID, other.ID} {
				account, err := st.CreateAccount(ctx, teamID, domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, InstanceURL: "https://example.test", Username: teamID, RemoteAccountID: teamID, AccessToken: "token"})
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					own = account
				} else {
					foreign = account
				}
			}
			targets := []string{foreign.ID}
			switch scenario {
			case "foreign-already-posted":
				targets = []string{foreign.ID, own.ID}
			case "mixed":
				targets = []string{own.ID, foreign.ID}
			case "same-team", "same-team-retry":
				targets = []string{own.ID}
			}
			// Bypass validation deliberately: represent a persisted legacy post.
			post, err := st.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "legacy", Content: "body", TargetAccounts: targets, ScheduledAt: time.Now().Add(-time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "foreign-already-posted" || scenario == "same-team-retry" {
				// Successful targets are excluded by LoadPostTargets on a retry.
				if err := st.MarkPostTargetResult(ctx, post.ID, targets[0], domain.PostStatusPosted, "https://example.test/posted", "", nil, ""); err != nil {
					t.Fatal(err)
				}
			}
			// Scheduler receives the full target snapshot from ListDuePosts.
			due, err := st.ListDuePosts(ctx, 10)
			if err != nil || len(due) != 1 {
				t.Fatalf("due=%v err=%v", due, err)
			}
			fp := &fakeProvider{name: "mastodon"}
			var logs bytes.Buffer
			svc := New(slog.New(slog.NewTextHandler(&logs, nil)), st, provider.NewRegistry(fp), time.Minute, 1, 0, 0, 0, 0, nil)
			svc.processPost(ctx, due[0])
			updated, err := st.GetScheduledPost(ctx, team.ID, post.ID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "same-team" || scenario == "same-team-retry" {
				expectedPublishes := 1
				if scenario == "same-team-retry" {
					expectedPublishes = 0
				}
				if updated.Status != domain.PostStatusPosted || len(fp.published) != expectedPublishes {
					t.Fatalf("status=%s publishes=%d", updated.Status, len(fp.published))
				}
				return
			}
			if updated.Status != domain.PostStatusFailed || updated.LastError == "" {
				t.Errorf("status=%s last_error=%q; want failed with error", updated.Status, updated.LastError)
			}
			if len(fp.published) != 0 {
				t.Errorf("published %d targets of inconsistent post", len(fp.published))
			}
			if !strings.Contains(logs.String(), "ERROR") {
				t.Errorf("missing boundary failure log: %s", logs.String())
			}
		})
	}
}
