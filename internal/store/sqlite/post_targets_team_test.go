package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

func TestSQLite_LoadPostTargets_OnlyPostTeam(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, err := s.UpsertOIDCUser(ctx, "target-team-"+uuid.NewString(), "targets@example.com", "Targets")
	if err != nil {
		t.Fatal(err)
	}
	teamA, err := s.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "target-a-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	teamB, err := s.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "target-b-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	createAccount := func(teamID, username string) domain.SocialAccount {
		t.Helper()
		account, err := s.CreateAccount(ctx, teamID, domain.ConnectedAccount{
			Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
			InstanceURL: "https://example.com", Username: username, AccessToken: "token",
		})
		if err != nil {
			t.Fatal(err)
		}
		return account
	}
	own := createAccount(teamA.ID, "own")
	posted := createAccount(teamA.ID, "posted")
	foreign := createAccount(teamB.ID, "foreign")
	// Reproduce an already-persisted post with a foreign-team target.
	post, err := s.CreateScheduledPost(ctx, teamA.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{
		Content: "team A content", ScheduledAt: time.Now().UTC().Add(-time.Minute),
		TargetAccounts: []string{own.ID, posted.ID, foreign.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostTargetResult(ctx, post.ID, posted.ID, domain.PostStatusPosted, "https://example.com/published", "", nil, ""); err != nil {
		t.Fatal(err)
	}
	targets, err := s.LoadPostTargets(ctx, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].ID != own.ID || targets[0].TeamID != teamA.ID {
		ids := make([]string, 0, len(targets))
		for _, account := range targets {
			ids = append(ids, account.ID)
		}
		t.Fatalf("LoadPostTargets IDs = %v, want only same-team unposted account %s", ids, own.ID)
	}
}
