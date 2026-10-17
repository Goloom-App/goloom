package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store"
)

func TestRSSDirectCreationAudit(t *testing.T) {
	ctx := context.Background()
	s, _, team, user := newRecurringAIFixture(t, false)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{Name: "Feed", FeedURL: "https://example.test/feed"})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	if err := svc.createRSSPostDirect(ctx, feed, "https://secret:credential@example.test/item?token=secret", "private source body", "private title", time.Now(), true, user.ID); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(events))
	}
	e := events[0]
	if e.Action != "post.automation_create" || e.ActorKind != "system" || e.ActorUserID != "" || e.TargetType != "post" || e.TargetID == nil || e.Metadata["feed_id"] != feed.ID || e.Metadata["initiator_user_id"] != user.ID || e.Metadata["channel"] != "scheduler" {
		t.Fatalf("wrong attribution: %+v", e)
	}
	raw, _ := json.Marshal(e)
	for _, secret := range []string{"private source body", "private title", "credential", "token=secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("audit leaked %q: %s", secret, raw)
		}
	}
}

type failingAuditStore struct{ store.Store }

func (s failingAuditStore) InsertAuditEvent(context.Context, domain.AuditEvent) error {
	return errors.New("audit unavailable")
}

type failingDeliveryStore struct{ store.Store }

func (s failingDeliveryStore) DrainAuditEvents(context.Context) error {
	return errors.New("audit delivery unavailable")
}

func TestAutomationCreationRollsBackWhenAuditFails(t *testing.T) {
	ctx := context.Background()
	s, _, team, user := newRecurringAIFixture(t, false)
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{Name: "Feed", FeedURL: "https://example.test/feed"})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(testLogger(), failingAuditStore{s}, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	if err := svc.createRSSPostDirect(ctx, feed, "item", "content", "title", time.Now(), true, user.ID); err == nil {
		t.Fatal("expected audit failure")
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Fatalf("untracked committed posts: %d", len(posts))
	}
	imported, err := s.RSSItemAlreadyImported(ctx, feed.ID, "item")
	if err != nil || imported {
		t.Fatalf("dedupe committed despite rollback: %v, %v", imported, err)
	}
}

func TestPublicationAuditsPartialOutcomes(t *testing.T) {
	ctx := context.Background()
	st := &mockStore{loadTargets: []domain.SocialAccount{{ID: "unsupported", Provider: "missing"}, {ID: "ok", Provider: "ok"}}}
	svc := New(testLogger(), st, provider.NewRegistry(&fakeProvider{name: "ok", publishRes: provider.PublishResult{RemoteID: "remote"}}), time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(ctx, domain.ScheduledPost{ID: "post", TeamID: "team", Content: "private body"})
	seen := map[string]bool{}
	for _, event := range st.auditEvents {
		if event.ActorKind != "system" || event.Metadata["post_id"] != "post" || event.Metadata["attempt"] != "1" || event.Metadata["account_id"] == "" {
			t.Fatalf("missing correlation: %+v", event)
		}
		seen[event.Action+":"+event.Metadata["account_id"]] = true
	}
	if !seen["post.publish_success:ok"] || !seen["post.publish_failure:unsupported"] {
		t.Fatalf("missing partial outcomes: %v", seen)
	}
}

func TestExternalImportAudit(t *testing.T) {
	ctx := context.Background()
	st := &mockStore{createImportedPostFn: func(context.Context, string, string, domain.ImportedPostInput) (domain.ScheduledPost, error) {
		return domain.ScheduledPost{ID: "imported"}, nil
	}}
	fp := &feedProvider{name: "feed", posts: []provider.AuthorPost{{RemoteID: "secret remote body", Content: "private body"}}}
	svc := New(testLogger(), st, provider.NewRegistry(fp), time.Minute, 1, 0, 0, 0, 0, nil)
	n, err := svc.importExternalPostsForAccount(ctx, "team", "user", domain.SocialAccount{ID: "account", Provider: "feed", RemoteAccountID: "remote"}, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("import = %d, %v", n, err)
	}
	if len(st.auditEvents) != 1 {
		t.Fatalf("events = %d, want 1", len(st.auditEvents))
	}
	e := st.auditEvents[0]
	if e.Action != "account.import" || e.Metadata["post_id"] != "imported" || e.Metadata["account_id"] != "account" || e.ActorKind != "system" {
		t.Fatalf("event: %+v", e)
	}
}

func TestUncertainPublicationCannotBeResetByPreparationFailure(t *testing.T) {
	ctx := context.Background()
	s, _, team, user := newRecurringAIFixture(t, false)
	account, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "unsupported", AuthType: domain.AccountAuthTypeOAuthToken, AccessToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Content: "body", ScheduledAt: time.Now(), TargetAccounts: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostProcessing(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimPostTargetPublication(ctx, post.ID, account.ID); err != nil {
		t.Fatal(err)
	}
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(ctx, post)
	claimed, err := s.ClaimPostTargetPublication(ctx, post.ID, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("uncertain publication reset to retryable after preparation failure")
	}
}

func TestAuditDeliveryRecoveryNeverRepeatsPublication(t *testing.T) {
	ctx := context.Background()
	s, _, team, user := newRecurringAIFixture(t, false)
	account, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "ok", AuthType: domain.AccountAuthTypeOAuthToken, AccessToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "title", Content: "body", ScheduledAt: time.Now(), TargetAccounts: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	fp := &fakeProvider{name: "ok", publishRes: provider.PublishResult{RemoteID: "remote"}}
	svc := New(testLogger(), failingDeliveryStore{s}, provider.NewRegistry(fp), time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(ctx, post)
	svc.processPost(ctx, post)
	if len(fp.published) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(fp.published))
	}
	for range 2 {
		if err := s.DrainAuditEvents(ctx); err != nil {
			t.Fatal(err)
		}
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.publish_success"})
	if err != nil || len(events) != 1 {
		t.Fatalf("recovered outcomes = %v, %v", events, err)
	}
}
