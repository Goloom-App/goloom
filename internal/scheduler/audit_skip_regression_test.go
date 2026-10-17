package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/store"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
)

func auditSkipFixture(t *testing.T) (*sqlitestore.Store, domain.Team, domain.User, domain.PostTemplate, time.Time) {
	t.Helper()
	ctx := context.Background()
	s, _, team, user := newRecurringAIFixture(t, false)
	account, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken, AccessToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, domain.AuthenticatedPrincipal{User: user}, domain.CreatePostTemplateInput{Title: "Round {counter}", Content: "Content", TargetAccountIDs: []string{account.ID}, RecurrenceJSON: `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`})
	if err != nil {
		t.Fatal(err)
	}
	return s, team, user, tmpl, time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
}
func auditSkipJob(t *testing.T, s store.Store, team domain.Team, user domain.User, tmpl domain.PostTemplate, occurrence time.Time, origin string) domain.AIJob {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"params": map[string]any{"target_account_ids": tmpl.TargetAccountIDs, "recurring_automation": map[string]any{"template_id": tmpl.ID, "template_occurrence_at": occurrence.Format(time.RFC3339), "post_kind": "main", "fallback_content": "fallback", "template_counter": 7}}})
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.CreateAIJob(context.Background(), domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Origin: origin, Type: domain.AIJobTypeVoiceEngine, Status: domain.AIJobStatusPending, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestSkipPendingAIOnlyOccurrencePreventsLateCompletion(t *testing.T) {
	ctx := context.Background()
	s, team, user, tmpl, occurrence := auditSkipFixture(t)
	job := auditSkipJob(t, s, team, user, tmpl, occurrence, domain.AIJobOriginAutomation)
	// A queued AI round has already advanced the template cursor without a post.
	next := occurrence.Add(7 * 24 * time.Hour)
	if err := s.SetPostTemplateMaterializationState(ctx, tmpl.ID, &next, 8, 1); err != nil {
		t.Fatal(err)
	}
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	if _, err := svc.SkipPostTemplateOccurrence(ctx, team.ID, tmpl.ID, occurrence, nil); err != nil {
		t.Fatal(err)
	}
	current, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.AIJobStatusFailed || current.ErrorMessage != "cancelled" {
		t.Fatalf("AI-only skipped round remains active: %+v", current)
	}
	claimed, err := s.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusCompleted, []byte(`{"content":"late callback"}`), "")
	if err != nil || claimed {
		t.Fatalf("late callback can revive skipped round: claimed=%v err=%v", claimed, err)
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("skipped round created posts: %+v %v", posts, err)
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "ai_job.cancel"})
	if err != nil || len(events) != 1 || events[0].TargetID == nil || *events[0].TargetID != job.ID || events[0].ActorKind != "system" {
		t.Fatalf("cancel evidence: %+v %v", events, err)
	}
}

type completeRecurringAfterSnapshot struct {
	store.Store
	completed bool
}

func (s *completeRecurringAfterSnapshot) ListAIJobs(ctx context.Context, teamID string, limit int) ([]domain.AIJob, error) {
	jobs, err := s.Store.ListAIJobs(ctx, teamID, limit)
	if err != nil {
		return nil, err
	}
	if !s.completed && len(jobs) > 0 {
		s.completed = true
		if err := s.Store.UpdateAIJobStatus(ctx, jobs[0].ID, domain.AIJobStatusCompleted, []byte(`{"content":"finished"}`), ""); err != nil {
			return nil, err
		}
	}
	return jobs, nil
}
func TestRecurringCancellationDoesNotOverwriteCompletedSnapshot(t *testing.T) {
	ctx := context.Background()
	s, team, user, tmpl, occurrence := auditSkipFixture(t)
	job := auditSkipJob(t, s, team, user, tmpl, occurrence, domain.AIJobOriginAutomation)
	wrapped := &completeRecurringAfterSnapshot{Store: s}
	svc := New(testLogger(), wrapped, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	err := wrapped.WithAuditTransaction(ctx, func(ctx context.Context) error {
		if err := wrapped.LockAutomationSource(ctx, "post_template", tmpl.ID); err != nil {
			return err
		}
		return svc.cancelPendingRecurringAIJobs(ctx, team.ID, tmpl.ID, &occurrence)
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.AIJobStatusCompleted {
		t.Fatalf("completed job overwritten from stale snapshot: %+v", current)
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "ai_job.cancel"})
	if err != nil || len(events) != 0 {
		t.Fatalf("false cancellation evidence: %+v %v", events, err)
	}
}

func TestRecurringCancellationIgnoresForgedUserMetadata(t *testing.T) {
	ctx := context.Background()
	s, team, user, tmpl, occurrence := auditSkipFixture(t)
	forged := auditSkipJob(t, s, team, user, tmpl, occurrence, domain.AIJobOriginUser)
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	err := s.WithAuditTransaction(ctx, func(ctx context.Context) error {
		if err := s.LockAutomationSource(ctx, "post_template", tmpl.ID); err != nil {
			return err
		}
		return svc.cancelPendingRecurringAIJobs(ctx, team.ID, tmpl.ID, &occurrence)
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.GetAIJobByID(ctx, team.ID, forged.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.AIJobStatusPending || current.Origin != domain.AIJobOriginUser {
		t.Fatalf("user job canceled through forged source metadata: %+v", current)
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "ai_job.cancel"})
	if err != nil || len(events) != 0 {
		t.Fatalf("forged metadata caused cancellation evidence: %+v %v", events, err)
	}
}

func TestSkipPendingAIOnlyPreservesLaterJobs(t *testing.T) {
	ctx := context.Background()
	s, team, user, tmpl, occurrence := auditSkipFixture(t)
	skipped := auditSkipJob(t, s, team, user, tmpl, occurrence, domain.AIJobOriginAutomation)
	later := auditSkipJob(t, s, team, user, tmpl, occurrence.Add(7*24*time.Hour), domain.AIJobOriginAutomation)
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	if _, err := svc.SkipPostTemplateOccurrence(ctx, team.ID, tmpl.ID, occurrence, nil); err != nil {
		t.Fatal(err)
	}
	first, err := s.GetAIJobByID(ctx, team.ID, skipped.ID)
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.GetAIJobByID(ctx, team.ID, later.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != domain.AIJobStatusFailed || next.Status != domain.AIJobStatusPending {
		t.Fatalf("exact skip changed wrong jobs: skipped=%s later=%s", first.Status, next.Status)
	}
}

func TestShiftPendingAIOnlyRecreatesOriginalCounter(t *testing.T) {
	ctx := context.Background()
	s, team, user, tmpl, occurrence := auditSkipFixture(t)
	job := auditSkipJob(t, s, team, user, tmpl, occurrence, domain.AIJobOriginAutomation)
	next := occurrence.Add(7 * 24 * time.Hour)
	if err := s.SetPostTemplateMaterializationState(ctx, tmpl.ID, &next, 8, 1); err != nil {
		t.Fatal(err)
	}
	shifted := occurrence.Add(2 * time.Hour)
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	if _, err := svc.SkipPostTemplateOccurrence(ctx, team.ID, tmpl.ID, occurrence, &shifted); err != nil {
		t.Fatal(err)
	}
	old, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if old.Status != domain.AIJobStatusFailed {
		t.Fatalf("shifted old job active: %s", old.Status)
	}
	posts, err := s.ListPostTemplateLinkedPosts(ctx, team.ID, tmpl.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("shift posts=%+v err=%v", posts, err)
	}
	post, err := s.GetScheduledPost(ctx, team.ID, posts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if posts[0].TemplateCounter == nil || *posts[0].TemplateCounter != 7 || !post.ScheduledAt.Equal(shifted) || !posts[0].TemplateOccurrenceAt.Equal(occurrence) {
		t.Fatalf("shift lost original slot/counter: %+v", posts[0])
	}
	fresh, err := s.GetPostTemplate(ctx, team.ID, tmpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.NextMaterializeAt == nil || !fresh.NextMaterializeAt.Equal(next) || fresh.CounterNext != 8 {
		t.Fatalf("shift changed future cursor: %+v", fresh)
	}
}

type skipAuditFailureStore struct{ store.Store }

func (s skipAuditFailureStore) InsertAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	if event.Action == "ai_job.cancel" {
		return errors.New("injected cancellation audit failure")
	}
	return s.Store.InsertAuditEvent(ctx, event)
}
func TestPendingSkipAuditFailureRollsBackSkipAndCancellation(t *testing.T) {
	ctx := context.Background()
	s, team, user, tmpl, occurrence := auditSkipFixture(t)
	job := auditSkipJob(t, s, team, user, tmpl, occurrence, domain.AIJobOriginAutomation)
	svc := New(testLogger(), skipAuditFailureStore{s}, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	if _, err := svc.SkipPostTemplateOccurrence(ctx, team.ID, tmpl.ID, occurrence, nil); err == nil {
		t.Fatal("cancellation committed without audit")
	}
	current, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.AIJobStatusPending {
		t.Fatalf("job escaped rollback: %s", current.Status)
	}
	skipped, err := s.IsPostTemplateOccurrenceSkipped(ctx, tmpl.ID, occurrence)
	if err != nil || skipped {
		t.Fatalf("skip escaped rollback: %v %v", skipped, err)
	}
}

func TestPendingOnlyShiftMissingCountersRollsBack(t *testing.T) {
	ctx := context.Background()
	s, team, user, tmpl, occurrence := auditSkipFixture(t)
	payload, err := json.Marshal(map[string]any{"params": map[string]any{"recurring_automation": map[string]any{"template_id": tmpl.ID, "template_occurrence_at": occurrence.Format(time.RFC3339), "post_kind": "main"}}})
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.CreateAIJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Origin: domain.AIJobOriginAutomation, Type: domain.AIJobTypeVoiceEngine, Status: domain.AIJobStatusPending, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	shifted := occurrence.Add(time.Hour)
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	if _, err := svc.SkipPostTemplateOccurrence(ctx, team.ID, tmpl.ID, occurrence, &shifted); err == nil {
		t.Fatal("pending-only shift accepted missing original counters")
	}
	current, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.AIJobStatusPending {
		t.Fatalf("invalid shift canceled old job: %s", current.Status)
	}
	storedShift, err := s.GetPostTemplateShiftTo(ctx, tmpl.ID, occurrence)
	if err != nil || storedShift != nil {
		t.Fatalf("invalid shift persisted: %v %v", storedShift, err)
	}
	posts, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil || len(posts) != 0 {
		t.Fatalf("invalid shift posts: %+v %v", posts, err)
	}
}
