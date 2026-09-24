package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/store/postgres"
	"github.com/google/uuid"
)

func pushFixture(t *testing.T, s *postgres.Store, role domain.TeamRole) (domain.User, domain.Team) {
	t.Helper()
	ctx := context.Background()
	user, err := s.UpsertOIDCUser(ctx, "push-"+uuid.NewString(), "push@pg.test", "Push")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, user.ID, domain.CreateTeamInput{
		Name:        "push-" + uuid.NewString(),
		Description: "d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if role != domain.RoleOwner {
		if _, err := s.AddTeamMember(ctx, team.ID, domain.AddTeamMemberInput{
			UserID: user.ID, Role: role,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return user, team
}

func newSub(userID, endpoint string) domain.PushSubscription {
	return domain.PushSubscription{
		UserID:   userID,
		Endpoint: endpoint,
		P256dh:   "p256dh-" + endpoint,
		Auth:     "auth-" + endpoint,
		Enabled:  true,
	}
}

func TestPostgres_PushSubscriptionCRUD(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, _ := pushFixture(t, s, domain.RoleOwner)

	created, err := s.CreatePushSubscription(ctx, user.ID, newSub(user.ID, "https://push.test/"+uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	if !created.Enabled || created.ID == "" || created.UserID != user.ID {
		t.Fatalf("created: %#v", created)
	}

	list, err := s.ListPushSubscriptions(ctx, user.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %#v", err, list)
	}

	got, err := s.GetPushSubscription(ctx, user.ID, created.ID)
	if err != nil || got.Endpoint != created.Endpoint {
		t.Fatalf("get: %v %#v", err, got)
	}

	disabled, err := s.UpdatePushSubscriptionEnabled(ctx, user.ID, created.ID, false)
	if err != nil || disabled.Enabled {
		t.Fatalf("disable: %v %#v", err, disabled)
	}

	if _, err := s.UpdatePushSubscriptionEnabled(ctx, uuid.NewString(), created.ID, true); !errors.Is(err, domain.ErrPushSubscriptionNotFound) {
		t.Fatalf("update by other user: %v", err)
	}
	if _, err := s.GetPushSubscription(ctx, uuid.NewString(), created.ID); !errors.Is(err, domain.ErrPushSubscriptionNotFound) {
		t.Fatalf("get by other user: %v", err)
	}

	if err := s.DeletePushSubscription(ctx, uuid.NewString(), created.ID); !errors.Is(err, domain.ErrPushSubscriptionNotFound) {
		t.Fatalf("delete by other user: %v", err)
	}
	if err := s.DeletePushSubscription(ctx, user.ID, created.ID); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListPushSubscriptions(ctx, user.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("list after delete: %v %#v", err, list)
	}
	if err := s.DeletePushSubscription(ctx, user.ID, created.ID); !errors.Is(err, domain.ErrPushSubscriptionNotFound) {
		t.Fatalf("double delete: %v", err)
	}
}

func TestPostgres_PushSubscriptionRetire(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, _ := pushFixture(t, s, domain.RoleOwner)
	created, err := s.CreatePushSubscription(ctx, user.ID, newSub(user.ID, "https://push.test/"+uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RetirePushSubscription(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPushSubscription(ctx, user.ID, created.ID); !errors.Is(err, domain.ErrPushSubscriptionNotFound) {
		t.Fatalf("retired sub still present: %v", err)
	}
}

func TestPostgres_PushSubscriptionResubscribeReenables(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, _ := pushFixture(t, s, domain.RoleOwner)
	endpoint := "https://push.test/" + uuid.NewString()

	first, err := s.CreatePushSubscription(ctx, user.ID, newSub(user.ID, endpoint))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdatePushSubscriptionEnabled(ctx, user.ID, first.ID, false); err != nil {
		t.Fatal(err)
	}
	second, err := s.CreatePushSubscription(ctx, user.ID, newSub(user.ID, endpoint))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("endpoint reuse should keep id: %q vs %q", second.ID, first.ID)
	}
	if !second.Enabled {
		t.Fatal("resubscribe must re-enable the device master switch")
	}
}

func TestPostgres_ListPushTargetsRoleAndPrefs(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	owner, ownerTeam := pushFixture(t, s, domain.RoleOwner)
	editor, editorTeam := pushFixture(t, s, domain.RoleEditor)
	viewer, viewerTeam := pushFixture(t, s, domain.RoleViewer)

	ownerSub, err := s.CreatePushSubscription(ctx, owner.ID, newSub(owner.ID, "https://push.test/owner-"+uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePushSubscription(ctx, editor.ID, newSub(editor.ID, "https://push.test/editor-"+uuid.NewString())); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePushSubscription(ctx, viewer.ID, newSub(viewer.ID, "https://push.test/viewer-"+uuid.NewString())); err != nil {
		t.Fatal(err)
	}

	// Missing pref row defaults to enabled.
	targets, err := s.ListPushTargets(ctx, ownerTeam.ID)
	if err != nil || len(targets) != 1 || targets[0].UserID != owner.ID {
		t.Fatalf("owner default: %v %#v", err, targets)
	}
	targets, err = s.ListPushTargets(ctx, editorTeam.ID)
	if err != nil || len(targets) != 1 || targets[0].UserID != editor.ID {
		t.Fatalf("editor default: %v %#v", err, targets)
	}
	// Viewer members never receive notifications.
	targets, err = s.ListPushTargets(ctx, viewerTeam.ID)
	if err != nil || len(targets) != 0 {
		t.Fatalf("viewer must not be a target: %v %#v", err, targets)
	}

	// Per-team pref disables the team for this user.
	if _, err := s.SetTeamNotificationPref(ctx, owner.ID, ownerTeam.ID, false); err != nil {
		t.Fatal(err)
	}
	targets, err = s.ListPushTargets(ctx, ownerTeam.ID)
	if err != nil || len(targets) != 0 {
		t.Fatalf("pref disabled: %v %#v", err, targets)
	}

	// Device master switch disables the device for all teams.
	if _, err := s.SetTeamNotificationPref(ctx, owner.ID, ownerTeam.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdatePushSubscriptionEnabled(ctx, owner.ID, ownerSub.ID, false); err != nil {
		t.Fatal(err)
	}
	targets, err = s.ListPushTargets(ctx, ownerTeam.ID)
	if err != nil || len(targets) != 0 {
		t.Fatalf("device disabled: %v %#v", err, targets)
	}
}

func TestPostgres_TeamNotificationPrefsUpsert(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, team := pushFixture(t, s, domain.RoleOwner)

	prefs, err := s.ListTeamNotificationPrefs(ctx, user.ID)
	if err != nil || len(prefs) != 0 {
		t.Fatalf("empty prefs: %v %#v", err, prefs)
	}

	pref, err := s.SetTeamNotificationPref(ctx, user.ID, team.ID, false)
	if err != nil || pref.Enabled {
		t.Fatalf("set false: %v %#v", err, pref)
	}
	pref, err = s.SetTeamNotificationPref(ctx, user.ID, team.ID, true)
	if err != nil || !pref.Enabled {
		t.Fatalf("set true: %v %#v", err, pref)
	}
	prefs, err = s.ListTeamNotificationPrefs(ctx, user.ID)
	if err != nil || len(prefs) != 1 || !prefs[0].Enabled {
		t.Fatalf("list: %v %#v", err, prefs)
	}
}

func TestPostgres_VAPIDKeysRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.GetVAPIDKeys(ctx); !errors.Is(err, domain.ErrVAPIDKeysNotSet) {
		t.Fatalf("empty: %v", err)
	}

	if err := s.UpsertVAPIDKeys(ctx, domain.VAPIDKeys{
		PublicKey: "pub", PrivateKey: "priv", UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	keys, err := s.GetVAPIDKeys(ctx)
	if err != nil || keys.PublicKey != "pub" || keys.PrivateKey != "priv" {
		t.Fatalf("get: %v %#v", err, keys)
	}

	if err := s.UpsertVAPIDKeys(ctx, domain.VAPIDKeys{
		PublicKey: "pub2", PrivateKey: "priv2", UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	keys, err = s.GetVAPIDKeys(ctx)
	if err != nil || keys.PublicKey != "pub2" {
		t.Fatalf("get after upsert: %v %#v", err, keys)
	}
}

func TestPostgres_CountOpenReviewItems(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, team := pushFixture(t, s, domain.RoleOwner)
	principal := domain.AuthenticatedPrincipal{User: user, Kind: "session"}
	now := time.Now().UTC()

	base := domain.CreatePostInput{
		Title:       "in review",
		Content:     "body",
		ScheduledAt: now,
		Draft:       true,
		Source:      domain.PostSourceAutomation,
	}
	if _, err := s.CreateScheduledPost(ctx, team.ID, principal, base); err != nil {
		t.Fatal(err)
	}
	scheduled := base
	scheduled.Title = "already scheduled"
	scheduled.Draft = false
	if _, err := s.CreateScheduledPost(ctx, team.ID, principal, scheduled); err != nil {
		t.Fatal(err)
	}
	manualDraft := base
	manualDraft.Title = "manual draft"
	manualDraft.Source = domain.PostSourceScheduled
	if _, err := s.CreateScheduledPost(ctx, team.ID, principal, manualDraft); err != nil {
		t.Fatal(err)
	}

	count, err := s.CountOpenReviewItems(ctx, team.ID)
	if err != nil || count != 1 {
		t.Fatalf("count: %v %d, want 1 (only draft automation posts)", err, count)
	}

	_, other := pushFixture(t, s, domain.RoleOwner)
	otherCount, err := s.CountOpenReviewItems(ctx, other.ID)
	if err != nil || otherCount != 0 {
		t.Fatalf("other team count: %v %d", err, otherCount)
	}
}

func TestPostgres_CountUserOpenReviewItems(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	user, teamA := pushFixture(t, s, domain.RoleOwner)
	principal := domain.AuthenticatedPrincipal{User: user, Kind: "session"}
	now := time.Now().UTC()

	base := domain.CreatePostInput{
		Title:       "in review",
		Content:     "body",
		ScheduledAt: now,
		Draft:       true,
		Source:      domain.PostSourceAutomation,
	}
	if _, err := s.CreateScheduledPost(ctx, teamA.ID, principal, base); err != nil {
		t.Fatal(err)
	}
	// A second owned team with another open item: the user badge total must sum
	// across teams, while each team's count stays at one.
	teamB, err := s.CreateTeam(ctx, user.ID, domain.CreateTeamInput{Name: "push-b-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateScheduledPost(ctx, teamB.ID, principal, base); err != nil {
		t.Fatal(err)
	}
	// A viewer membership in a third team with open items must not count.
	viewer, _ := pushFixture(t, s, domain.RoleOwner)
	viewerTeam, err := s.CreateTeam(ctx, viewer.ID, domain.CreateTeamInput{Name: "push-v-" + uuid.NewString(), Description: ""})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateScheduledPost(ctx, viewerTeam.ID, domain.AuthenticatedPrincipal{User: domain.User{ID: viewer.ID}, Kind: "session"}, base); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddTeamMember(ctx, viewerTeam.ID, domain.AddTeamMemberInput{UserID: user.ID, Role: domain.RoleViewer}); err != nil {
		t.Fatal(err)
	}

	total, err := s.CountUserOpenReviewItems(ctx, user.ID)
	if err != nil || total != 2 {
		t.Fatalf("user total: %v %d, want 2 (sum across owned teams, viewer excluded)", err, total)
	}
	teamACount, err := s.CountOpenReviewItems(ctx, teamA.ID)
	if err != nil || teamACount != 1 {
		t.Fatalf("team A count: %v %d, want 1", err, teamACount)
	}
	// A user with no memberships has zero.
	stranger, _ := pushFixture(t, s, domain.RoleOwner)
	zero, err := s.CountUserOpenReviewItems(ctx, stranger.ID)
	if err != nil || zero != 0 {
		t.Fatalf("stranger total: %v %d, want 0", err, zero)
	}
}