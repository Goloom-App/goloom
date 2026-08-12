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

// next_materialize_at is the materialization cursor, not "the next occurrence":
// once the horizon has run it sits past every round already in the calendar, so
// it names an occurrence for which no post exists yet. Anything acting on "the
// next round" — skip, shift, the card's own next label — has to resolve the
// occurrence from the linked posts instead. This test pins that gap down so the
// cursor does not get mistaken for the imminent round again; see
// SkipPostTemplateOccurrence for the behaviour that depends on it.
func TestMaterialize_cursorRunsAheadOfMaterializedRounds(t *testing.T) {
	firstOcc := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC) // Wed
	tmpl := domain.PostTemplate{
		ID:                      "tmpl1",
		TeamID:                  "team1",
		AuthorUserID:            "user1",
		Title:                   "Folge {counter}",
		Content:                 "Folge {counter}",
		RecurrenceJSON:          `{"kind":"monthly_ordinal_weekday","occurrences":[{"ordinal":1,"weekday":3},{"ordinal":3,"weekday":3}],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs:        []string{"acc1"},
		Enabled:                 true,
		NextMaterializeAt:       &firstOcc,
		MaterializeHorizonDays:  28,
		CounterNext:             385,
		AnnouncementEnabled:     true,
		AnnouncementTitle:       "Ankündigung Folge {main_counter}",
		AnnouncementContent:     "Folge #{main_counter} in 2 Tagen",
		AnnouncementDaysBefore:  2,
		AnnouncementCounterNext: 385,
	}
	h := newStatefulTemplateHarness(t, tmpl)

	// Tick 1: horizon materializes the upcoming rounds (385, 386, ...).
	h.tick(t)
	afterHorizon := h.currentTemplate()

	type materialized struct {
		occ     time.Time
		role    string
		counter int
	}
	var created []materialized
	h.st.mu.Lock()
	for _, in := range h.st.createScheduledPostCalls {
		ctr := 0
		if in.TemplateCounter != nil {
			ctr = *in.TemplateCounter
		}
		created = append(created, materialized{occ: in.TemplateOccurrenceAt.UTC(), role: in.TemplatePostRole, counter: ctr})
	}
	h.st.mu.Unlock()
	if len(created) == 0 {
		t.Fatalf("expected horizon to materialize posts, got none")
	}
	t.Logf("horizon materialized %d posts, next_materialize_at=%s counter_next=%d",
		len(created), afterHorizon.NextMaterializeAt.Format(time.RFC3339), afterHorizon.CounterNext)
	for _, c := range created {
		t.Logf("  occ=%s role=%s counter=%d", c.occ.Format(time.RFC3339), c.role, c.counter)
	}

	// What the UI sends when the user clicks "Skip next".
	skipTarget := afterHorizon.NextMaterializeAt.UTC()

	// The imminent occurrence (folge 385) is materialized and NOT what gets skipped.
	if skipTarget.Equal(firstOcc) {
		t.Fatalf("precondition failed: next_materialize_at still points at the imminent occurrence")
	}
	imminentStillThere := false
	for _, c := range created {
		if c.occ.Equal(firstOcc) && c.role == domain.TemplatePostRoleMain && c.counter == 385 {
			imminentStillThere = true
		}
	}
	if !imminentStillThere {
		t.Fatalf("expected folge 385 materialized for %s", firstOcc.Format(time.RFC3339))
	}

	// Now the skip lands and the scheduler ticks again.
	h.st.mu.Lock()
	h.st.isPostTemplateOccurrenceSkippedFn = func(templateID string, occurrenceAt time.Time) (bool, error) {
		return occurrenceAt.UTC().Equal(skipTarget), nil
	}
	h.st.mu.Unlock()
	h.resetCalls()
	h.tick(t)
	afterSkip := h.currentTemplate()

	// Existing horizon posts are untouched: nothing deleted, nothing renumbered.
	if got := h.newPosts(); got != 0 {
		t.Logf("skip tick created %d new posts", got)
	}
	if afterSkip.CounterNext != afterHorizon.CounterNext {
		t.Errorf("counter moved on skip: %d -> %d", afterHorizon.CounterNext, afterSkip.CounterNext)
	}
	t.Logf("after skip: next_materialize_at=%s counter_next=%d",
		afterSkip.NextMaterializeAt.Format(time.RFC3339), afterSkip.CounterNext)
}
