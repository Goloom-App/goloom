package aijobs_test

import (
	"context"
	"errors"
	"testing"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/store"
)

type auditFailStore struct{ store.Store }

func (s auditFailStore) InsertAuditEvent(context.Context, domain.AuditEvent) error {
	return errors.New("audit unavailable")
}

func TestSubmitJobAuditFailureLeavesNoJob(t *testing.T) {
	s := newJobTestStore(t)
	user, team := newJobTestTeam(t, s, true)
	m := aijobs.NewManager(auditFailStore{s}, &fakeRunner{})
	_, err := m.SubmitJob(context.Background(), domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine})
	m.Wait()
	if err == nil {
		t.Fatal("expected audit error")
	}
	jobs, err := s.ListAIJobs(context.Background(), team.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("untracked committed jobs: %d", len(jobs))
	}
}

func TestJobWithoutCompleterAuditsTerminalOutcome(t *testing.T) {
	s := newJobTestStore(t)
	user, team := newJobTestTeam(t, s, true)
	m := aijobs.NewManager(s, &fakeRunner{})
	job, err := m.SubmitJob(context.Background(), domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine})
	if err != nil {
		t.Fatal(err)
	}
	m.Wait()
	events, err := s.ListAuditEvents(context.Background(), domain.AuditFilter{TeamID: team.ID, Action: "ai_job.complete"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].TargetID == nil || *events[0].TargetID != job.ID || events[0].ActorKind != "system" {
		t.Fatalf("terminal audit: %+v", events)
	}
}

func TestJobDispatchWaitsForOuterCommit(t *testing.T) {
	s := newJobTestStore(t)
	user, team := newJobTestTeam(t, s, true)
	m := aijobs.NewManager(s, &fakeRunner{})
	err := s.WithAuditTransaction(context.Background(), func(ctx context.Context) error {
		if _, err := m.SubmitJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: user.ID, Type: domain.AIJobTypeVoiceEngine}); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	m.Wait()
	if err == nil {
		t.Fatal("expected rollback")
	}
	jobs, err := s.ListAIJobs(context.Background(), team.ID, 100)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("rolled back jobs = %v, %v", jobs, err)
	}
}
