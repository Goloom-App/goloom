package scheduler

import (
	"context"
	"sort"
	"strconv"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/scheduling"
)

// skipHarness drives SkipPostTemplateOccurrence against a store that actually
// behaves like the real one: linked posts disappear when deleted, the
// materialization cursor and counters follow the rewind, and re-materialized
// rounds show up as new linked posts. Without that statefulness the re-walk
// after a skip is untestable.
type skipHarness struct {
	st   *mockStore
	svc  *Service
	live domain.PostTemplate

	skips  map[int64]bool
	shifts map[int64]time.Time
	// nextPostID numbers recreated posts so deletions can be tracked by ID.
	nextPostID int
}

func newSkipHarness(t *testing.T, base domain.PostTemplate, linked []domain.PostTemplateLinkedPost) *skipHarness {
	t.Helper()
	h := &skipHarness{live: base, skips: map[int64]bool{}, shifts: map[int64]time.Time{}}
	st := &mockStore{listPostTemplateLinkedPosts: linked}
	h.st = st

	st.getPostTemplateFn = func(ctx context.Context, teamID, templateID string) (domain.PostTemplate, error) {
		return h.currentTemplate(), nil
	}
	// CreateScheduledPost already holds st.mu, so this hook must not lock.
	st.createScheduledPostFn = func(ctx context.Context, teamID string, principal domain.AuthenticatedPrincipal, input domain.CreatePostInput) (domain.ScheduledPost, error) {
		h.nextPostID++
		id := "new-" + strconv.Itoa(h.nextPostID)
		ctr := 0
		if input.TemplateCounter != nil {
			ctr = *input.TemplateCounter
		}
		st.listPostTemplateLinkedPosts = append(st.listPostTemplateLinkedPosts, domain.PostTemplateLinkedPost{
			ID:                   id,
			Status:               domain.PostStatusPending,
			TemplateOccurrenceAt: input.TemplateOccurrenceAt.UTC(),
			TemplatePostRole:     input.TemplatePostRole,
			TemplateCounter:      &ctr,
		})
		return domain.ScheduledPost{ID: id}, nil
	}
	st.deleteLinkedPostsFn = func(postIDs []string) (int, error) {
		gone := map[string]bool{}
		for _, id := range postIDs {
			gone[id] = true
		}
		kept := st.listPostTemplateLinkedPosts[:0]
		for _, p := range st.listPostTemplateLinkedPosts {
			if !gone[p.ID] {
				kept = append(kept, p)
			}
		}
		st.listPostTemplateLinkedPosts = kept
		return len(postIDs), nil
	}
	st.hasRoleFn = func(occ time.Time, role string) bool {
		for _, p := range st.listPostTemplateLinkedPosts {
			if p.TemplateOccurrenceAt.UTC().Equal(occ.UTC()) && p.TemplatePostRole == role {
				return true
			}
		}
		return false
	}
	st.isPostTemplateOccurrenceSkippedFn = func(templateID string, occurrenceAt time.Time) (bool, error) {
		key := occurrenceAt.UTC().UnixNano()
		_, shifted := h.shifts[key]
		return h.skips[key] || shifted, nil
	}
	st.addSkipFn = func(occurrenceAt time.Time) {
		h.skips[occurrenceAt.UnixNano()] = true
	}
	st.shiftOccurrenceFn = func(occurrenceAt, shiftTo time.Time) {
		h.shifts[occurrenceAt.UnixNano()] = shiftTo
	}
	st.getPostTemplateShiftToFn = func(templateID string, occurrenceAt time.Time) *time.Time {
		if to, ok := h.shifts[occurrenceAt.UTC().UnixNano()]; ok {
			return &to
		}
		return nil
	}
	st.setMaterializationStateFn = func(next *time.Time, counterNext, annCounterNext int) {
		h.live.NextMaterializeAt = next
		h.live.CounterNext = counterNext
		h.live.AnnouncementCounterNext = annCounterNext
	}

	h.svc = New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	return h
}

func (h *skipHarness) currentTemplate() domain.PostTemplate {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
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

// rounds returns the surviving main posts as occurrence->counter, which is the
// thing a user actually reads off the calendar.
func (h *skipHarness) rounds() []roundView {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	var out []roundView
	for _, p := range h.st.listPostTemplateLinkedPosts {
		if p.TemplatePostRole != domain.TemplatePostRoleMain {
			continue
		}
		ctr := 0
		if p.TemplateCounter != nil {
			ctr = *p.TemplateCounter
		}
		out = append(out, roundView{occ: p.TemplateOccurrenceAt.UTC(), counter: ctr})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].occ.Before(out[j].occ) })
	return out
}

type roundView struct {
	occ     time.Time
	counter int
}

func skipTestTemplate(firstOcc time.Time) domain.PostTemplate {
	return domain.PostTemplate{
		ID:                      "tmpl1",
		TeamID:                  "team1",
		AuthorUserID:            "user1",
		Title:                   "Folge {counter}",
		Content:                 "Folge {counter}",
		RecurrenceJSON:          `{"kind":"monthly_ordinal_weekday","occurrences":[{"ordinal":1,"weekday":3},{"ordinal":3,"weekday":3}],"hour":10,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs:        []string{"acc1"},
		Enabled:                 true,
		MaterializeHorizonDays:  45,
		AnnouncementEnabled:     true,
		AnnouncementTitle:       "Ankündigung Folge {main_counter}",
		AnnouncementContent:     "Folge #{main_counter} in 2 Tagen",
		AnnouncementDaysBefore:  2,
		NextMaterializeAt:       &firstOcc,
		CounterNext:             385,
		AnnouncementCounterNext: 385,
	}
}

// ruleOccurrences returns the next n occurrences the template's own recurrence
// rule produces, so tests line up with what the materializer will actually walk
// instead of hand-picked dates.
func ruleOccurrences(t *testing.T, tmpl domain.PostTemplate, n int) []time.Time {
	t.Helper()
	rule, err := scheduling.ParseRecurrenceJSON(tmpl.RecurrenceJSON)
	if err != nil {
		t.Fatalf("ParseRecurrenceJSON: %v", err)
	}
	out := make([]time.Time, 0, n)
	cur := time.Now().UTC()
	for i := 0; i < n; i++ {
		next, err := scheduling.NextOccurrence(rule, cur)
		if err != nil {
			t.Fatalf("NextOccurrence: %v", err)
		}
		out = append(out, next.UTC())
		cur = next
	}
	return out
}

func linkedRound(idPrefix string, occ time.Time, counter int) []domain.PostTemplateLinkedPost {
	c := counter
	return []domain.PostTemplateLinkedPost{
		{ID: idPrefix + "-main", Status: domain.PostStatusPending, TemplateOccurrenceAt: occ, TemplatePostRole: domain.TemplatePostRoleMain, TemplateCounter: &c},
		{ID: idPrefix + "-ann", Status: domain.PostStatusPending, TemplateOccurrenceAt: occ, TemplatePostRole: domain.TemplatePostRoleAnnouncement, TemplateCounter: &c},
	}
}

// Skipping the imminent round must drop that slot, delete its posts (including
// the announcement already sitting in the calendar) and renumber every later
// materialized round down by one — the edition number must not be burned.
func TestSkipPostTemplateOccurrence_dropsSlotAndRenumbersFutureRounds(t *testing.T) {
	// Three rounds materialized off the template's own rule, cursor parked past
	// them — the state the horizon leaves behind.
	tmpl := skipTestTemplate(time.Now().UTC())
	occs := ruleOccurrences(t, tmpl, 4)
	occ1, occ2, occ3, cursor := occs[0], occs[1], occs[2], occs[3]

	// Horizon wide enough to cover all four rounds after the rewind.
	tmpl.MaterializeHorizonDays = int(cursor.Sub(time.Now().UTC()).Hours()/24) + 2
	tmpl.NextMaterializeAt = &cursor
	tmpl.CounterNext = 388
	tmpl.AnnouncementCounterNext = 388

	var linked []domain.PostTemplateLinkedPost
	linked = append(linked, linkedRound("r385", occ1, 385)...)
	linked = append(linked, linkedRound("r386", occ2, 386)...)
	linked = append(linked, linkedRound("r387", occ3, 387)...)

	h := newSkipHarness(t, tmpl, linked)

	result, err := h.svc.SkipPostTemplateOccurrence(context.Background(), "team1", "tmpl1", occ1, nil)
	if err != nil {
		t.Fatalf("SkipPostTemplateOccurrence: %v", err)
	}

	// The skipped slot is gone entirely, announcement included.
	for _, r := range h.rounds() {
		if r.occ.Equal(occ1) {
			t.Errorf("skipped occurrence %s still materialized (counter %d)", occ1.Format(time.RFC3339), r.counter)
		}
	}
	h.st.mu.Lock()
	for _, p := range h.st.listPostTemplateLinkedPosts {
		if p.TemplateOccurrenceAt.UTC().Equal(occ1) {
			t.Errorf("post %s for skipped occurrence survived (role %s)", p.ID, p.TemplatePostRole)
		}
	}
	h.st.mu.Unlock()

	// Later rounds keep their dates but move down one edition.
	want := map[int64]int{
		occ2.UnixNano(): 385,
		occ3.UnixNano(): 386,
	}
	for _, r := range h.rounds() {
		if exp, ok := want[r.occ.UnixNano()]; ok {
			if r.counter != exp {
				t.Errorf("round %s: counter = %d, want %d", r.occ.Format(time.RFC3339), r.counter, exp)
			}
			delete(want, r.occ.UnixNano())
		}
	}
	for occ := range want {
		t.Errorf("round %s missing after skip", time.Unix(0, occ).UTC().Format(time.RFC3339))
	}

	// The freed edition number must be reused, not burned.
	lowest := 0
	for _, r := range h.rounds() {
		if lowest == 0 || r.counter < lowest {
			lowest = r.counter
		}
	}
	if lowest != 385 {
		t.Errorf("edition 385 was burned by the skip: lowest surviving counter = %d", lowest)
	}
	if result.DeletedPosts < 6 {
		t.Errorf("expected all materialized rounds torn down, deleted = %d", result.DeletedPosts)
	}
	t.Logf("result=%+v counter_next=%d", result, h.currentTemplate().CounterNext)
	for _, r := range h.rounds() {
		t.Logf("  occ=%s counter=%d", r.occ.Format(time.RFC3339), r.counter)
	}
}

// A skip for a round the cursor has not reached yet must still be recorded, but
// must not tear down or rewind anything.
func TestSkipPostTemplateOccurrence_unmaterializedRoundJustRecordsSkip(t *testing.T) {
	tmpl := skipTestTemplate(time.Now().UTC())
	occs := ruleOccurrences(t, tmpl, 4)
	future := occs[3]
	tmpl.NextMaterializeAt = &future
	h := newSkipHarness(t, tmpl, nil)

	if _, err := h.svc.SkipPostTemplateOccurrence(context.Background(), "team1", "tmpl1", future, nil); err != nil {
		t.Fatalf("SkipPostTemplateOccurrence: %v", err)
	}
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	if len(h.st.deleteLinkedPostsCalls) != 0 {
		t.Errorf("expected no deletions, got %v", h.st.deleteLinkedPostsCalls)
	}
	if len(h.st.setMaterializationStateCalls) != 0 {
		t.Errorf("expected no rewind, got %d calls", len(h.st.setMaterializationStateCalls))
	}
}

// Already-published rounds must never be torn down by a skip.
func TestSkipPostTemplateOccurrence_blockedByPostedRound(t *testing.T) {
	tmpl := skipTestTemplate(time.Now().UTC())
	occ := ruleOccurrences(t, tmpl, 1)[0]

	linked := linkedRound("r385", occ, 385)
	linked[0].Status = domain.PostStatusPosted

	h := newSkipHarness(t, tmpl, linked)
	if _, err := h.svc.SkipPostTemplateOccurrence(context.Background(), "team1", "tmpl1", occ, nil); err != ErrRegenerateBlocked {
		t.Fatalf("expected ErrRegenerateBlocked, got %v", err)
	}
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	if len(h.st.deleteLinkedPostsCalls) != 0 {
		t.Errorf("blocked skip must not delete anything, got %v", h.st.deleteLinkedPostsCalls)
	}
}

// Shifting the imminent round moves its posts to the new time while keeping the
// edition number — same tear-down/re-walk path as a skip.
func TestSkipPostTemplateOccurrence_shiftKeepsEditionAtNewTime(t *testing.T) {
	tmpl := skipTestTemplate(time.Now().UTC())
	occ := ruleOccurrences(t, tmpl, 1)[0]
	shiftTo := occ.Add(3 * 24 * time.Hour)

	h := newSkipHarness(t, tmpl, linkedRound("r385", occ, 385))

	if _, err := h.svc.SkipPostTemplateOccurrence(context.Background(), "team1", "tmpl1", occ, &shiftTo); err != nil {
		t.Fatalf("SkipPostTemplateOccurrence(shift): %v", err)
	}
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	found := false
	for _, in := range h.st.createScheduledPostCalls {
		if in.TemplatePostRole == domain.TemplatePostRoleMain && in.ScheduledAt.UTC().Equal(shiftTo) {
			found = true
			if in.TemplateCounter == nil || *in.TemplateCounter != 385 {
				t.Errorf("shifted round changed edition: %v, want 385", in.TemplateCounter)
			}
		}
	}
	if !found {
		t.Errorf("no main post recreated at shifted time %s", shiftTo.Format(time.RFC3339))
	}
}
