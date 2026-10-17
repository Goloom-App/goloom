package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store"
)

type auditInsertFailureStore struct {
	store.Store
	cancel context.CancelFunc
}

func (s auditInsertFailureStore) InsertAuditEvent(ctx context.Context, e domain.AuditEvent) error {
	if s.cancel != nil {
		s.cancel()
		return s.Store.InsertAuditEvent(ctx, e)
	}
	return errors.New("injected audit insert failure")
}

func TestAuditedMutationRollback(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "insert_failure", true: "request_cancelled"}[cancelled], func(t *testing.T) {
			s := newValidationMemoryStore(t)
			a := newTestAPI(t, s)
			ctx := context.Background()
			u, err := s.UpsertOIDCUser(ctx, "audit-rollback", "audit@test.test", "Actor")
			if err != nil {
				t.Fatal(err)
			}
			team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Before"})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			failed := auditInsertFailureStore{Store: s}
			if cancelled {
				failed.cancel = cancel
			}
			a.store = failed
			req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString(`{"name":"After"}`))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("teamID", team.ID)
			req = req.WithContext(security.WithPrincipal(ctx, domain.AuthenticatedPrincipal{User: u, Kind: domain.AuditActorHuman}))
			w := httptest.NewRecorder()
			a.auditedHandler(a.handleUpdateTeam)(w, req)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			got, err := s.GetTeamByID(context.Background(), team.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != "Before" {
				t.Fatalf("untracked change persisted: %+v", got)
			}
			count, err := s.CountAuditEvents(context.Background(), domain.AuditFilter{TeamID: team.ID})
			if err != nil || count != 0 {
				t.Fatalf("count=%d err=%v", count, err)
			}
		})
	}
}

type staleAIJobStore struct {
	store.Store
	stale domain.AIJob
	first bool
}

func (s *staleAIJobStore) GetAIJobByID(ctx context.Context, teamID, jobID string) (domain.AIJob, error) {
	if !s.first {
		s.first = true
		return s.stale, nil
	}
	return s.Store.GetAIJobByID(ctx, teamID, jobID)
}
func TestCancelAIJobDoesNotOverwriteConcurrentCompletion(t *testing.T) {
	s := newValidationMemoryStore(t)
	a := newTestAPI(t, s)
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "cancel-race", "actor@test.test", "Actor")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Race"})
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.CreateAIJob(ctx, domain.AIJob{TeamID: team.ID, AuthorUserID: u.ID, Type: domain.AIJobTypeVoiceEngine, Status: domain.AIJobStatusPending, Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAIJobStatus(ctx, job.ID, domain.AIJobStatusCompleted, []byte(`{}`), ""); err != nil {
		t.Fatal(err)
	}
	a.store = &staleAIJobStore{Store: s, stale: job}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.SetPathValue("teamID", team.ID)
	req.SetPathValue("jobID", job.ID)
	req = req.WithContext(security.WithPrincipal(ctx, domain.AuthenticatedPrincipal{User: u, Kind: domain.AuditActorHuman}))
	w := httptest.NewRecorder()
	a.auditedHandler(a.handleCancelAIJob)(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("concurrent completion cancelled: %d %s", w.Code, w.Body.String())
	}
	current, err := s.GetAIJobByID(ctx, team.ID, job.ID)
	if err != nil || current.Status != domain.AIJobStatusCompleted {
		t.Fatalf("completion overwritten: %+v %v", current, err)
	}
	n, _ := s.CountAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID, Action: "ai_job.cancel"})
	if n != 0 {
		t.Fatal("invented cancel event")
	}
}
