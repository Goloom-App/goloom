package sqlite_test

import (
	"context"
	"sync"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestClaimAIJobCompletion(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, team := makeAITeam(t, s)
	create := func() domain.AIJob {
		t.Helper()
		job, err := s.CreateAIJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: u.ID, Type: domain.AIJobTypeVoiceEngine})
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	job := create()
	start := make(chan struct{})
	var wg sync.WaitGroup
	wins := make(chan bool, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			<-start
			won, err := s.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusCompleted, []byte(`{"content":"post"}`), "")
			wins <- won
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(wins)
	close(errs)
	count := 0
	for won := range wins {
		if won {
			count++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count != 1 {
		t.Fatalf("winners=%d, want 1", count)
	}
	got, err := s.GetAIJobByIDGlobal(ctx, job.ID)
	if err != nil || got.Status != domain.AIJobStatusCompleted || got.CompletedAt == nil {
		t.Fatalf("claimed job=%+v, %v", got, err)
	}
	if err := s.UpdateAIJobStatus(ctx, job.ID, domain.AIJobStatusFailed, nil, "publishing failed"); err != nil {
		t.Fatal(err)
	}
	won, err := s.ClaimAIJobCompletion(ctx, job.ID, domain.AIJobStatusCompleted, nil, "")
	if err != nil || won {
		t.Fatalf("failed job reclaimed: %v %v", won, err)
	}
	got, err = s.GetAIJobByIDGlobal(ctx, job.ID)
	if err != nil || got.ErrorMessage != "publishing failed" || got.Status != domain.AIJobStatusFailed {
		t.Fatalf("winner failure lost: %+v %v", got, err)
	}
	for _, outcome := range []struct {
		status  domain.AIJobStatus
		message string
	}{
		{domain.AIJobStatusCompleted, ""},
		{domain.AIJobStatusFailed, "provider failed"},
		{domain.AIJobStatusFailed, "cancelled"},
		{domain.AIJobStatus("unknown"), "legacy unsupported state"},
	} {
		terminal := create()
		if err := s.UpdateAIJobStatus(ctx, terminal.ID, outcome.status, nil, outcome.message); err != nil {
			t.Fatal(err)
		}
		won, err := s.ClaimAIJobCompletion(ctx, terminal.ID, domain.AIJobStatusFailed, nil, "overwrite")
		if err != nil || won {
			t.Fatalf("terminal %s/%q reclaimed: %v %v", outcome.status, outcome.message, won, err)
		}
		got, err := s.GetAIJobByIDGlobal(ctx, terminal.ID)
		if err != nil || got.Status != outcome.status || got.ErrorMessage != outcome.message {
			t.Fatalf("terminal outcome overwritten: %+v %v", got, err)
		}
	}
	pending := create()
	if won, err := s.ClaimAIJobCompletion(ctx, pending.ID, domain.AIJobStatusProcessing, nil, ""); err == nil || won {
		t.Fatalf("invalid status accepted: %v %v", won, err)
	}
	if won, err := s.ClaimAIJobCompletion(ctx, pending.ID, domain.AIJobStatusFailed, nil, "AI failed"); err != nil || !won {
		t.Fatalf("failed claim: %v %v", won, err)
	}
	if won, err := s.ClaimAIJobCompletion(ctx, "00000000-0000-0000-0000-000000000000", domain.AIJobStatusFailed, nil, ""); err != nil || won {
		t.Fatalf("missing claimed: %v %v", won, err)
	}
}
