package sqlite_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
)

func TestAuditConcurrentReadThenWriteBothCommit(t *testing.T) {
	ctx := context.Background()
	enc, err := security.NewEncrypter("sqlite-integration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.New(ctx, "file:"+filepath.Join(t.TempDir(), "concurrency.db"), enc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	u, err := s.UpsertOIDCUser(ctx, "concurrent-owner", "owner@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Concurrent"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"One", "Two"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			<-start
			errs <- s.WithAuditTransaction(ctx, func(txctx context.Context) error {
				if _, err := s.GetTeamByID(txctx, team.ID); err != nil {
					return err
				}
				// No barrier: an immediate transaction can serialize callbacks safely.
				time.Sleep(30 * time.Millisecond)
				if _, err := s.UpdateTeam(txctx, team.ID, domain.UpdateTeamInput{Name: name}); err != nil {
					return err
				}
				return s.InsertAuditEvent(txctx, domain.AuditEvent{TeamID: team.ID, ActorUserID: u.ID, ActorKind: domain.AuditActorHuman, Action: "team.update", TargetType: "team", TargetID: &team.ID})
			})
		}(name)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent audited update failed: %v", err)
		}
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("audit count %d, want 2", len(events))
	}
}

func TestAuditConcurrentReadThenWriteOverridesUnsafeDriverSettings(t *testing.T) {
	ctx := context.Background()
	enc, err := security.NewEncrypter("sqlite-integration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.New(ctx, "file:"+filepath.Join(t.TempDir(), "concurrency.db")+"?_txlock=deferred&_fk=off&_timeout=0&_pragma=busy_timeout(0)&_pragma=cache_size(-500)", enc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	u, err := s.UpsertOIDCUser(ctx, "concurrent-owner", "owner@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Concurrent"})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"One", "Two"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			<-start
			errs <- s.WithAuditTransaction(ctx, func(txctx context.Context) error {
				if _, err := s.GetTeamByID(txctx, team.ID); err != nil {
					return err
				}
				// No barrier: an immediate transaction can serialize callbacks safely.
				time.Sleep(30 * time.Millisecond)
				if _, err := s.UpdateTeam(txctx, team.ID, domain.UpdateTeamInput{Name: name}); err != nil {
					return err
				}
				return s.InsertAuditEvent(txctx, domain.AuditEvent{TeamID: team.ID, ActorUserID: u.ID, ActorKind: domain.AuditActorHuman, Action: "team.update", TargetType: "team", TargetID: &team.ID})
			})
		}(name)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent audited update failed: %v", err)
		}
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("audit count %d, want 2", len(events))
	}
}
