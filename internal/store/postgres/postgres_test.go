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

func truncateUsers(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, postgresDSN(t))
	if err != nil {
		t.Fatalf("connect for truncate: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "truncate table users cascade"); err != nil {
		t.Fatalf("truncate users: %v", err)
	}
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
	// "First user" semantics need an empty users table; earlier tests in this
	// package share the database, so reset it for this test.
	truncateUsers(t)
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
	if err != nil || len(teams) != 2 {
		t.Fatalf("owner teams (personal + shared): %v %#v", err, teams)
	}
	if !teams[0].IsPersonal || teams[0].PersonalForUserID != owner.ID {
		t.Fatalf("expected personal workspace first: %#v", teams[0])
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
		t.Fatalf("update: %+v err=%v", updated, err)
	}
	plainAfter, err := s.DecryptProviderInstanceClientSecret(updated)
	if err != nil || plainAfter != "secret" {
		t.Fatalf("oauth client secret must be preserved when update omits ClientSecret: %q %v", plainAfter, err)
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
	if err := s.MarkPostTargetResult(ctx, post.ID, acc.ID, domain.PostStatusPosted, "https://post", "", nil, ""); err != nil {
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

func TestPostgres_UpdateMediaItemFilename(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "medren-"+uuid.NewString(), "medren@pg.test", "Medren")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "medren-" + uuid.NewString()})
	created, err := s.CreateMediaItem(ctx, domain.MediaItem{
		TeamID: team.ID, Sha256: "ren-" + uuid.NewString(), Filename: "old.png", MimeType: "image/png", SizeBytes: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := s.UpdateMediaItemFilename(ctx, team.ID, created.ID, "new-name.png")
	if err != nil || updated.Filename != "new-name.png" {
		t.Fatalf("rename: %+v err=%v", updated, err)
	}
	if _, err := s.UpdateMediaItemFilename(ctx, uuid.NewString(), created.ID, "x.png"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("rename across teams: want pgx.ErrNoRows, got %v", err)
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

func TestPostgres_GetScheduledPostByID_returnsFullPost(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u, _ := s.UpsertOIDCUser(ctx, "gspid-"+uuid.NewString(), "gspid@pg.test", "GSPID")
	team, _ := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "gspid-" + uuid.NewString(), Description: "desc"})
	acc, _ := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://x", Username: "x", AccessToken: "t",
	})
	principal := domain.AuthenticatedPrincipal{User: u}
	enabled := true
	tmpl, err := s.CreatePostTemplate(ctx, team.ID, principal, domain.CreatePostTemplateInput{
		Title:            "tmpl",
		Content:          "tmpl body {counter}",
		RecurrenceJSON:   `{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}`,
		TargetAccountIDs: []string{acc.ID},
		Enabled:          &enabled,
	})
	if err != nil {
		t.Fatalf("CreatePostTemplate: %v", err)
	}
	tmplCtr := 3
	post, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content: "test body", ScheduledAt: time.Now().UTC(),
		TargetAccounts:  []string{acc.ID},
		PostTemplateID:  &tmpl.ID,
		TemplateCounter: &tmplCtr,
	})
	if err != nil {
		t.Fatal(err)
	}

	// GetScheduledPostByID must return without scan-column mismatch.
	got, err := s.GetScheduledPostByID(ctx, post.ID)
	if err != nil {
		t.Fatalf("GetScheduledPostByID: %v", err)
	}
	if got.ID != post.ID {
		t.Fatalf("ID: got %q, want %q", got.ID, post.ID)
	}
	if got.TeamID != team.ID {
		t.Fatalf("TeamID: got %q, want %q", got.TeamID, team.ID)
	}
	if got.Content != "test body" {
		t.Fatalf("Content: got %q, want %q", got.Content, "test body")
	}
	if len(got.TargetAccounts) != 1 || got.TargetAccounts[0] != acc.ID {
		t.Fatalf("TargetAccounts: got %v, want [%q]", got.TargetAccounts, acc.ID)
	}
	if got.PostTemplateID == nil || *got.PostTemplateID != tmpl.ID {
		t.Fatalf("PostTemplateID: got %v, want %q", got.PostTemplateID, tmpl.ID)
	}
	if got.TemplateCounter == nil || *got.TemplateCounter != tmplCtr {
		t.Fatalf("TemplateCounter: got %v, want %d", got.TemplateCounter, tmplCtr)
	}

	// Also verify GetScheduledPost (by team+id) returns identical data.
	byTeam, err := s.GetScheduledPost(ctx, team.ID, post.ID)
	if err != nil {
		t.Fatalf("GetScheduledPost: %v", err)
	}
	if byTeam.ID != got.ID || byTeam.TemplateCounter == nil || *byTeam.TemplateCounter != tmplCtr {
		t.Fatalf("GetScheduledPost mismatch: %+v vs %+v", byTeam, got)
	}
}
