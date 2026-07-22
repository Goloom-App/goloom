package scheduler

import (
	"context"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
)

type roleKey struct {
	occ  int64
	role string
}

// statefulTemplateHarness drives materializePostTemplates across ticks while
// tracking materialized (occurrence, role) pairs and the live template
// counters, so horizon loops and re-walks behave like production.
type statefulTemplateHarness struct {
	st        *mockStore
	svc       *Service
	live      domain.PostTemplate
	seenRoles map[roleKey]bool
}

func newStatefulTemplateHarness(t *testing.T, base domain.PostTemplate) *statefulTemplateHarness {
	t.Helper()
	h := &statefulTemplateHarness{live: base, seenRoles: map[roleKey]bool{}}
	st := &mockStore{}
	h.st = st

	st.getPostTemplateFn = func(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error) {
		return h.currentTemplate(), nil
	}
	// mockStore.CreateScheduledPost already holds st.mu and appended the call,
	// so this hook must not lock or append again.
	st.createScheduledPostFn = func(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
		if input.TemplateOccurrenceAt != nil {
			h.seenRoles[roleKey{input.TemplateOccurrenceAt.UTC().UnixNano(), input.TemplatePostRole}] = true
		}
		return domain.ScheduledPost{ID: "p"}, nil
	}
	st.hasRoleFn = func(occ time.Time, role string) bool {
		return h.seenRoles[roleKey{occ.UTC().UnixNano(), role}]
	}
	h.svc = New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	return h
}

func (h *statefulTemplateHarness) currentTemplate() domain.PostTemplate {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	return h.currentTemplateLocked()
}

func (h *statefulTemplateHarness) currentTemplateLocked() domain.PostTemplate {
	cur := h.live
	if n := len(h.st.advancePostTemplateCalls); n > 0 {
		last := h.st.advancePostTemplateCalls[n-1]
		cur.CounterNext = last.counterNext
		cur.NextMaterializeAt = last.nextMaterialize
	}
	if n := len(h.st.advanceAnnouncementCounterCalls); n > 0 {
		cur.AnnouncementCounterNext = h.st.advanceAnnouncementCounterCalls[n-1].counterNext
	}
	return cur
}

func (h *statefulTemplateHarness) tick(t *testing.T) {
	t.Helper()
	h.st.mu.Lock()
	h.st.listDuePostTemplates = []domain.PostTemplate{h.currentTemplateLocked()}
	h.st.mu.Unlock()
	if err := h.svc.materializePostTemplates(context.Background()); err != nil {
		t.Fatalf("materializePostTemplates: %v", err)
	}
	h.live = h.currentTemplate()
}

func (h *statefulTemplateHarness) resetCalls() {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	h.st.advancePostTemplateCalls = nil
	h.st.advanceAnnouncementCounterCalls = nil
	h.st.createScheduledPostCalls = nil
}

func (h *statefulTemplateHarness) newPosts() int {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	return len(h.st.createScheduledPostCalls)
}

// Regression: re-walking already-materialized occurrences (e.g. after a
// template edit resets next_materialize_at backward across horizon-materialized
// posts) must NOT advance the edition counter, and must not drift the main and
// announcement counters apart. Previously the counter was bumped once per
// re-walked occurrence with no new post created.
func TestMaterialize_reWalkDoesNotAdvanceCounter(t *testing.T) {
	firstOcc := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC) // Wed
	tmpl := domain.PostTemplate{
		ID:                      "tmpl1",
		TeamID:                  "team1",
		AuthorUserID:            "user1",
		Title:                   "Edition {counter}",
		Content:                 "Edition {counter}",
		RecurrenceJSON:          `{"kind":"weekly","weekdays":[3],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs:        []string{"acc1"},
		Enabled:                 true,
		NextMaterializeAt:       &firstOcc,
		MaterializeHorizonDays:  28,
		CounterNext:             384,
		AnnouncementEnabled:     true,
		AnnouncementTitle:       "Reminder {main_counter}",
		AnnouncementContent:     "Reminder: edition #{main_counter} in 2 days",
		AnnouncementDaysBefore:  2,
		AnnouncementCounterNext: 384,
	}
	h := newStatefulTemplateHarness(t, tmpl)

	// Tick 1: horizon pre-materializes several rounds ahead.
	h.tick(t)
	afterHorizon := h.currentTemplate()
	if afterHorizon.CounterNext == tmpl.CounterNext {
		t.Fatalf("expected horizon to materialize rounds, counter unchanged at %d", afterHorizon.CounterNext)
	}
	if afterHorizon.CounterNext != afterHorizon.AnnouncementCounterNext {
		t.Fatalf("main and announcement counters should stay in lockstep, got main=%d ann=%d",
			afterHorizon.CounterNext, afterHorizon.AnnouncementCounterNext)
	}

	// Simulate a template edit that changes the recurrence: the store recomputes
	// next_materialize_at to the next occurrence after now, which is *earlier*
	// than where the horizon advanced it — without deleting the already
	// materialized posts.
	resetOcc := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	h.live.NextMaterializeAt = &resetOcc
	h.resetCalls()

	// Tick 2: re-walk of already-materialized occurrences.
	h.tick(t)
	afterEdit := h.currentTemplate()

	if got := h.newPosts(); got != 0 {
		t.Errorf("re-walk should create no new posts, got %d", got)
	}
	if afterEdit.CounterNext != afterHorizon.CounterNext {
		t.Errorf("edition counter jumped on re-walk: %d -> %d (want unchanged)",
			afterHorizon.CounterNext, afterEdit.CounterNext)
	}
	if afterEdit.CounterNext != afterEdit.AnnouncementCounterNext {
		t.Errorf("main/announcement counters drifted: main=%d ann=%d",
			afterEdit.CounterNext, afterEdit.AnnouncementCounterNext)
	}
}
