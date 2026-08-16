package scheduler

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
	"git.f4mily.net/goloom/internal/scheduling"
	"git.f4mily.net/goloom/internal/security"
	sqlitestore "git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

// The mock-based skip tests cannot catch SQL-level mismatches — timestamp
// formatting in the skip lookup, the linked-post delete, the materialization
// rewind. This drives the same flow through a real store.
func TestSkipPostTemplateOccurrence_againstSQLite(t *testing.T) {
	ctx := context.Background()
	enc, err := security.NewEncrypter("scheduler-skip-test-secret-32byte!")
	if err != nil {
		t.Fatal(err)
	}
	st, err := sqlitestore.New(ctx, "file:"+uuid.NewString()+"?mode=memory&cache=shared", enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	u, err := st.UpsertOIDCUser(ctx, "skip-"+uuid.NewString(), "skip@test.test", "Skip")
	if err != nil {
		t.Fatal(err)
	}
	team, err := st.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "skip-team-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	acc, err := st.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://mastodon.test", Username: "skip", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}

	recJSON := `{"kind":"monthly_ordinal_weekday","occurrences":[{"ordinal":1,"weekday":3},{"ordinal":3,"weekday":3}],"hour":10,"minute":0,"timezone":"UTC"}`
	tmpl, err := st.CreatePostTemplate(ctx, team.ID, domain.AuthenticatedPrincipal{User: u}, domain.CreatePostTemplateInput{
		Title:                   "Folge {counter}",
		Content:                 "Folge {counter}",
		RecurrenceJSON:          recJSON,
		TargetAccountIDs:        []string{acc.ID},
		MaterializeHorizonDays:  intPtr(60),
		AnnouncementEnabled:     boolPtr(true),
		AnnouncementTitle:       "Ankündigung Folge {main_counter}",
		AnnouncementContent:     "Folge #{main_counter} in 2 Tagen",
		AnnouncementDaysBefore:  intPtr(2),
		CounterNext:             intPtr(385),
		AnnouncementCounterNext: intPtr(385),
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}

	svc := New(testLogger(), st, provider.NewRegistry(), time.Minute, 1, 0, 0, 0, 0, nil)
	if err := svc.materializePostTemplates(ctx); err != nil {
		t.Fatalf("materializePostTemplates: %v", err)
	}

	before := roundsFromStore(t, st, team.ID, tmpl.ID)
	if len(before) < 2 {
		t.Fatalf("expected the horizon to materialize at least 2 rounds, got %d", len(before))
	}
	imminent := before[0]
	second := before[1]
	t.Logf("before skip: %s", formatRounds(before))

	result, err := svc.SkipPostTemplateOccurrence(ctx, team.ID, tmpl.ID, imminent.occ, nil)
	if err != nil {
		t.Fatalf("SkipPostTemplateOccurrence: %v", err)
	}
	after := roundsFromStore(t, st, team.ID, tmpl.ID)
	t.Logf("after skip (deleted %d): %s", result.DeletedPosts, formatRounds(after))

	// The skipped slot is gone from the calendar entirely.
	for _, r := range after {
		if r.occ.Equal(imminent.occ) {
			t.Errorf("skipped round %s still present with counter %d", r.occ.Format(time.RFC3339), r.counter)
		}
	}
	// Its announcement is gone too.
	linked, err := st.ListPostTemplateLinkedPosts(ctx, team.ID, tmpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range linked {
		if p.TemplateOccurrenceAt.UTC().Equal(imminent.occ) {
			t.Errorf("post %s (%s) for the skipped round survived", p.ID, p.TemplatePostRole)
		}
	}
	// The freed edition number moves to the next round instead of being burned.
	if len(after) == 0 {
		t.Fatal("no rounds left after skip")
	}
	if !after[0].occ.Equal(second.occ) {
		t.Errorf("first round after skip = %s, want %s", after[0].occ.Format(time.RFC3339), second.occ.Format(time.RFC3339))
	}
	if after[0].counter != imminent.counter {
		t.Errorf("edition %d was burned: round %s carries %d",
			imminent.counter, after[0].occ.Format(time.RFC3339), after[0].counter)
	}
	// And the skip is durable: re-running the materializer must not resurrect it.
	if err := svc.materializePostTemplates(ctx); err != nil {
		t.Fatalf("second materialize: %v", err)
	}
	for _, r := range roundsFromStore(t, st, team.ID, tmpl.ID) {
		if r.occ.Equal(imminent.occ) {
			t.Errorf("skipped round %s came back on the next tick", r.occ.Format(time.RFC3339))
		}
	}

	// Sanity: the rule really does produce the occurrence we skipped, so the test
	// is not passing because the date was never scheduled in the first place.
	rule, err := scheduling.ParseRecurrenceJSON(recJSON)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduling.NextOccurrence(rule, imminent.occ.Add(-time.Second)); err != nil {
		t.Fatalf("NextOccurrence: %v", err)
	}
}

func roundsFromStore(t *testing.T, st *sqlitestore.Store, teamID, templateID string) []roundView {
	t.Helper()
	linked, err := st.ListPostTemplateLinkedPosts(context.Background(), teamID, templateID)
	if err != nil {
		t.Fatal(err)
	}
	var out []roundView
	for _, p := range linked {
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

func intPtr(n int) *int    { return &n }
func boolPtr(b bool) *bool { return &b }

func formatRounds(rs []roundView) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, r.occ.Format("2006-01-02 15:04")+"=#"+strconv.Itoa(r.counter))
	}
	return strings.Join(parts, " ")
}
