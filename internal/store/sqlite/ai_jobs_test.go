package sqlite_test

import (
	"context"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
)

func TestAIJobLifecycle(t *testing.T) {
	testAIJobsLifecycle(t)
}

func TestAIJobsLifecycle(t *testing.T) {
	testAIJobsLifecycle(t)
}

func testAIJobsLifecycle(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	s := newTestStore(t)
	u, team := makeAITeam(t, s)

	job, err := s.CreateAIJob(ctx, domain.AIJob{
		TeamID:       team.ID,
		AuthorUserID: u.ID,
		Type:         domain.AIJobTypeVoiceEngine,
		Status:       domain.AIJobStatusPending,
		Payload:      []byte(`{"prompt":"write a post"}`),
	})
	if err != nil {
		t.Fatalf("CreateAIJob: %v", err)
	}
	if job.Status != domain.AIJobStatusPending {
		t.Fatalf("status after create: got %q", job.Status)
	}
	if job.CompletedAt != nil {
		t.Fatal("expected CompletedAt=nil on create")
	}

	if err := s.UpdateAIJobStatus(ctx, job.ID, domain.AIJobStatusProcessing, nil, ""); err != nil {
		t.Fatalf("UpdateAIJobStatus processing: %v", err)
	}

	processing, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatalf("GetAIJobByID processing: %v", err)
	}
	if processing.Status != domain.AIJobStatusProcessing {
		t.Fatalf("status after processing: got %q", processing.Status)
	}
	if processing.CompletedAt != nil {
		t.Fatal("expected CompletedAt=nil while processing")
	}

	result := []byte(`{"post_id":"abc123"}`)
	if err := s.UpdateAIJobStatus(ctx, job.ID, domain.AIJobStatusCompleted, result, ""); err != nil {
		t.Fatalf("UpdateAIJobStatus completed: %v", err)
	}

	completed, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil {
		t.Fatalf("GetAIJobByID completed: %v", err)
	}
	if completed.Status != domain.AIJobStatusCompleted {
		t.Fatalf("status after completion: got %q", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set after completion")
	}
	if string(completed.Result) != string(result) {
		t.Fatalf("result: got %s, want %s", string(completed.Result), string(result))
	}
}
