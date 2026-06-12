package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/aijobs"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

type noopRunner struct{}

func (noopRunner) RunJob(context.Context, domain.AIJob, domain.AIServiceConfig, domain.AIContext) (json.RawMessage, error) {
	return json.RawMessage(`{"content":"ai text"}`), nil
}

func newRecurringAIFixture(t *testing.T, aiEnabled bool) (*sqlite.Store, *aijobs.Manager, domain.Team, domain.User) {
	t.Helper()
	ctx := context.Background()
	enc, err := security.NewEncrypter("scheduler-test-secret-32-bytes!!!")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.New(ctx, "file:"+uuid.NewString()+"?mode=memory&cache=shared", enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	user, err := s.UpsertOIDCUser(ctx, "sched-"+uuid.NewString(), "sched@test", "Sched")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "sched-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if aiEnabled {
		enabled := true
		if _, err := s.UpdateTeam(ctx, team.ID, domain.UpdateTeamInput{Name: team.Name, IsAIEnabled: &enabled}); err != nil {
			t.Fatal(err)
		}
		team.IsAIEnabled = true
		if _, err := s.UpsertAIServiceConfig(ctx, team.ID, domain.AIServiceConfig{
			Provider: "openai", Model: "gpt-test", APIKey: "sk-test",
		}); err != nil {
			t.Fatal(err)
		}
	}
	return s, aijobs.NewManager(s, noopRunner{}), team, user
}

func recurringTestTemplate(team domain.Team, user domain.User) domain.PostTemplate {
	return domain.PostTemplate{
		ID:               "tpl-" + uuid.NewString(),
		TeamID:           team.ID,
		AuthorUserID:     user.ID,
		Title:            "Stammtisch #{counter}",
		Content:          "Stammtisch Nr. {counter}",
		TargetAccountIDs: []string{"acc-1"},
		Enabled:          true,
		AiEnhanceEnabled: true,
		PromptHint:       "locker bleiben",
	}
}

func TestSubmitRecurringAIEnhancementQueuesJobWithMeta(t *testing.T) {
	ctx := context.Background()
	s, manager, team, user := newRecurringAIFixture(t, true)
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)

	tmpl := recurringTestTemplate(team, user)
	scheduledAt := time.Date(2027, 3, 1, 18, 0, 0, 0, time.UTC)
	if err := svc.submitRecurringAIEnhancement(ctx, tmpl, "Stammtisch Nr. {counter}", "Stammtisch #4", scheduledAt, false, scheduledAt); err != nil {
		t.Fatalf("submitRecurringAIEnhancement: %v", err)
	}
	manager.Wait()

	jobs, err := s.ListAIJobs(ctx, team.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 queued AI job, got %d", len(jobs))
	}
	var payload struct {
		Params struct {
			SourceContent       string   `json:"source_content"`
			PromptHint          string   `json:"prompt_hint"`
			Schedule            bool     `json:"schedule"`
			RecurringPostKind   string   `json:"recurring_post_kind"`
			TargetAccountIDs    []string `json:"target_account_ids"`
			RecurringAutomation struct {
				TemplateID      string `json:"template_id"`
				FallbackContent string `json:"fallback_content"`
				PostKind        string `json:"post_kind"`
				TemplateCounter int    `json:"template_counter"`
			} `json:"recurring_automation"`
		} `json:"params"`
	}
	if err := json.Unmarshal(jobs[0].Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	p := payload.Params
	if p.SourceContent != "Stammtisch Nr. {counter}" || p.PromptHint != "locker bleiben" {
		t.Fatalf("source/hint: %+v", p)
	}
	if p.Schedule {
		t.Fatal("recurring jobs must not self-schedule")
	}
	if p.RecurringPostKind != recurringPostKindMain || p.RecurringAutomation.PostKind != recurringPostKindMain {
		t.Fatalf("post kind: %+v", p)
	}
	if p.RecurringAutomation.TemplateID != tmpl.ID {
		t.Fatalf("template id: %q", p.RecurringAutomation.TemplateID)
	}
	if p.RecurringAutomation.FallbackContent != "Stammtisch Nr. {counter}" {
		t.Fatalf("fallback content: %q", p.RecurringAutomation.FallbackContent)
	}
	if len(p.TargetAccountIDs) != 1 || p.TargetAccountIDs[0] != "acc-1" {
		t.Fatalf("targets: %v", p.TargetAccountIDs)
	}
}

func TestSubmitRecurringAIEnhancementWithoutAIConfigFails(t *testing.T) {
	ctx := context.Background()
	s, manager, team, user := newRecurringAIFixture(t, false)
	svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)

	tmpl := recurringTestTemplate(team, user)
	at := time.Now().UTC()
	if err := svc.submitRecurringAIEnhancement(ctx, tmpl, "x", "t", at, false, at); err == nil {
		t.Fatal("missing AI config must surface as error so the caller can fall back to the template")
	}
}

func TestShouldEnhanceRecurringWithAI(t *testing.T) {
	ctx := context.Background()

	t.Run("AllGatesOpen", func(t *testing.T) {
		s, manager, team, user := newRecurringAIFixture(t, true)
		svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
		if !svc.shouldEnhanceRecurringWithAI(ctx, recurringTestTemplate(team, user)) {
			t.Fatal("expected AI enhancement to be active")
		}
	})

	t.Run("TeamAIDisabled", func(t *testing.T) {
		s, manager, team, user := newRecurringAIFixture(t, false)
		svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
		if svc.shouldEnhanceRecurringWithAI(ctx, recurringTestTemplate(team, user)) {
			t.Fatal("team without AI must not enhance")
		}
	})

	t.Run("TemplateFlagOff", func(t *testing.T) {
		s, manager, team, user := newRecurringAIFixture(t, true)
		svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, manager)
		tmpl := recurringTestTemplate(team, user)
		tmpl.AiEnhanceEnabled = false
		if svc.shouldEnhanceRecurringWithAI(ctx, tmpl) {
			t.Fatal("template without ai_enhance must not enhance")
		}
	})

	t.Run("NoJobManager", func(t *testing.T) {
		s, _, team, user := newRecurringAIFixture(t, true)
		svc := New(testLogger(), s, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
		if svc.shouldEnhanceRecurringWithAI(ctx, recurringTestTemplate(team, user)) {
			t.Fatal("nil job manager must not enhance")
		}
	})
}

func TestExpandedAnnouncementReference(t *testing.T) {
	mainEventAt := time.Date(2027, 3, 10, 18, 0, 0, 0, time.UTC)
	tmpl := domain.PostTemplate{
		AnnouncementEnabled:     true,
		AnnouncementContent:     "Ankündigung Nr. {counter}",
		AnnouncementTitle:       "Ankündigung #{counter}",
		AnnouncementDaysBefore:  2,
		AnnouncementCounterNext: 5,
		CounterNext:             5,
	}
	content, title := expandedAnnouncementReference(tmpl, mainEventAt)
	if content != "Ankündigung Nr. 5" {
		t.Fatalf("content = %q", content)
	}
	if title != "Ankündigung #5" {
		t.Fatalf("title = %q", title)
	}

	disabled := domain.PostTemplate{AnnouncementEnabled: false, AnnouncementContent: "x"}
	if c, ti := expandedAnnouncementReference(disabled, mainEventAt); c != "" || ti != "" {
		t.Fatalf("disabled announcement must be empty, got %q %q", c, ti)
	}
}

func TestRegeneratePostTemplateHorizon(t *testing.T) {
	now := time.Now().UTC()
	occ1 := now.Add(24 * time.Hour).Truncate(time.Second)
	occ2 := now.Add(8 * 24 * time.Hour).Truncate(time.Second)
	ctr1, ctr2 := 4, 5
	tmpl := domain.PostTemplate{
		ID:                "tpl-h",
		TeamID:            "team1",
		AuthorUserID:      "user1",
		Title:             "main #{counter}",
		Content:           "body {counter}",
		RecurrenceJSON:    `{"kind":"weekly","weekdays":[3],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs:  []string{"acc1"},
		Enabled:           true,
		NextMaterializeAt: ptrTime(now.Add(15 * 24 * time.Hour)),
		CounterNext:       6,
	}
	linked := []domain.PostTemplateLinkedPost{
		{ID: "p1", Status: domain.PostStatusPending, TemplateOccurrenceAt: occ1, TemplatePostRole: domain.TemplatePostRoleMain, TemplateCounter: &ctr1},
		{ID: "p2", Status: domain.PostStatusDraft, TemplateOccurrenceAt: occ2, TemplatePostRole: domain.TemplatePostRoleMain, TemplateCounter: &ctr2},
	}
	st := &mockStore{
		getPostTemplateFn: func(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error) {
			return tmpl, nil
		},
		listPostTemplateLinkedPosts: linked,
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)

	result, err := svc.RegeneratePostTemplateHorizon(context.Background(), "team1", "tpl-h")
	if err != nil {
		t.Fatalf("RegeneratePostTemplateHorizon: %v", err)
	}
	if result.DeletedPosts != 2 || result.RegeneratedOccurrences != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRegeneratePostTemplateHorizonBlockedByPostedPost(t *testing.T) {
	now := time.Now().UTC()
	ctr := 3
	st := &mockStore{
		getPostTemplateFn: func(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error) {
			return domain.PostTemplate{ID: "tpl-b", TeamID: "team1", Enabled: true}, nil
		},
		listPostTemplateLinkedPosts: []domain.PostTemplateLinkedPost{
			{ID: "p1", Status: domain.PostStatusPosted, TemplateOccurrenceAt: now.Add(24 * time.Hour), TemplatePostRole: domain.TemplatePostRoleMain, TemplateCounter: &ctr},
		},
	}
	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)

	if _, err := svc.RegeneratePostTemplateHorizon(context.Background(), "team1", "tpl-b"); err == nil {
		t.Fatal("posted posts in scope must block horizon regenerate")
	}
}
