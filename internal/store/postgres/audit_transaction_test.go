package postgres_test

import (
	"context"
	"errors"
	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
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
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Before"})
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
		if got.Name != "Before" {
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
		if got.Name != "Before" {
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
