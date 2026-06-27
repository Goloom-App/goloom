package sqlite_test

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

// seedFailedPost creates a scheduled post and drives it to a terminal failed
// state with a failed target, returning the post id.
func seedFailedPost(t *testing.T, ctx context.Context, s *sqlite.Store, teamID, accID string, principal domain.AuthenticatedPrincipal, errMsg string) string {
	t.Helper()
	post, err := s.CreateScheduledPost(ctx, teamID, principal, domain.CreatePostInput{
		Title: "Launch", Content: "c", ScheduledAt: time.Now().UTC().Add(-time.Hour),
		TargetAccounts: []string{accID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostTargetResult(ctx, post.ID, accID, domain.PostStatusFailed, "", errMsg, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostResult(ctx, post.ID, 5, domain.PostStatusFailed, errMsg, nil); err != nil {
		t.Fatal(err)
	}
	return post.ID
}

func TestSQLite_PublishFailures_listAcknowledgeMetric(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "pf", "pf@x", "PF")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "pf-" + uuid.NewString(), Description: ""})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "handle", AccessToken: "t",
	})
	principal := domain.AuthenticatedPrincipal{User: u}
	postID := seedFailedPost(t, ctx, s, team.ID, acc.ID, principal, "token expired")

	// Metric counts the failure.
	m, err := s.AdminMetrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if m.PostsFailed != 1 {
		t.Fatalf("PostsFailed = %d, want 1", m.PostsFailed)
	}

	// List surfaces details + the per-target error.
	failures, err := s.AdminListPublishFailures(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	f := failures[0]
	if f.PostID != postID || f.LastError != "token expired" {
		t.Fatalf("failure = %+v", f)
	}
	if f.TeamName == "" || f.AttemptCount != 5 {
		t.Fatalf("failure metadata = %+v", f)
	}
	if len(f.Targets) != 1 || f.Targets[0].LastError != "token expired" || f.Targets[0].Provider != "mastodon" {
		t.Fatalf("targets = %+v", f.Targets)
	}

	// Acknowledge removes it from attention.
	ok, err := s.AdminAcknowledgeFailedPost(ctx, postID)
	if err != nil || !ok {
		t.Fatalf("acknowledge: ok=%v err=%v", ok, err)
	}
	m, err = s.AdminMetrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if m.PostsFailed != 0 {
		t.Fatalf("after acknowledge PostsFailed = %d, want 0", m.PostsFailed)
	}
	failures, err = s.AdminListPublishFailures(ctx)
	if err != nil || len(failures) != 0 {
		t.Fatalf("after acknowledge list = %#v err=%v", failures, err)
	}

	// Acknowledging again is a no-op (not failed/unacknowledged anymore).
	if ok, _ := s.AdminAcknowledgeFailedPost(ctx, postID); ok {
		t.Fatal("second acknowledge should report ok=false")
	}
}

func TestSQLite_PublishFailures_retryRequeues(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "pfr", "pfr@x", "PFR")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "pfr-" + uuid.NewString(), Description: ""})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "h", AccessToken: "t",
	})
	principal := domain.AuthenticatedPrincipal{User: u}
	postID := seedFailedPost(t, ctx, s, team.ID, acc.ID, principal, "boom")

	ok, err := s.AdminRetryFailedPost(ctx, postID)
	if err != nil || !ok {
		t.Fatalf("retry: ok=%v err=%v", ok, err)
	}

	got, err := s.GetScheduledPostByID(ctx, postID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.PostStatusPending {
		t.Fatalf("status = %q, want pending", got.Status)
	}
	if got.AttemptCount != 0 {
		t.Fatalf("attempt_count = %d, want 0", got.AttemptCount)
	}

	// It is no longer a publish failure.
	failures, err := s.AdminListPublishFailures(ctx)
	if err != nil || len(failures) != 0 {
		t.Fatalf("after retry list = %#v err=%v", failures, err)
	}

	// Retrying a non-failed post is a no-op.
	if ok, _ := s.AdminRetryFailedPost(ctx, postID); ok {
		t.Fatal("retry of pending post should report ok=false")
	}
}

// A failed post whose target already published must not re-queue that target,
// so the scheduler won't double-post on retry.
func TestSQLite_PublishFailures_retrySkipsPostedTargets(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "pfp", "pfp@x", "PFP")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "pfp-" + uuid.NewString(), Description: ""})
	posted, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://a", Username: "ok", AccessToken: "t",
	})
	failed, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://b", Username: "bad", AccessToken: "t",
	})
	principal := domain.AuthenticatedPrincipal{User: u}
	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Title: "Multi", Content: "c", ScheduledAt: time.Now().UTC().Add(-time.Hour),
		TargetAccounts: []string{posted.ID, failed.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostTargetResult(ctx, post.ID, posted.ID, domain.PostStatusPosted, "https://a/1", "", nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostTargetResult(ctx, post.ID, failed.ID, domain.PostStatusFailed, "", "boom", nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostResult(ctx, post.ID, 5, domain.PostStatusFailed, "boom", nil); err != nil {
		t.Fatal(err)
	}

	if ok, err := s.AdminRetryFailedPost(ctx, post.ID); err != nil || !ok {
		t.Fatalf("retry: ok=%v err=%v", ok, err)
	}

	// The scheduler loads only non-posted targets, so retry re-publishes the
	// failed account but never the already-posted one.
	targets, err := s.LoadPostTargets(ctx, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].ID != failed.ID {
		t.Fatalf("LoadPostTargets after retry = %#v, want only failed account", targets)
	}
}
