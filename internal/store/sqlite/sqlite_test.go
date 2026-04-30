package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func newTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	enc, err := security.NewEncrypter("sqlite-integration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Isolated named memory DB per test (shared cache for single pool).
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	s, err := sqlite.New(ctx, dsn, enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLite_UpsertOIDCUser_firstIsAdmin(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u1, err := s.UpsertOIDCUser(ctx, "sub-1", "a@x", "A")
	if err != nil {
		t.Fatal(err)
	}
	if !u1.IsAdmin {
		t.Fatal("first user should be admin")
	}
	u2, err := s.UpsertOIDCUser(ctx, "sub-2", "b@x", "B")
	if err != nil {
		t.Fatal(err)
	}
	if u2.IsAdmin {
		t.Fatal("second user should not be admin by default")
	}
}

func TestSQLite_UpsertOIDCUser_updatesExisting(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.UpsertOIDCUser(ctx, "same", "old@x", "Old"); err != nil {
		t.Fatal(err)
	}
	u, err := s.UpsertOIDCUser(ctx, "same", "new@x", "New")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "new@x" || u.Name != "New" {
		t.Fatalf("user: %+v", u)
	}
}

func TestSQLite_EnsureBootstrapAdmin_and_LookupAPIToken(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	token := "bootstrap-plain-token"
	if err := s.EnsureBootstrapAdmin(ctx, "admin@local", "Admin", token); err != nil {
		t.Fatal(err)
	}
	p, err := s.LookupAPIToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "api_token" || p.User.Email != "admin@local" || !p.User.IsAdmin {
		t.Fatalf("principal: %+v", p)
	}
	if _, err := s.LookupAPIToken(ctx, "wrong-token"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("lookup wrong token: %v", err)
	}
}

func TestSQLite_ListUsers_SetUserAdmin(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.UpsertOIDCUser(ctx, "u1", "u1@x", "U1"); err != nil {
		t.Fatal(err)
	}
	u1, err := s.UpsertOIDCUser(ctx, "u2", "u2@x", "U2")
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListUsers(ctx)
	if err != nil || len(list) < 2 {
		t.Fatalf("ListUsers: %v %#v", err, list)
	}
	updated, err := s.SetUserAdmin(ctx, u1.ID, true)
	if err != nil || !updated.IsAdmin {
		t.Fatalf("SetUserAdmin: %+v %v", updated, err)
	}
}

func TestSQLite_TeamsAndMemberships(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	owner, err := s.UpsertOIDCUser(ctx, "owner", "o@x", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.UpsertOIDCUser(ctx, "member", "m@x", "Member")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{
		Name:        "t-" + uuid.NewString(),
		Description: "d",
	})
	if err != nil {
		t.Fatal(err)
	}
	teams, err := s.ListTeamsForUser(ctx, owner.ID, false)
	if err != nil || len(teams) != 1 {
		t.Fatalf("owner teams: %v %#v", err, teams)
	}
	allTeams, err := s.ListTeamsForUser(ctx, member.ID, true)
	if err != nil || len(allTeams) < 1 {
		t.Fatalf("admin list: %v", err)
	}
	if _, err := s.AddTeamMember(ctx, team.ID, domain.AddTeamMemberInput{UserID: member.ID, Role: domain.RoleEditor}); err != nil {
		t.Fatal(err)
	}
	members, err := s.ListTeamMembers(ctx, team.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("members: %#v %v", members, err)
	}
	ok, err := s.UserHasAnyTeamRole(ctx, member.ID, team.ID, domain.RoleEditor)
	if err != nil || !ok {
		t.Fatalf("UserHasAnyTeamRole editor: ok=%v err=%v", ok, err)
	}
	ok, err = s.UserHasAnyTeamRole(ctx, member.ID, team.ID, domain.RoleOwner)
	if err != nil || ok {
		t.Fatalf("UserHasAnyTeamRole owner mismatch: ok=%v err=%v", ok, err)
	}
	if err := s.RemoveTeamMember(ctx, team.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	members, err = s.ListTeamMembers(ctx, team.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("after remove: %#v", members)
	}
}

func TestSQLite_ProviderInstances(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "pi", "pi@x", "PI")
	input := domain.PreparedProviderInstance{
		Provider:              "mastodon",
		Name:                  "inst-" + uuid.NewString(),
		InstanceURL:           "https://social.example",
		ClientID:              "cid",
		ClientSecret:        "secret",
		Scopes:                []string{"read", "write"},
		AuthorizationEndpoint: "https://social.example/oauth/authorize",
		TokenEndpoint:         "https://social.example/oauth/token",
	}
	created, err := s.CreateProviderInstance(ctx, u.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if !created.HasClientSecret {
		t.Fatal("expected HasClientSecret")
	}
	plain, err := s.DecryptProviderInstanceClientSecret(created)
	if err != nil || plain != "secret" {
		t.Fatalf("decrypt secret: %q %v", plain, err)
	}
	emptySecret, err := s.CreateProviderInstance(ctx, u.ID, domain.PreparedProviderInstance{
		Provider:    "mastodon",
		Name:        "no-secret-" + uuid.NewString(),
		InstanceURL: "https://other.example",
		ClientID:    "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	plain2, err := s.DecryptProviderInstanceClientSecret(emptySecret)
	if err != nil || plain2 != "" {
		t.Fatalf("empty client secret decrypt: %q %v", plain2, err)
	}
	all, err := s.ListProviderInstances(ctx, "")
	if err != nil || len(all) < 2 {
		t.Fatalf("List all: %v len=%d", err, len(all))
	}
	filtered, err := s.ListProviderInstances(ctx, "mastodon")
	if err != nil || len(filtered) < 2 {
		t.Fatalf("List mastodon: %v", err)
	}
	byID, err := s.GetProviderInstanceByID(ctx, created.ID)
	if err != nil || byID.ID != created.ID {
		t.Fatal(err)
	}
	updated, err := s.UpdateProviderInstance(ctx, created.ID, domain.PreparedProviderInstance{
		Provider:    "mastodon",
		Name:        "renamed",
		InstanceURL: "https://social.example",
		ClientID:    "cid2",
	})
	if err != nil || updated.Name != "renamed" || updated.ClientID != "cid2" {
		t.Fatalf("update: %+v %v", updated, err)
	}
}

func TestSQLite_SocialAccounts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "acc", "a@x", "A")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "team-" + uuid.NewString(), Description: ""})
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider:        "mastodon",
		AuthType:        domain.AccountAuthTypeOAuthToken,
		InstanceURL:     "https://m.example",
		Username:        "user",
		RemoteAccountID: "1",
		AccessToken:     "at",
		RefreshToken:    "rt",
	})
	if err != nil {
		t.Fatal(err)
	}
	at, err := s.DecryptAccessToken(acc)
	if err != nil || at != "at" {
		t.Fatalf("access: %v", err)
	}
	rt, err := s.DecryptRefreshToken(acc)
	if err != nil || rt != "rt" {
		t.Fatalf("refresh: %v", err)
	}
	accNoRef, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider:        "mastodon",
		AuthType:        domain.AccountAuthTypeOAuthToken,
		InstanceURL:     "https://m2.example",
		Username:        "u2",
		AccessToken:     "only",
		RefreshToken:    "",
	})
	if err != nil {
		t.Fatal(err)
	}
	rtEmpty, err := s.DecryptRefreshToken(accNoRef)
	if err != nil || rtEmpty != "" {
		t.Fatalf("empty refresh: %q %v", rtEmpty, err)
	}
	list, err := s.ListTeamAccounts(ctx, team.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListTeamAccounts: %v", err)
	}
	byIDs, err := s.GetAccountsByIDs(ctx, team.ID, []string{acc.ID})
	if err != nil || len(byIDs) != 1 {
		t.Fatalf("GetAccountsByIDs: %#v", byIDs)
	}
	emptyIDs, err := s.GetAccountsByIDs(ctx, team.ID, nil)
	if err != nil || emptyIDs != nil {
		t.Fatalf("empty ids: %#v %v", emptyIDs, err)
	}
	if err := s.DeleteAccount(ctx, team.ID, acc.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSQLite_ScheduledPostsLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "auth", "auth@x", "Auth")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "post-" + uuid.NewString(), Description: ""})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "x", AccessToken: "t",
	})
	principal := domain.AuthenticatedPrincipal{User: u, Kind: "oidc"}
	when := time.Now().UTC().Add(24 * time.Hour)
	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Title: "T", Content: "body", ScheduledAt: when, TargetAccounts: []string{acc.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(post.TargetAccounts) != 1 {
		t.Fatalf("targets: %#v", post.TargetAccounts)
	}
	got, err := s.GetScheduledPost(ctx, team.ID, post.ID)
	if err != nil || got.Content != "body" {
		t.Fatal(err)
	}
	list, err := s.ListTeamPosts(ctx, team.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListTeamPosts: %v", err)
	}
	updated, err := s.UpdateScheduledPost(ctx, team.ID, post.ID, domain.CreatePostInput{
		Title: "T2", Content: "new", ScheduledAt: when.Add(time.Hour), TargetAccounts: []string{acc.ID},
	})
	if err != nil || updated.Content != "new" {
		t.Fatal(err)
	}
	if err := s.CancelScheduledPost(ctx, team.ID, post.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteScheduledPost(ctx, team.ID, post.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSQLite_ListDuePosts_and_processingFlow(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "due", "due@x", "Due")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "due-" + uuid.NewString(), Description: ""})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "x", AccessToken: "t",
	})
	principal := domain.AuthenticatedPrincipal{User: u}
	past := time.Now().UTC().Add(-time.Minute)
	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content: "c", ScheduledAt: past, TargetAccounts: []string{acc.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	due, err := s.ListDuePosts(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, p := range due {
		if p.ID == post.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("due post not listed")
	}
	if err := s.MarkPostProcessing(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	next := time.Now().UTC().Add(30 * time.Minute)
	if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusFailed, "e", &next); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostTargetResult(ctx, post.ID, acc.ID, domain.PostStatusPosted, "https://u", ""); err != nil {
		t.Fatal(err)
	}
	targets, err := s.LoadPostTargets(ctx, post.ID)
	if err != nil || len(targets) != 1 {
		t.Fatalf("LoadPostTargets: %v", err)
	}
	links, err := s.LoadPublishedLinksByPostIDs(ctx, []string{post.ID})
	if err != nil || links[post.ID][acc.ID] != "https://u" {
		t.Fatalf("links: %#v %v", links, err)
	}
	emptyLinks, err := s.LoadPublishedLinksByPostIDs(ctx, nil)
	if err != nil || len(emptyLinks) != 0 {
		t.Fatalf("empty post ids links: %#v", emptyLinks)
	}
}

func TestSQLite_GetScheduledPost_notFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "nf", "nf@x", "Nf")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "nf-" + uuid.NewString(), Description: ""})
	_, err := s.GetScheduledPost(ctx, team.ID, uuid.NewString())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("want ErrNoRows, got %v", err)
	}
}
