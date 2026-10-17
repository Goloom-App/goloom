package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"git.f4mily.net/goloom/api"
	"git.f4mily.net/goloom/internal/config"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/store"
)

type callbackAuditFailure struct{ store.Store }

func (s callbackAuditFailure) InsertAuditEvent(context.Context, domain.AuditEvent) error {
	return errors.New("audit unavailable")
}

func TestAutomationCallbackAuditReplay(t *testing.T) {
	ctx := context.Background()
	f, acc, tmpl := newRecurringCallbackFixture(t)
	job := makeRecurringJob(t, f, recurringJobPayload(tmpl.ID, acc.ID, "private fallback", time.Now().Add(time.Hour)))
	for range 2 {
		f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusFailed, nil, "https://credential:secret@example.test/?token=secret")
	}
	events, err := f.store.ListAuditEvents(ctx, domain.AuditFilter{TeamID: f.team.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want one fallback and one post", len(events))
	}
	actions := map[string]bool{}
	for _, e := range events {
		actions[e.Action] = true
		if e.ActorKind != "system" || e.ActorUserID != "" || e.TokenID != nil || e.Metadata["job_id"] != job.ID || e.Metadata["initiator_user_id"] != f.user.ID {
			t.Fatalf("wrong automation identity/correlation: %+v", e)
		}
		raw, _ := json.Marshal(e)
		if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "private fallback") {
			t.Fatalf("secret in audit: %s", raw)
		}
	}
	if !actions["ai_job.fallback"] || !actions["post.automation_create"] {
		t.Fatalf("actions: %v", actions)
	}
}

func TestAutomationCallbackAuditFailureRollsBack(t *testing.T) {
	ctx := context.Background()
	f, acc, tmpl := newRecurringCallbackFixture(t)
	job := makeRecurringJob(t, f, recurringJobPayload(tmpl.ID, acc.ID, "fallback", time.Now().Add(time.Hour)))
	a := api.New(nil, callbackAuditFailure{f.store}, nil, nil, config.Config{}, nil, nil, nil, nil)
	a.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"hello"}`), "")
	posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 0 {
		t.Fatalf("untracked committed posts: %d", len(posts))
	}
	updated, err := f.store.GetAIJobByIDGlobal(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.AIJobStatusPending {
		t.Fatalf("job terminal despite rollback: %s", updated.Status)
	}
}

func TestAutomationCallbackConcurrentReplay(t *testing.T) {
	ctx := context.Background()
	f, acc, tmpl := newRecurringCallbackFixture(t)
	job := makeRecurringJob(t, f, recurringJobPayload(tmpl.ID, acc.ID, "fallback", time.Now().Add(time.Hour)))
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.api.CompleteAIJob(ctx, job.ID, domain.AIJobStatusCompleted, json.RawMessage(`{"content":"hello"}`), "")
		}()
	}
	wg.Wait()
	posts, err := f.store.ListTeamPosts(ctx, f.team.ID)
	if err != nil || len(posts) != 1 {
		t.Fatalf("replay posts = %v, %v", posts, err)
	}
	events, err := f.store.ListAuditEvents(ctx, domain.AuditFilter{TeamID: f.team.ID})
	if err != nil || len(events) != 2 {
		t.Fatalf("replay events = %v, %v", events, err)
	}
}
