package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func postgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_URL to run postgres integration tests")
	}
	return dsn
}

func newTestStore(t *testing.T) *postgres.Store {
	t.Helper()
	enc, err := security.NewEncrypter("postgres-integration-test-secret-32b")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, err := postgres.New(ctx, postgresDSN(t), enc)
	if err != nil {
		t.Fatalf("postgres.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPostgres_UpsertOIDCUser_firstIsAdmin(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u1, err := s.UpsertOIDCUser(ctx, "sub-pg-"+uuid.NewString(), "a@pg.test", "A")
	if err != nil {
		t.Fatal(err)
	}
	if !u1.IsAdmin {
		t.Fatal("first user should be admin")
	}
	u2, err := s.UpsertOIDCUser(ctx, "sub-pg-"+uuid.NewString(), "b@pg.test", "B")
	if err != nil {
		t.Fatal(err)
	}
	if u2.IsAdmin {
		t.Fatal("second user should not be admin by default")
	}
}

func TestPostgres_UpsertOIDCUser_updatesExisting(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	sub := "sub-pg-" + uuid.NewString()
	if _, err := s.UpsertOIDCUser(ctx, sub, "old@pg.test", "Old"); err != nil {
		t.Fatal(err)
	}
	u, err := s.UpsertOIDCUser(ctx, sub, "new@pg.test", "New")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "new@pg.test" || u.Name != "New" {
		t.Fatalf("user: %+v", u)
	}
}

func TestPostgres_EnsureBootstrapAdmin_and_LookupAPIToken(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	token := "bootstrap-pg-token-" + uuid.NewString()
	if err := s.EnsureBootstrapAdmin(ctx, "admin-pg@local", "AdminPG", token); err != nil {
		t.Fatal(err)
	}
	p, err := s.LookupAPIToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "api_token" || p.User.Email != "admin-pg@local" || !p.User.IsAdmin {
		t.Fatalf("principal: %+v", p)
	}
	if _, err := s.LookupAPIToken(ctx, "wrong-token-"+uuid.NewString()); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("lookup wrong token: %v", err)
	}
}

func TestPostgres_ListUsers_SetUserAdmin(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	if _, err := s.UpsertOIDCUser(ctx, "lu1-"+uuid.NewString(), "lu1@pg.test", "LU1"); err != nil {
		t.Fatal(err)
	}
	u1, err := s.UpsertOIDCUser(ctx, "lu2-"+uuid.NewString(), "lu2@pg.test", "LU2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListUsers(ctx); err != nil {
		t.Fatal(err)
	}
	updated, err := s.SetUserAdmin(ctx, u1.ID, true)
	if err != nil || !updated.IsAdmin {
		t.Fatalf("SetUserAdmin: %+v %v", updated, err)
	}
}

func TestPostgres_TeamsAndMemberships(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	owner, err := s.UpsertOIDCUser(ctx, "own-"+uuid.NewString(), "own@pg.test", "Own")
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.UpsertOIDCUser(ctx, "mem-"+uuid.NewString(), "mem@pg.test", "Mem")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, owner.ID, domain.CreateTeamInput{
		Name:        "t-pg-" + uuid.NewString(),
		Description: "d",
	})
	if err != nil {
		t.Fatal(err)
	}
	teams, err := s.ListTeamsForUser(ctx, owner.ID, false)
	if err != nil || len(teams) != 1 {
		t.Fatalf("owner teams: %v %#v", err, teams)
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
		t.Fatalf("UserHasAnyTeamRole: ok=%v err=%v", ok, err)
	}
	if err := s.RemoveTeamMember(ctx, team.ID, member.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPostgres_ProviderInstances(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "pi-"+uuid.NewString(), "pi@pg.test", "PI")
	url := "https://social-" + uuid.NewString() + ".example"
	input := domain.PreparedProviderInstance{
		Provider:              "mastodon",
		Name:                  "inst-" + uuid.NewString(),
		InstanceURL:           url,
		ClientID:              "cid",
		ClientSecret:          "secret",
		Scopes:                []string{"read"},
		AuthorizationEndpoint: url + "/oauth/authorize",
		TokenEndpoint:         url + "/oauth/token",
	}
	created, err := s.CreateProviderInstance(ctx, u.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := s.DecryptProviderInstanceClientSecret(created)
	if err != nil || plain != "secret" {
		t.Fatalf("decrypt: %q %v", plain, err)
	}
	if _, err := s.ListProviderInstances(ctx, "mastodon"); err != nil {
		t.Fatal(err)
	}
	byID, err := s.GetProviderInstanceByID(ctx, created.ID)
	if err != nil || byID.ID != created.ID {
		t.Fatal(err)
	}
	updated, err := s.UpdateProviderInstance(ctx, created.ID, domain.PreparedProviderInstance{
		Provider:    "mastodon",
		Name:        "renamed-pg",
		InstanceURL: url,
		ClientID:    "cid2",
	})
	if err != nil || updated.Name != "renamed-pg" {
		t.Fatalf("update: %+v", updated)
	}
}

func TestPostgres_SocialAccounts(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "sa-"+uuid.NewString(), "sa@pg.test", "SA")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "team-pg-" + uuid.NewString(), Description: ""})
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m-pg.example", Username: "u", RemoteAccountID: "1",
		AccessToken: "at", RefreshToken: "rt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAccountsByIDs(ctx, team.ID, []string{acc.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListTeamAccounts(ctx, team.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccount(ctx, team.ID, acc.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPostgres_ScheduledPosts_and_due(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "sp-"+uuid.NewString(), "sp@pg.test", "SP")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "sp-team-" + uuid.NewString(), Description: ""})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "x", AccessToken: "t",
	})
	principal := domain.AuthenticatedPrincipal{User: u}
	past := time.Now().UTC().Add(-2 * time.Minute)
	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content: "body", ScheduledAt: past, TargetAccounts: []string{acc.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	due, err := s.ListDuePosts(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range due {
		if p.ID == post.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("post should be due")
	}
	if err := s.MarkPostProcessing(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	next := time.Now().UTC().Add(15 * time.Minute)
	if err := s.MarkPostResult(ctx, post.ID, 1, domain.PostStatusFailed, "err", &next); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostTargetResult(ctx, post.ID, acc.ID, domain.PostStatusPosted, "https://post", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadPostTargets(ctx, post.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadPublishedLinksByPostIDs(ctx, []string{post.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelScheduledPost(ctx, team.ID, post.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteScheduledPost(ctx, team.ID, post.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPostgres_GetProviderInstanceByID_notFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	_, err := s.GetProviderInstanceByID(ctx, uuid.NewString())
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("want pgx.ErrNoRows, got %v", err)
	}
}

func TestPostgres_GetScheduledPost_notFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "nf-"+uuid.NewString(), "nf@pg.test", "NF")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "nf-" + uuid.NewString(), Description: ""})
	_, err := s.GetScheduledPost(ctx, team.ID, uuid.NewString())
	if err == nil || err.Error() != "post not found" {
		t.Fatalf("want post not found, got %v", err)
	}
}
