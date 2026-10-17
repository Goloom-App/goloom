package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
	"github.com/google/uuid"
	"sync"
	"testing"
	"time"
)

func TestAuditTransactionAtomicity(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, err := s.UpsertOIDCUser(ctx, uuid.NewString(), "audit@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Before-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	event := domain.AuditEvent{TeamID: team.ID, ActorUserID: u.ID, ActorKind: domain.AuditActorHuman, Action: "team.update", TargetType: "team", TargetID: &team.ID}
	t.Run("insert failure rolls back mutation", func(t *testing.T) {
		err := s.WithAuditTransaction(ctx, func(txctx context.Context) error {
			if _, err := s.UpdateTeam(txctx, team.ID, domain.UpdateTeamInput{Name: "Lost", Description: ""}); err != nil {
				return err
			}
			bad := event
			bad.TeamID = uuid.NewString()
			return s.InsertAuditEvent(txctx, bad)
		})
		if err == nil {
			t.Fatal("expected audit insert failure")
		}
		got, err := s.GetTeamByID(ctx, team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != team.Name {
			t.Fatalf("untracked mutation persisted: %s", got.Name)
		}
	})
	t.Run("cancellation rolls back mutation", func(t *testing.T) {
		cancelctx, cancel := context.WithCancel(ctx)
		defer cancel()
		err := s.WithAuditTransaction(cancelctx, func(txctx context.Context) error {
			if _, err := s.UpdateTeam(txctx, team.ID, domain.UpdateTeamInput{Name: "Cancelled"}); err != nil {
				return err
			}
			cancel()
			return s.InsertAuditEvent(txctx, event)
		})
		if err == nil {
			t.Fatal("expected cancellation error")
		}
		got, err := s.GetTeamByID(ctx, team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != team.Name {
			t.Fatalf("cancelled mutation persisted: %s", got.Name)
		}
	})
	t.Run("nested transaction commits with outer audit", func(t *testing.T) {
		var created domain.Team
		err := s.WithAuditTransaction(ctx, func(txctx context.Context) error {
			var err error
			created, err = s.CreateTeam(txctx, u.ID, domain.CreateTeamInput{Name: "Nested"})
			if err != nil {
				return err
			}
			event.TeamID = created.ID
			event.TargetID = &created.ID
			return s.InsertAuditEvent(txctx, event)
		})
		if err != nil {
			t.Fatal(err)
		}
		events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: created.ID})
		if err != nil || len(events) != 1 {
			t.Fatalf("events: %v %v", events, err)
		}
	})
	t.Run("callback error rolls back nested transaction", func(t *testing.T) {
		var id string
		want := errors.New("abort")
		err := s.WithAuditTransaction(ctx, func(txctx context.Context) error {
			created, err := s.CreateTeam(txctx, u.ID, domain.CreateTeamInput{Name: "Aborted"})
			if err != nil {
				return err
			}
			id = created.ID
			return want
		})
		if !errors.Is(err, want) {
			t.Fatalf("error: %v", err)
		}
		if _, err := s.GetTeamByID(ctx, id); err == nil {
			t.Fatalf("nested mutation persisted: %v", err)
		}
	})
}

func TestAuditOrderingIdempotencyAndFilters(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, err := s.UpsertOIDCUser(ctx, uuid.NewString(), "order@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Ordering"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	// Insertion order deliberately differs from deterministic descending-ID order.
	for _, id := range ids {
		e := domain.AuditEvent{ID: id, TeamID: team.ID, ActorUserID: u.ID, ActorKind: domain.AuditActorHuman, Action: "post.create", TargetType: "post", CreatedAt: now, Metadata: map[string]string{"channel": "scheduler", "outcome": "success"}}
		if err := s.InsertAuditEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
		if err := s.InsertAuditEvent(ctx, e); err != nil {
			t.Fatalf("replayed event: %v", err)
		}
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.create", ActorUserID: u.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("duplicate events: %d", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i-1].ID <= events[i].ID {
			t.Fatal("unstable tied timestamp order")
		}
	}
	page, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Limit: 1, Offset: 1})
	if err != nil || len(page) != 1 || page[0].ID != events[1].ID {
		t.Fatalf("page: %v %v", page, err)
	}
	count, err := s.CountAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.create", ActorUserID: u.ID})
	if err != nil || count != 3 {
		t.Fatalf("count %d: %v", count, err)
	}
	before := now.Add(-time.Second)
	after := now.Add(time.Second)
	for _, f := range []domain.AuditFilter{{TeamID: team.ID, Before: &before}, {TeamID: team.ID, After: &after}, {TeamID: team.ID, ActorUserID: "other"}, {TeamID: team.ID, Action: "post.delete"}} {
		n, err := s.CountAuditEvents(ctx, f)
		if err != nil || n != 0 {
			t.Fatalf("filter: %d %v", n, err)
		}
	}
}

func TestClaimPostTargetPublication(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, err := s.UpsertOIDCUser(ctx, uuid.NewString(), "claim@example.test", "Claim")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Claim"})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "bluesky", AuthType: domain.AccountAuthTypeAppPassword, InstanceURL: "https://b.social", Username: "b", AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: u}, domain.CreatePostInput{Content: "test", ScheduledAt: time.Now(), TargetAccounts: []string{acc.ID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []bool{true, false} {
		got, err := s.ClaimPostTargetPublication(ctx, post.ID, acc.ID)
		if err != nil || got != want {
			t.Fatalf("claim %v: %v, want %v", got, err, want)
		}
	}
	if err := s.MarkPostTargetResult(ctx, post.ID, acc.ID, domain.PostStatusFailed, "", "confirmed_failure", nil, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ClaimPostTargetPublication(ctx, post.ID, acc.ID); err != nil || !got {
		t.Fatalf("retry claim %v: %v", got, err)
	}
	if err := s.MarkPostTargetResult(ctx, post.ID, acc.ID, domain.PostStatusPosted, "https://b.social/post", "", nil, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ClaimPostTargetPublication(ctx, post.ID, acc.ID); err != nil || got {
		t.Fatalf("posted replay %v: %v", got, err)
	}
}

func TestAuditAfterCommitHooks(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	calls := 0
	if err := s.WithAuditTransaction(ctx, func(txctx context.Context) error {
		store.AfterAuditCommit(txctx, func(cleanctx context.Context) {
			calls++
			// This starts a new transaction: a stale committed tx would fail.
			if err := s.WithAuditTransaction(cleanctx, func(context.Context) error { return nil }); err != nil {
				t.Errorf("stale hook context: %v", err)
			}
		})
		if calls != 0 {
			t.Fatal("hook before commit")
		}
		abort := errors.New("nested abort")
		if err := s.WithAuditTransaction(txctx, func(nested context.Context) error {
			store.AfterAuditCommit(nested, func(context.Context) { t.Error("rolled back nested hook ran") })
			return abort
		}); !errors.Is(err, abort) {
			t.Fatal(err)
		}
		return s.WithAuditTransaction(txctx, func(nested context.Context) error {
			store.AfterAuditCommit(nested, func(context.Context) { calls++ })
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("hooks %d", calls)
	}
	_ = s.WithAuditTransaction(ctx, func(txctx context.Context) error {
		store.AfterAuditCommit(txctx, func(context.Context) { t.Error("rolled back outer hook ran") })
		return errors.New("abort")
	})
	store.AfterAuditCommit(ctx, func(context.Context) { calls++ })
	if calls != 3 {
		t.Fatal("outside hook not immediate")
	}
}
func TestAuditTransactionClaimAIJobCompletion(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, err := s.UpsertOIDCUser(ctx, uuid.NewString(), "job@example.test", "Job")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Job-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.CreateAIJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: u.ID, Type: domain.AIJobTypeVoiceEngine, Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusProcessing, nil, ""); err == nil || got {
		t.Fatalf("nonterminal claim %v: %v", got, err)
	}
	if got, err := s.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusCompleted, []byte(`{"count":1}`), ""); err != nil || !got {
		t.Fatalf("first claim %v: %v", got, err)
	}
	if got, err := s.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusFailed, nil, "replay"); err != nil || got {
		t.Fatalf("replayed claim %v: %v", got, err)
	}
	got, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.AIJobStatusCompleted {
		t.Fatalf("replay overwrote completion: %s", got.Status)
	}
}
func TestMarkPostProcessingClaimsOnce(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, err := s.UpsertOIDCUser(ctx, uuid.NewString(), "processing@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Processing-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: u}, domain.CreatePostInput{Content: "test", ScheduledAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostProcessing(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostProcessing(ctx, post.ID); err == nil {
		t.Fatal("duplicate queue claim accepted")
	}
	for _, status := range []domain.PostStatus{domain.PostStatusDraft, domain.PostStatusPosted, domain.PostStatusCancelled} {
		if err := s.MarkPostResult(ctx, post.ID, 0, status, "", nil); err != nil {
			t.Fatal(err)
		}
		if err := s.MarkPostProcessing(ctx, post.ID); err == nil {
			t.Fatalf("claimed %s post", status)
		}
	}
	if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusFailed, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostProcessing(ctx, post.ID); err != nil {
		t.Fatal("failed post must remain retryable", err)
	}
}
func TestRepairFuturePostedPostsAuditsBothTeams(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, err := s.UpsertOIDCUser(ctx, uuid.NewString(), "repair@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	tokenID := "initiating-token"
	ctx = security.WithPrincipal(ctx, domain.AuthenticatedPrincipal{User: u, Kind: domain.AuditActorToken, TokenID: &tokenID})
	for range 2 {
		team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Repair-" + uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: u}, domain.CreatePostInput{Content: "secret post body", ScheduledAt: time.Now().Add(time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.RepairFuturePostedPosts(ctx)
	if err != nil || n < 2 {
		t.Fatalf("repair %d: %v", n, err)
	}
	teams, err := s.ListTeamsForUser(ctx, u.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, team := range teams {
		events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.repair"})
		if err != nil || len(events) != 1 {
			t.Fatalf("events %v: %v", events, err)
		}
		e := events[0]
		if e.ActorKind != "system" || e.TargetType != "post" || e.TargetID == nil || e.Metadata["initiator_user_id"] != u.ID || e.Metadata["initiator_token_id"] != tokenID || e.Metadata["status_before"] != "posted" || e.Metadata["status_after"] != "pending" {
			t.Fatalf("bad repair attribution: %+v", e)
		}
	}
	n, err = s.RepairFuturePostedPosts(ctx)
	if err != nil || n != 0 {
		t.Fatalf("repair replay %d: %v", n, err)
	}
}
func TestAutomationSourceLockAndPendingOccurrence(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, err := s.UpsertOIDCUser(ctx, uuid.NewString(), "source@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Source-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	template, err := s.CreatePostTemplate(ctx, team.ID, domain.AuthenticatedPrincipal{User: u}, domain.CreatePostTemplateInput{Title: "Template", Content: "secret", RecurrenceJSON: `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{Name: "Feed", FeedURL: "https://example.test/feed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LockAutomationSource(ctx, "post_template", template.ID); err == nil {
		t.Fatal("lock outside transaction accepted")
	}
	for _, item := range []struct{ kind, id string }{{"rss_feed", feed.ID}, {"post_template", template.ID}} {
		if err := s.WithAuditTransaction(ctx, func(txctx context.Context) error { return s.LockAutomationSource(txctx, item.kind, item.id) }); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.WithAuditTransaction(ctx, func(txctx context.Context) error { return s.LockAutomationSource(txctx, "invalid", template.ID) }); err == nil {
		t.Fatal("unknown lock kind accepted")
	}
	if err := s.WithAuditTransaction(ctx, func(txctx context.Context) error {
		return s.LockAutomationSource(txctx, "post_template", uuid.NewString())
	}); err == nil {
		t.Fatal("missing source accepted")
	}
	occurrence := time.Now().UTC().Truncate(time.Second)
	payload := []byte(fmt.Sprintf(`{"params":{"recurring_automation":{"template_id":%q,"template_occurrence_at":%q,"post_kind":"main"}}}`, template.ID, occurrence.Format(time.RFC3339)))
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	start := make(chan struct{})
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- s.WithAuditTransaction(ctx, func(txctx context.Context) error {
				if err := s.LockAutomationSource(txctx, "post_template", template.ID); err != nil {
					return err
				}
				exists, err := s.HasPostTemplateRoleMaterialized(txctx, template.ID, occurrence, "main")
				if err != nil || exists {
					return err
				}
				_, err = s.CreateAIJob(txctx, domain.AIJob{TeamID: team.ID, AuthorUserID: u.ID, Origin: domain.AIJobOriginAutomation, Type: domain.AIJobTypeVoiceEngine, Payload: payload})
				return err
			})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	jobs, err := s.ListAIJobs(ctx, team.ID, 20)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("duplicate source jobs %d: %v", len(jobs), err)
	}
	for _, status := range []domain.AIJobStatus{domain.AIJobStatusPending, domain.AIJobStatusProcessing, domain.AIJobStatusCompleted, domain.AIJobStatusFailed} {
		if err := s.UpdateAIJobStatus(ctx, jobs[0].ID, status, nil, ""); err != nil {
			t.Fatal(err)
		}
		exists, err := s.HasPostTemplateRoleMaterialized(ctx, template.ID, occurrence, "main")
		want := status == domain.AIJobStatusPending || status == domain.AIJobStatusProcessing
		if err != nil || exists != want {
			t.Fatalf("pending occurrence %s = %v, want %v: %v", status, exists, want, err)
		}
		other, err := s.HasPostTemplateRoleMaterialized(ctx, template.ID, occurrence, "announcement")
		if err != nil || other {
			t.Fatalf("different role matched: %v %v", other, err)
		}
	}
	// Reserved automation metadata from another team must never claim this source.
	otherTeam, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Other-source-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAIJob(ctx, domain.AIJob{TeamID: otherTeam.ID, AuthorUserID: u.ID, Origin: domain.AIJobOriginAutomation, Type: domain.AIJobTypeVoiceEngine, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	exists, err := s.HasPostTemplateRoleMaterialized(ctx, template.ID, occurrence, "main")
	if err != nil || exists {
		t.Fatalf("foreign-team job claimed source: %v %v", exists, err)
	}

}
