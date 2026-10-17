package postgres_test

import (
	"context"
	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestAuditOutboxSurvivesDeliveryFailure(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db, err := pgx.Connect(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.UpsertOIDCUser(ctx, uuid.NewString(), "outbox@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Before-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	e := domain.AuditEvent{ID: uuid.NewString(), TeamID: team.ID, ActorUserID: u.ID, ActorKind: domain.AuditActorHuman, Action: "post.publish", TargetType: "post"}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), "drop trigger if exists fail_audit on audit_events; drop function if exists fail_audit_insert()")
		_ = db.Close(context.Background())
	})
	if _, err := db.Exec(ctx, `create function fail_audit_insert() returns trigger language plpgsql as $$ begin raise exception 'injected audit failure'; end $$; create trigger fail_audit before insert on audit_events for each row execute function fail_audit_insert()`); err != nil {
		t.Fatal(err)
	}
	err = s.WithAuditTransaction(ctx, func(txctx context.Context) error {
		if _, err := s.UpdateTeam(txctx, team.ID, domain.UpdateTeamInput{Name: "Observed-" + uuid.NewString()}); err != nil {
			return err
		}
		return s.EnqueueAuditEvent(txctx, e)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DrainAuditEvents(ctx); err == nil {
		t.Fatal("expected delivery failure")
	}
	var pending int
	if err := db.QueryRow(ctx, "select count(*) from audit_pending").Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("pending %d: %v", pending, err)
	}
	if _, err := db.Exec(ctx, "drop trigger fail_audit on audit_events; drop function fail_audit_insert()"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.DrainAuditEvents(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.EnqueueAuditEvent(ctx, e); err != nil {
		t.Fatal(err)
	}
	if err := s.DrainAuditEvents(ctx); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListAuditEvents(ctx, domain.AuditFilter{TeamID: team.ID})
	if err != nil || len(events) != 1 || events[0].ID != e.ID {
		t.Fatalf("events %v: %v", events, err)
	}
	if err := db.QueryRow(ctx, "select count(*) from audit_pending").Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("pending %d: %v", pending, err)
	}
}

func TestRepairFuturePostedPostsRollsBackAuditFailure(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	db, err := pgx.Connect(ctx, postgresDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	u, err := s.UpsertOIDCUser(ctx, uuid.NewString(), "repair-failure@example.test", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "Repair-failure-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	post, err := s.CreateScheduledPost(ctx, team.ID, domain.AuthenticatedPrincipal{User: u}, domain.CreatePostInput{Content: "secret", ScheduledAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusCancelled, "", nil) })
	if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `create function fail_repair_insert() returns trigger language plpgsql as $$ begin raise exception 'audit unavailable'; end $$; create trigger fail_repair before insert on audit_events for each row execute function fail_repair_insert()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := pgx.Connect(context.Background(), postgresDSN(t))
		if err == nil {
			defer cleanup.Close(context.Background())
			_, _ = cleanup.Exec(context.Background(), "drop trigger if exists fail_repair on audit_events; drop function if exists fail_repair_insert()")
		}
	})
	if n, err := s.RepairFuturePostedPosts(ctx); err == nil || n != 0 {
		t.Fatalf("untracked repair %d: %v", n, err)
	}
	got, err := s.GetScheduledPostByID(ctx, post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.PostStatusPosted {
		t.Fatalf("repair survived audit failure: %s", got.Status)
	}
}
