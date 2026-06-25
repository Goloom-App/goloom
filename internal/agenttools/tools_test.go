package agenttools

import (
	"context"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"github.com/google/uuid"
)

var bg = context.Background

// ===== reads =====

func TestGetTeamsCore(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["read"]`)
	out, err := coreGetTeams(bg(), f.deps, inv, GetTeamsInput{})
	if err != nil {
		t.Fatalf("coreGetTeams: %v", err)
	}
	found := false
	for _, team := range out.Teams {
		if team.TeamID == f.team.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected team %s in result, got %+v", f.team.ID, out.Teams)
	}
}

func TestZeroPrincipalRejected(t *testing.T) {
	f := newFixture(t)
	// No principal bound: a team-scoped write must be refused, never executed.
	if _, err := coreDraftPost(bg(), f.deps, Invocation{}, DraftPostInput{
		TeamID: f.team.ID, Title: "x", Content: "x", TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("missing principal must be rejected")
	}
}

func TestDraftAndModifyPost(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["read","write"]`)

	draft, err := coreDraftPost(bg(), f.deps, inv, DraftPostInput{
		TeamID:         f.team.ID,
		Title:          "Draft",
		Content:        "Hallo",
		TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatalf("coreDraftPost: %v", err)
	}
	if draft.PostID == "" {
		t.Fatal("draft post id missing")
	}
	post, err := f.store.GetScheduledPost(bg(), f.team.ID, draft.PostID)
	if err != nil {
		t.Fatalf("draft not in store: %v", err)
	}
	if post.Status != domain.PostStatusDraft {
		t.Fatalf("status = %s, want draft", post.Status)
	}

	newContent := "Geänderter Inhalt"
	modified, err := coreModifyPost(bg(), f.deps, inv, ModifyPostInput{
		TeamID:  f.team.ID,
		PostID:  draft.PostID,
		Content: &newContent,
	})
	if err != nil {
		t.Fatalf("coreModifyPost: %v", err)
	}
	if modified.PostID != draft.PostID {
		t.Fatalf("modify result: %+v", modified)
	}
	post, _ = f.store.GetScheduledPost(bg(), f.team.ID, draft.PostID)
	if post.Content != newContent {
		t.Fatalf("content = %q, want %q", post.Content, newContent)
	}
}

func TestGetCalendarCore(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["read","write"]`)

	scheduledAt := time.Now().UTC().Add(24 * time.Hour)
	if _, err := f.store.CreateScheduledPost(bg(), f.team.ID, inv.Principal, domain.CreatePostInput{
		Content: "Kalendereintrag", ScheduledAt: scheduledAt, TargetAccounts: []string{f.account.ID},
	}); err != nil {
		t.Fatal(err)
	}

	out, err := coreGetCalendar(bg(), f.deps, inv, GetCalendarInput{
		TeamID:   f.team.ID,
		FromDate: time.Now().UTC().Format(time.RFC3339),
		ToDate:   time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("coreGetCalendar: %v", err)
	}
	if len(out.Posts) != 1 || out.Posts[0].Content != "Kalendereintrag" {
		t.Fatalf("calendar posts: %+v", out.Posts)
	}
}

func TestGetPlatformsCore(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["read"]`)
	out, err := coreGetPlatforms(bg(), f.deps, inv, GetPlatformsInput{TeamID: f.team.ID})
	if err != nil {
		t.Fatalf("coreGetPlatforms: %v", err)
	}
	if len(out.Accounts) != 1 || out.Accounts[0].AccountID != f.account.ID {
		t.Fatalf("accounts: %+v", out.Accounts)
	}
}

// ===== helpers =====

func TestParseWeekday(t *testing.T) {
	cases := map[string]int{"monday": 1, "Monday": 1, "sunday": 0, "saturday": 6, "unknown": -1, "": -1}
	for in, want := range cases {
		if got := ParseWeekday(in); got != want {
			t.Fatalf("ParseWeekday(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestTruncateString(t *testing.T) {
	if got := TruncateString("hello", 10); got != "hello" {
		t.Fatalf("short string changed: %q", got)
	}
	got := TruncateString("hello world", 8)
	if len(got) > 8+len("...") || got == "hello world" {
		t.Fatalf("truncate = %q", got)
	}
}

func TestFindNextFreeSlot(t *testing.T) {
	after := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC) // a Monday
	before := after.Add(14 * 24 * time.Hour)

	slot, ok := FindNextFreeSlot(nil, after, before, -1)
	if !ok || slot.Before(after) || slot.After(before) {
		t.Fatalf("free calendar: slot=%v ok=%v", slot, ok)
	}
	slot, ok = FindNextFreeSlot(nil, after, before, 3 /* Wednesday */)
	if !ok || slot.Weekday() != time.Wednesday {
		t.Fatalf("weekday slot = %v (%s), ok=%v", slot, slot.Weekday(), ok)
	}
	if _, ok := FindNextFreeSlot(nil, after, after.Add(time.Hour), 3); ok {
		t.Fatal("no Wednesday inside one hour window")
	}
}

// ===== campaigns / schedule / delete =====

func TestCampaignCore(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write","delete"]`)

	created, err := coreCreateCampaign(bg(), f.deps, inv, CreateCampaignInput{
		TeamID:           f.team.ID,
		Name:             "Montagsfrage",
		RequiredHashtags: []string{"#montag"},
	})
	if err != nil {
		t.Fatalf("coreCreateCampaign: %v", err)
	}
	if created.CampaignID == "" || created.Name != "Montagsfrage" {
		t.Fatalf("create result: %+v", created)
	}
	got, err := coreGetCampaign(bg(), f.deps, inv, GetCampaignInput{TeamID: f.team.ID, CampaignID: created.CampaignID})
	if err != nil {
		t.Fatalf("coreGetCampaign: %v", err)
	}
	if got.Name != "Montagsfrage" || !got.IsActive {
		t.Fatalf("get result: %+v", got)
	}
}

func TestScheduleGetAndDeletePost(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write","delete"]`)

	scheduledAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	scheduled, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID:         f.team.ID,
		Title:          "Geplanter Titel",
		Content:        "Geplanter Post",
		ScheduledAt:    scheduledAt.Format(time.RFC3339),
		TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatalf("coreSchedulePost: %v", err)
	}
	if scheduled.PostID == "" || scheduled.Status != string(domain.PostStatusPending) {
		t.Fatalf("schedule result: %+v", scheduled)
	}

	if _, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Content: "x", ScheduledAt: "not-a-date", TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("invalid scheduled_at must error")
	}

	posts, err := coreGetPosts(bg(), f.deps, inv, GetPostsInput{TeamID: f.team.ID})
	if err != nil {
		t.Fatalf("coreGetPosts: %v", err)
	}
	if posts.Total != 1 {
		t.Fatalf("posts total = %d, want 1", posts.Total)
	}

	deleted, err := coreDeletePost(bg(), f.deps, inv, DeletePostInput{TeamID: f.team.ID, PostID: scheduled.PostID})
	if err != nil {
		t.Fatalf("coreDeletePost: %v", err)
	}
	if !deleted.Success {
		t.Fatalf("delete result: %+v", deleted)
	}
	if _, err := f.store.GetScheduledPost(bg(), f.team.ID, scheduled.PostID); err == nil {
		t.Fatal("post must be gone after delete")
	}
}

func TestForbiddenForOutsider(t *testing.T) {
	f := newFixture(t)
	outsider, err := f.store.UpsertOIDCUser(bg(), "outsider-"+uuid.NewString(), "out@test", "Out")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := f.store.CreateUserAPIToken(bg(), outsider.ID, "out", nil, `["write","delete"]`, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := f.store.LookupAPIToken(bg(), token)
	if err != nil {
		t.Fatal(err)
	}
	inv := Invocation{Principal: principal, Transport: TransportMCP}

	if _, err := coreDraftPost(bg(), f.deps, inv, DraftPostInput{
		TeamID: f.team.ID, Content: "fremd", TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("outsider must not draft into a foreign team")
	}
	if _, err := coreGetCalendar(bg(), f.deps, inv, GetCalendarInput{TeamID: f.team.ID}); err == nil {
		t.Fatal("outsider must not read a foreign calendar")
	}
}

func TestGetAnalyticsTimeslotsCore(t *testing.T) {
	f := newFixture(t)
	principal := f.principal(t, `["read"]`)
	inv := Invocation{Principal: principal, Transport: TransportMCP}

	mon := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	for i, likes := range []int64{8, 4} {
		post, err := f.store.CreateScheduledPost(bg(), f.team.ID, principal, domain.CreatePostInput{
			Content: "x", ScheduledAt: mon.Add(time.Duration(i) * time.Minute), TargetAccounts: []string{f.account.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.MarkPostResult(bg(), post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
			t.Fatal(err)
		}
		if err := f.store.UpsertPostMetrics(bg(), post.ID, f.account.ID, map[string]int64{"likes": likes}, ""); err != nil {
			t.Fatal(err)
		}
	}

	out, err := coreGetAnalyticsTimeslots(bg(), f.deps, inv, GetAnalyticsTimeslotsInput{TeamID: f.team.ID})
	if err != nil {
		t.Fatalf("coreGetAnalyticsTimeslots: %v", err)
	}
	if out.Timezone != "UTC" || len(out.Timeslots) != 1 {
		t.Fatalf("out = %#v", out)
	}
	slot := out.Timeslots[0]
	if slot.Weekday != "Monday" || slot.Hour != 10 || slot.Posts != 2 || slot.TotalEngagement != 12 || slot.AvgEngagement != 6 {
		t.Fatalf("slot = %#v", slot)
	}

	berlin, err := coreGetAnalyticsTimeslots(bg(), f.deps, inv, GetAnalyticsTimeslotsInput{TeamID: f.team.ID, Timezone: "Europe/Berlin"})
	if err != nil {
		t.Fatalf("berlin: %v", err)
	}
	if len(berlin.Timeslots) != 1 || berlin.Timeslots[0].Hour != 12 {
		t.Fatalf("berlin slot = %#v, want hour 12", berlin.Timeslots)
	}
	if _, err := coreGetAnalyticsTimeslots(bg(), f.deps, inv, GetAnalyticsTimeslotsInput{TeamID: f.team.ID, Timezone: "Mars/Olympus"}); err == nil {
		t.Fatal("invalid timezone must error")
	}
}

// ===== schedule_post validation (postservice pipeline through the tool) =====

func TestGetAccountGrowthCore(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["read"]`)

	now := time.Now().UTC()
	if err := f.store.UpsertAccountMetrics(bg(), f.account.ID, map[string]int64{"followers": 100, "following": 50, "posts": 10}, now.AddDate(0, 0, -20)); err != nil {
		t.Fatal(err)
	}
	if err := f.store.UpsertAccountMetrics(bg(), f.account.ID, map[string]int64{"followers": 150, "following": 60, "posts": 20}, now.AddDate(0, 0, -2)); err != nil {
		t.Fatal(err)
	}

	out, err := coreGetAccountGrowth(bg(), f.deps, inv, GetAccountGrowthInput{TeamID: f.team.ID, Days: 30})
	if err != nil {
		t.Fatalf("coreGetAccountGrowth: %v", err)
	}
	if len(out.Points) != 30 {
		t.Fatalf("len(points) = %d, want 30", len(out.Points))
	}
	// Baseline anchors on the first synced day (-20), not the leading fill zeros.
	if out.FollowersStart != 100 || out.FollowersEnd != 150 || out.FollowersDelta != 50 {
		t.Fatalf("followers start/end/delta = %d/%d/%d, want 100/150/50", out.FollowersStart, out.FollowersEnd, out.FollowersDelta)
	}
	if out.FollowingDelta != 10 || out.PostsDelta != 10 {
		t.Fatalf("following/posts delta = %d/%d, want 10/10", out.FollowingDelta, out.PostsDelta)
	}
	if last := out.Points[len(out.Points)-1]; last.Followers != 150 {
		t.Fatalf("last point followers = %d, want 150 (forward-filled)", last.Followers)
	}
	if out.AccountID != "all" {
		t.Fatalf("account_id = %q, want all", out.AccountID)
	}
}

func TestGetMetricHistoryCore(t *testing.T) {
	f := newFixture(t)
	principal := f.principal(t, `["read"]`)
	inv := Invocation{Principal: principal, Transport: TransportMCP}

	now := time.Now().UTC()
	post, err := f.store.CreateScheduledPost(bg(), f.team.ID, principal, domain.CreatePostInput{
		Content: "x", ScheduledAt: now.Add(time.Hour), TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.MarkPostResult(bg(), post.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := f.store.UpsertPostMetrics(bg(), post.ID, f.account.ID, map[string]int64{"likes": 4}, now.AddDate(0, 0, -10).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	if err := f.store.UpsertPostMetrics(bg(), post.ID, f.account.ID, map[string]int64{"likes": 12}, now.AddDate(0, 0, -1).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}

	out, err := coreGetMetricHistory(bg(), f.deps, inv, GetMetricHistoryInput{TeamID: f.team.ID, Metric: "likes", Days: 30})
	if err != nil {
		t.Fatalf("coreGetMetricHistory: %v", err)
	}
	if out.Metric != "likes" || len(out.Points) != 30 {
		t.Fatalf("metric=%q points=%d, want likes/30", out.Metric, len(out.Points))
	}
	if out.Start != 4 || out.End != 12 || out.Delta != 8 {
		t.Fatalf("start/end/delta = %d/%d/%d, want 4/12/8", out.Start, out.End, out.Delta)
	}

	if _, err := coreGetMetricHistory(bg(), f.deps, inv, GetMetricHistoryInput{TeamID: f.team.ID}); err == nil {
		t.Fatal("empty metric must error")
	}
}

func TestSchedulePost_RejectsOversizedContent(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	if _, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "Test post", Content: strings.Repeat("x", 400),
		ScheduledAt: soon(), TargetAccounts: []string{bsky.ID},
	}); err == nil {
		t.Fatal("oversized content must be rejected")
	}
	posts, _ := f.store.ListTeamPosts(bg(), f.team.ID)
	if len(posts) != 0 {
		t.Fatalf("no post must be saved when validation fails, got %d", len(posts))
	}
}

func TestSchedulePost_AppliesValidOverride(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	out, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "Test post", Content: strings.Repeat("x", 400),
		ScheduledAt: soon(), TargetAccounts: []string{bsky.ID},
		AccountContentOverride: map[string]string{bsky.ID: "short text for bluesky"},
	})
	if err != nil {
		t.Fatalf("valid override must be accepted: %v", err)
	}
	versions, err := f.store.ListPostVersionsForTeamPost(bg(), f.team.ID, out.PostID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].AccountID != bsky.ID || versions[0].Content != "short text for bluesky" {
		t.Fatalf("override must be persisted as a post version, got %+v", versions)
	}
}

func TestSchedulePost_OverrideKeyMismatchRejected(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	bsky := f.blueskyAccount(t)
	if _, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "Test post", Content: "fits everywhere",
		ScheduledAt: soon(), TargetAccounts: []string{bsky.ID},
		AccountContentOverride: map[string]string{"some-other-id": "ignored"},
	}); err == nil {
		t.Fatal("override for a non-target account must be rejected, not silently dropped")
	}
}

func TestSchedulePost_CrossTeamTargetRejected(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	foreign := f.foreignAccount(t)
	_, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "Test post", Content: "hello",
		ScheduledAt: soon(), TargetAccounts: []string{foreign.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "does not belong to team") {
		t.Fatalf("targeting another team's account must be rejected, got %v", err)
	}
}

func TestSchedulePost_UnknownTargetRejected(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	if _, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "Test post", Content: "hello",
		ScheduledAt: soon(), TargetAccounts: []string{uuid.NewString()},
	}); err == nil {
		t.Fatal("unknown target account must be rejected")
	}
}

func TestSchedulePost_EmptyContentRejected(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	if _, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "Test post", Content: "   ",
		ScheduledAt: soon(), TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("empty content must be rejected")
	}
}

func TestSchedulePost_EmptyTargetsRejected(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	if _, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "Test post", Content: "hello", ScheduledAt: soon(),
	}); err == nil {
		t.Fatal("missing target_accounts must be rejected")
	}
}

func TestSchedulePost_RequiresTitle(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	if _, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "   ", Content: "hello",
		ScheduledAt: soon(), TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("schedule_post must require an explicit title")
	}
}

func TestSchedulePost_RequiresTeam(t *testing.T) {
	f := newFixture(t)
	bsky := f.blueskyAccount(t)
	// An admin passes the team-access check even for an empty team, so the tool
	// itself must reject an empty team_id (via the pipeline's RequireTeam).
	admin := domain.AuthenticatedPrincipal{User: domain.User{ID: f.user.ID, IsAdmin: true}}
	inv := Invocation{Principal: admin, Transport: TransportMCP}
	_, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: "", Title: "T", Content: "hello", ScheduledAt: soon(), TargetAccounts: []string{bsky.ID},
	})
	if err == nil || !strings.Contains(err.Error(), "team_id") {
		t.Fatalf("schedule_post must reject an empty team_id, got %v", err)
	}
}

func TestSchedulePost_RecordsAuditEvent(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	created, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "Audit me", Content: "hi",
		ScheduledAt: soon(), TargetAccounts: []string{bsky.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := f.store.ListAuditEvents(bg(), domain.AuditFilter{TeamID: f.team.ID})
	if err != nil {
		t.Fatal(err)
	}
	var found *domain.AuditEvent
	for i := range events {
		if events[i].Action == "post.create" && events[i].TargetID != nil && *events[i].TargetID == created.PostID {
			found = &events[i]
		}
	}
	if found == nil {
		t.Fatalf("schedule_post must record a post.create audit event, got %d events", len(events))
	}
	if found.ActorKind != domain.AuditActorToken || found.TokenID == nil {
		t.Fatalf("audit must be attributed to the API token, got kind=%q token=%v", found.ActorKind, found.TokenID)
	}
}

// ===== draft_post validation =====

func TestDraftPost_AllowsOversizedButValidatesTargets(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	if _, err := coreDraftPost(bg(), f.deps, inv, DraftPostInput{
		TeamID: f.team.ID, Title: "Test post", Content: strings.Repeat("x", 400),
		TargetAccounts: []string{bsky.ID},
	}); err != nil {
		t.Fatalf("oversized draft must be allowed: %v", err)
	}
	foreign := f.foreignAccount(t)
	if _, err := coreDraftPost(bg(), f.deps, inv, DraftPostInput{
		TeamID: f.team.ID, Title: "Test post", Content: "hi", TargetAccounts: []string{foreign.ID},
	}); err == nil {
		t.Fatal("draft targeting another team's account must be rejected")
	}
}

func TestDraftPost_RequiresTitle(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	if _, err := coreDraftPost(bg(), f.deps, inv, DraftPostInput{
		TeamID: f.team.ID, Content: "hello", TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("draft_post must require an explicit title")
	}
}

// ===== modify_post validation =====

func TestModifyPost_RejectsOversizedContentOnScheduled(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	created, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "Test post", Content: "short",
		ScheduledAt: soon(), TargetAccounts: []string{bsky.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("y", 400)
	if _, err := coreModifyPost(bg(), f.deps, inv, ModifyPostInput{
		TeamID: f.team.ID, PostID: created.PostID, Content: &long,
	}); err == nil {
		t.Fatal("modifying to oversized content must be rejected")
	}
	post, _ := f.store.GetScheduledPost(bg(), f.team.ID, created.PostID)
	if post.Content != "short" {
		t.Fatalf("content must be unchanged after rejected modify, got %q", post.Content)
	}
}

func TestModifyPost_ShrinkTargetsWithStoredVersions(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	bsky := f.blueskyAccount(t)

	created, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "T", Content: "short", ScheduledAt: soon(),
		TargetAccounts:         []string{f.account.ID, bsky.ID},
		AccountContentOverride: map[string]string{bsky.ID: "bluesky text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	newTargets := []string{f.account.ID}
	if _, err := coreModifyPost(bg(), f.deps, inv, ModifyPostInput{
		TeamID: f.team.ID, PostID: created.PostID, TargetAccounts: &newTargets,
	}); err != nil {
		t.Fatalf("shrinking targets with a stored version must succeed: %v", err)
	}
}

func TestModifyPost_RejectsEmptyTitle(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	created, err := coreSchedulePost(bg(), f.deps, inv, SchedulePostInput{
		TeamID: f.team.ID, Title: "Original", Content: "short",
		ScheduledAt: soon(), TargetAccounts: []string{f.account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	empty := "  "
	if _, err := coreModifyPost(bg(), f.deps, inv, ModifyPostInput{
		TeamID: f.team.ID, PostID: created.PostID, Title: &empty,
	}); err == nil {
		t.Fatal("modify_post must reject clearing the title to empty")
	}
}

// ===== create_recurring / create_rss_feed / create_campaign validation =====

func TestCreateRecurring_RejectsCrossTeamTarget(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	foreign := f.foreignAccount(t)
	if _, err := coreCreateRecurring(bg(), f.deps, inv, CreateRecurringInput{
		TeamID: f.team.ID, Title: "t", Content: "c",
		RecurrenceJSON: `{"freq":"WEEKLY","interval":1,"byday":["TU"]}`,
		TargetAccounts: []string{foreign.ID},
	}); err == nil {
		t.Fatal("recurring with a cross-team target must be rejected")
	}
}

func TestCreateRecurring_RequiresContent(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	if _, err := coreCreateRecurring(bg(), f.deps, inv, CreateRecurringInput{
		TeamID: f.team.ID, Title: "t", Content: "  ",
		RecurrenceJSON: `{"freq":"WEEKLY"}`, TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("recurring without content must be rejected")
	}
}

func TestCreateRSSFeed_RejectsInvalidURL(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	for _, bad := range []string{"", "not a url", "ftp://example.com/feed"} {
		if _, err := coreCreateRSSFeed(bg(), f.deps, inv, CreateRSSFeedInput{
			TeamID: f.team.ID, FeedURL: bad, Name: "feed", TargetAccountIDs: []string{f.account.ID},
		}); err == nil {
			t.Fatalf("feed_url %q must be rejected", bad)
		}
	}
}

func TestCreateCampaign_RequiresName(t *testing.T) {
	f := newFixture(t)
	inv := f.inv(t, `["write"]`)
	if _, err := coreCreateCampaign(bg(), f.deps, inv, CreateCampaignInput{TeamID: f.team.ID, Name: "   "}); err == nil {
		t.Fatal("campaign without a name must be rejected")
	}
}

// ===== scope enforcement =====

func TestWriteToolsRequireScopes(t *testing.T) {
	f := newFixture(t)
	readOnly := f.inv(t, `["read"]`)

	if _, err := coreSchedulePost(bg(), f.deps, readOnly, SchedulePostInput{
		TeamID: f.team.ID, Content: "x", ScheduledAt: soon(), TargetAccounts: []string{f.account.ID},
	}); err == nil {
		t.Fatal("schedule_post must require a write scope")
	}
	if _, err := coreDeletePost(bg(), f.deps, readOnly, DeletePostInput{TeamID: f.team.ID, PostID: uuid.NewString()}); err == nil {
		t.Fatal("delete_post must require the delete scope")
	}
	if _, err := coreCreateCampaign(bg(), f.deps, readOnly, CreateCampaignInput{TeamID: f.team.ID, Name: "c"}); err == nil {
		t.Fatal("create_campaign must require a write scope")
	}
}
