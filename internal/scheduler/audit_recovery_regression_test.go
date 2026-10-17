package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/rss"
	"git.f4mily.net/goloom/internal/store"
)

type recoveryAuditFailureStore struct {
	store.Store
	failPublicationStart bool
	failAnnouncement     bool
}

func (s *recoveryAuditFailureStore) EnqueueAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	if s.failPublicationStart && event.Action == "post.publish_started" {
		return errors.New("injected publication start audit failure")
	}
	return s.Store.EnqueueAuditEvent(ctx, event)
}
func (s *recoveryAuditFailureStore) InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	if s.failAnnouncement && event.Action == "post.automation_create" && event.Metadata["post_kind"] == domain.TemplatePostRoleAnnouncement {
		return errors.New("injected announcement audit failure")
	}
	return s.Store.InsertAuditEvent(ctx, event)
}

func TestPublicationStartAuditFailureKeepsPostRetryable(t *testing.T) {
	ctx := context.Background()
	s, _, team, user := newRecurringAIFixture(t, false)
	account, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "ok", AuthType: domain.AccountAuthTypeOAuthToken, AccessToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostInput{Title: "title", Content: "body", ScheduledAt: time.Now().Add(-time.Minute), TargetAccounts: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostProcessing(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	wrapped := &recoveryAuditFailureStore{Store: s, failPublicationStart: true}
	fp := &fakeProvider{name: "ok", publishRes: provider.PublishResult{RemoteID: "remote"}}
	svc := New(testLogger(), wrapped, provider.NewRegistry(fp), time.Minute, 1, 0, 0, 0, 0, nil)
	svc.processPost(ctx, post)
	if len(fp.published) != 0 {
		t.Fatalf("provider called before durable start evidence: %d", len(fp.published))
	}
	fresh, err := s.GetScheduledPost(ctx, team.ID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status != domain.PostStatusPending && fresh.Status != domain.PostStatusFailed {
		t.Fatalf("audit failure stranded post in %q; want retryable status", fresh.Status)
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("failed claim emitted success evidence: %+v", events)
	}
	// Recovery retries only the previously unattempted publication.
	wrapped.failPublicationStart = false
	if err := s.MarkPostProcessing(ctx, post.ID); err != nil {
		t.Fatalf("retry claim: %v", err)
	}
	svc.processPost(ctx, fresh)
	if len(fp.published) != 1 {
		t.Fatalf("recovered provider calls = %d, want 1", len(fp.published))
	}
	fresh, err = s.GetScheduledPost(ctx, team.ID, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status != domain.PostStatusPosted {
		t.Fatalf("recovered status = %s", fresh.Status)
	}
	events, err = s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.publish_success"})
	if err != nil || len(events) != 1 {
		t.Fatalf("recovered success events = %+v, %v", events, err)
	}
}

func TestAnnouncementAuditFailureRollsBackWholeOccurrence(t *testing.T) {
	ctx := context.Background()
	s, _, team, user := newRecurringAIFixture(t, false)
	account, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, AccessToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostTemplateInput{
		Title: "Round {counter}", Content: "Round {counter}", TargetAccountIDs: []string{account.ID},
		RecurrenceJSON: `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`,
		Enabled:        boolPtr(true), MaterializeHorizonDays: intPtr(30), CounterNext: intPtr(7),
		AnnouncementEnabled: boolPtr(true), AnnouncementTitle: "Soon {main_counter}", AnnouncementContent: "Announcement {main_counter}", AnnouncementDaysBefore: intPtr(2), AnnouncementCounterNext: intPtr(3),
	})
	if err != nil {
		t.Fatal(err)
	}
	occurrence := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
	if err := s.SetPostTemplateMaterializationState(ctx, tmpl.ID, &occurrence, 7, 3); err != nil {
		t.Fatal(err)
	}
	tmpl, err = s.GetPostTemplate(ctx, team.ID, tmpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := tmpl
	wrapped := &recoveryAuditFailureStore{Store: s, failAnnouncement: true}
	svc := New(testLogger(), wrapped, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	advanced, err := svc.materializeTemplateOccurrence(ctx, &tmpl, occurrence)
	if err == nil || advanced {
		t.Fatalf("announcement audit failure accepted: advanced=%v err=%v", advanced, err)
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Fatalf("partial occurrence committed: %d posts", len(posts))
	}
	fresh, err := s.GetPostTemplate(ctx, team.ID, tmpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.NextMaterializeAt == nil || !fresh.NextMaterializeAt.Equal(*before.NextMaterializeAt) || fresh.CounterNext != before.CounterNext || fresh.AnnouncementCounterNext != before.AnnouncementCounterNext {
		t.Fatalf("cursor/counters escaped rollback: before=%+v after=%+v", before, fresh)
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("rolled-back occurrence left evidence: %+v", events)
	}
	wrapped.failAnnouncement = false
	advanced, err = svc.materializeTemplateOccurrence(ctx, &fresh, occurrence)
	if err != nil || !advanced {
		t.Fatalf("occurrence recovery: advanced=%v err=%v", advanced, err)
	}
	// Replaying the same occurrence cannot add another post or audit event.
	if _, err := svc.materializeTemplateOccurrence(ctx, &fresh, occurrence); err != nil {
		t.Fatal(err)
	}
	posts, err = s.ListTeamPosts(ctx, team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 {
		t.Fatalf("recovered occurrence posts = %d, want 2", len(posts))
	}
	linked, err := s.ListPostTemplateLinkedPosts(ctx, team.ID, tmpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]int{}
	for _, post := range linked {
		roles[post.TemplatePostRole]++
	}
	if roles[domain.TemplatePostRoleMain] != 1 || roles[domain.TemplatePostRoleAnnouncement] != 1 {
		t.Fatalf("recovered roles: %v", roles)
	}
	fresh, err = s.GetPostTemplate(ctx, team.ID, tmpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.CounterNext != 8 || fresh.AnnouncementCounterNext != 4 || fresh.NextMaterializeAt == nil || !fresh.NextMaterializeAt.After(occurrence) {
		t.Fatalf("recovered cursor/counters: %+v", fresh)
	}
	events, err = s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.automation_create"})
	if err != nil || len(events) != 2 {
		t.Fatalf("recovered creation events = %+v, %v", events, err)
	}
}

// Only database contention is acceptable during a concurrent SQLite source run.
// Validation, transaction and audit errors must still fail the integration check.
func runConcurrentAuditSource(t *testing.T, run func() error) {
	t.Helper()
	const workers = 6
	start := make(chan struct{})
	outcomes := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; outcomes <- run() }()
	}
	close(start)
	wg.Wait()
	close(outcomes)
	for err := range outcomes {
		if err == nil {
			continue
		}
		var sqliteError interface{ Code() int }
		if !errors.As(err, &sqliteError) || (sqliteError.Code()&0xff != 5 && sqliteError.Code()&0xff != 6) {
			t.Fatalf("unexpected source error: %v", err)
		}
	}
}

func TestConcurrentRSSSourceCreatesOneAuditedPost(t *testing.T) {
	ctx := context.Background()
	s, _, team, _ := newRecurringAIFixture(t, false)
	account, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, AccessToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	feed, err := s.CreateRSSFeedConfig(ctx, team.ID, domain.RSSFeedConfig{Name: "Feed", FeedURL: "https://example.test/feed", OutputMode: domain.AutomationOutputDraft, TargetAccountIDs: []string{account.ID}})
	if err != nil {
		t.Fatal(err)
	}
	item := rss.Item{GUID: "same-item", Link: "https://example.test/item", Title: "Item", Content: "source body", PublishedAt: time.Now().UTC()}
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	var created atomic.Int64
	runConcurrentAuditSource(t, func() error {
		made, err := svc.createPostFromRSSItem(ctx, feed, item)
		if made {
			created.Add(1)
		}
		return err
	})
	if created.Load() != 1 {
		t.Fatalf("source creation count = %d, want 1", created.Load())
	}
	if made, err := svc.createPostFromRSSItem(ctx, feed, item); err != nil || made {
		t.Fatalf("RSS replay = %v, %v", made, err)
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("source posts = %+v, %v", posts, err)
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.automation_create"})
	if err != nil || len(events) != 1 {
		t.Fatalf("source events = %+v, %v", events, err)
	}
	if events[0].TargetID == nil || *events[0].TargetID != posts[0].ID || events[0].Metadata["feed_id"] != feed.ID {
		t.Fatalf("RSS correlation: %+v", events[0])
	}
	fresh, err := s.GetRSSFeedConfigByID(ctx, team.ID, feed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.CounterNext != feed.CounterNext+1 {
		t.Fatalf("RSS counter advanced %d times", fresh.CounterNext-feed.CounterNext)
	}
}

func TestConcurrentRecurringSourceIgnoresForgedUserJob(t *testing.T) {
	ctx := context.Background()
	s, _, team, user := newRecurringAIFixture(t, false)
	account, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, AccessToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostTemplateInput{
		Title: "Round {counter}", Content: "Round {counter}", TargetAccountIDs: []string{account.ID},
		RecurrenceJSON: `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`,
		OutputMode:     domain.AutomationOutputDraft,
	})
	if err != nil {
		t.Fatal(err)
	}
	occurrence := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
	// Untrusted user jobs may contain source-shaped JSON. Provenance prevents
	// that metadata from reserving a real automation occurrence.
	payload, err := json.Marshal(map[string]any{"params": map[string]any{"recurring_automation": map[string]any{"template_id": tmpl.ID, "template_occurrence_at": occurrence.Format(time.RFC3339), "post_kind": domain.TemplatePostRoleMain}}})
	if err != nil {
		t.Fatal(err)
	}
	forged, err := s.CreateAIJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Origin: domain.AIJobOriginUser, Type: domain.AIJobTypeVoiceEngine, Status: domain.AIJobStatusPending, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	runConcurrentAuditSource(t, func() error {
		local := tmpl
		return svc.createScheduledPostFromTemplate(ctx, &local, occurrence, occurrence, domain.TemplatePostRoleMain)
	})
	if err := svc.createScheduledPostFromTemplate(ctx, &tmpl, occurrence, occurrence, domain.TemplatePostRoleMain); err != nil {
		t.Fatal(err)
	}
	posts, err := s.ListPostTemplateLinkedPosts(ctx, team.ID, tmpl.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("recurring source posts = %+v, %v", posts, err)
	}
	if posts[0].TemplatePostRole != domain.TemplatePostRoleMain || !posts[0].TemplateOccurrenceAt.Equal(occurrence) {
		t.Fatalf("recurring source correlation: %+v", posts[0])
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "post.automation_create"})
	if err != nil || len(events) != 1 {
		t.Fatalf("recurring source events = %+v, %v", events, err)
	}
	if events[0].TargetID == nil || *events[0].TargetID != posts[0].ID || events[0].Metadata["template_id"] != tmpl.ID {
		t.Fatalf("recurring event correlation: %+v", events[0])
	}
	unchanged, err := s.GetAIJobByID(ctx, team.ID, forged.ID)
	if err != nil || unchanged.Status != domain.AIJobStatusPending || unchanged.Origin != domain.AIJobOriginUser {
		t.Fatalf("forged user job changed: %+v, %v", unchanged, err)
	}
}
