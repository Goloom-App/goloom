package scheduler

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
)

func TestService_RegeneratePostTemplateOccurrence(t *testing.T) {
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	tmpl := domain.PostTemplate{
		ID:                "tpl1",
		TeamID:            "team1",
		AuthorUserID:      "user1",
		Title:             "main #{counter}",
		Content:           "body {counter}",
		RecurrenceJSON:    `{"kind":"weekly","weekdays":[3],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs:  []string{"acc1"},
		Enabled:           true,
		NextMaterializeAt: ptrTime(now.Add(7 * 24 * time.Hour)),
		CounterNext:       8,
	}
	mainCtr := 7
	linked := []domain.PostTemplateLinkedPost{
		{
			ID:                   "post-main",
			Status:               domain.PostStatusPending,
			TemplateOccurrenceAt: now,
			TemplatePostRole:     domain.TemplatePostRoleMain,
			TemplateCounter:      &mainCtr,
		},
	}

	st := &mockStore{
		getPostTemplateFn: func(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error) {
			return tmpl, nil
		},
		listPostTemplateLinkedPosts: linked,
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)

	result, err := svc.RegeneratePostTemplateOccurrence(context.Background(), "team1", "tpl1", now)
	if err != nil {
		t.Fatalf("RegeneratePostTemplateOccurrence: %v", err)
	}
	if result.DeletedPosts != 1 || result.RegeneratedOccurrences != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.createScheduledPostCalls) != 1 {
		t.Fatalf("expected 1 recreated post, got %d", len(st.createScheduledPostCalls))
	}
	if st.createScheduledPostCalls[0].TemplateCounter == nil || *st.createScheduledPostCalls[0].TemplateCounter != 7 {
		t.Fatalf("expected counter 7, got %v", st.createScheduledPostCalls[0].TemplateCounter)
	}
}

func TestService_RegeneratePostTemplateOccurrence_blockedPosted(t *testing.T) {
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	mainCtr := 7
	st := &mockStore{
		getPostTemplateFn: func(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error) {
			return domain.PostTemplate{ID: "tpl1", TeamID: "team1"}, nil
		},
		listPostTemplateLinkedPosts: []domain.PostTemplateLinkedPost{{
			ID:                   "post-main",
			Status:               domain.PostStatusPosted,
			TemplateOccurrenceAt: now,
			TemplatePostRole:     domain.TemplatePostRoleMain,
			TemplateCounter:      &mainCtr,
		}},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	_, err := svc.RegeneratePostTemplateOccurrence(context.Background(), "team1", "tpl1", now)
	if err != ErrRegenerateBlocked {
		t.Fatalf("expected ErrRegenerateBlocked, got %v", err)
	}
}

func TestRegeneratePostTemplateHorizonIgnoresPastPostedPosts(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-24 * time.Hour).Truncate(time.Second)
	future := now.Add(24 * time.Hour).Truncate(time.Second)
	pastCtr, futureCtr := 3, 4
	tmpl := domain.PostTemplate{
		ID:                     "tpl-h",
		TeamID:                 "team1",
		AuthorUserID:           "user1",
		Title:                  "main #{counter}",
		Content:                "body {counter}",
		RecurrenceJSON:         `{"kind":"weekly","weekdays":[3],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs:       []string{"acc1"},
		Enabled:                true,
		MaterializeHorizonDays: 7,
		NextMaterializeAt:      ptrTime(future.Add(7 * 24 * time.Hour)),
		CounterNext:            5,
	}
	st := &mockStore{
		getPostTemplateFn: func(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error) {
			return tmpl, nil
		},
		listPostTemplateLinkedPosts: []domain.PostTemplateLinkedPost{
			{ID: "old", Status: domain.PostStatusPosted, TemplateOccurrenceAt: past, TemplatePostRole: domain.TemplatePostRoleMain, TemplateCounter: &pastCtr},
			{ID: "new", Status: domain.PostStatusPending, TemplateOccurrenceAt: future, TemplatePostRole: domain.TemplatePostRoleMain, TemplateCounter: &futureCtr},
		},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)

	result, err := svc.RegeneratePostTemplateHorizon(context.Background(), "team1", "tpl-h")
	if err != nil {
		t.Fatalf("RegeneratePostTemplateHorizon: %v", err)
	}
	if result.DeletedPosts != 1 || result.RegeneratedOccurrences != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}
