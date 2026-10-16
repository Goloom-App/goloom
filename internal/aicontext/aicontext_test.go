package aicontext_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/aicontext"
	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/security"
	"git.f4mily.net/goloom/internal/store/sqlite"
	"github.com/google/uuid"
)

func newStore(t *testing.T) *sqlite.Store {
	t.Helper()
	enc, err := security.NewEncrypter("aicontext-test-secret-32-bytes-ok!")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.New(context.Background(), "file:"+uuid.NewString()+"?mode=memory&cache=shared", enc)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTeam(t *testing.T, s *sqlite.Store) (domain.User, domain.Team) {
	t.Helper()
	ctx := context.Background()
	u, err := s.UpsertOIDCUser(ctx, "ctx-"+uuid.NewString(), "ctx@test.local", "Ctx")
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.CreateTeam(ctx, u.ID, domain.CreateTeamInput{Name: "ctx-" + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	return u, team
}

func TestBuild_teamWithoutProfileIsWellFormed(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	_, team := newTeam(t, s)

	for name, opts := range map[string]aicontext.Options{"full": aicontext.Full(), "chat": aicontext.Chat(), "zero": {}} {
		got, err := aicontext.Build(ctx, s, team.ID, opts)
		if err != nil {
			t.Fatalf("%s: Build: %v", name, err)
		}
		if got.Team.ID != team.ID {
			t.Fatalf("%s: team id = %q", name, got.Team.ID)
		}
		if got.Profile != nil {
			t.Fatalf("%s: Profile should be nil without brand profile", name)
		}
		if got.CampaignFormats == nil || got.StyleExamples == nil || got.KnowledgeSources == nil ||
			got.RecentPosts == nil || got.UpcomingPosts == nil || got.Accounts == nil ||
			got.EngagementHours == nil || got.TopHashtags == nil {
			t.Fatalf("%s: slices must be empty, not nil: %+v", name, got)
		}
		if _, err := json.Marshal(got); err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
	}
}

func TestBuild_unknownTeamErrors(t *testing.T) {
	s := newStore(t)
	if _, err := aicontext.Build(context.Background(), s, uuid.NewString(), aicontext.Full()); err == nil {
		t.Fatal("expected error for unknown team")
	}
}

func TestBuild_optionsSelectParts(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	u, team := newTeam(t, s)

	if _, err := s.CreateTeamProfile(ctx, team.ID, domain.TeamProfile{
		TeamID: team.ID, StyleMetadata: domain.StyleMetadata{Tonality: "casual"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateStyleExample(ctx, team.ID, domain.StyleExample{Platform: "mastodon", Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateKnowledgeSource(ctx, team.ID, domain.KnowledgeSource{
		Type: domain.KnowledgeSourceText, Name: "About", Content: "text",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCampaignFormat(ctx, team.ID, domain.CampaignFormat{
		Name: "fmt", Structure: json.RawMessage(`{"type":"single"}`), IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	acc, err := s.CreateAccount(ctx, team.ID, domain.ConnectedAccount{
		Provider: "mastodon", AuthType: domain.AccountAuthTypeOAuthToken,
		InstanceURL: "https://m.example", Username: "ctxu", AccessToken: "tok",
	})
	if err != nil {
		t.Fatal(err)
	}
	principal := domain.AuthenticatedPrincipal{User: u}
	past, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content: "published", ScheduledAt: time.Now().UTC().Add(-time.Hour), TargetAccounts: []string{acc.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPostResult(ctx, past.ID, 1, domain.PostStatusPosted, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateScheduledPost(ctx, team.ID, principal, domain.CreatePostInput{
		Content: "upcoming", ScheduledAt: time.Now().UTC().Add(time.Hour), TargetAccounts: []string{acc.ID},
	}); err != nil {
		t.Fatal(err)
	}

	full, err := aicontext.Build(ctx, s, team.ID, aicontext.Full())
	if err != nil {
		t.Fatal(err)
	}
	if full.Profile == nil || full.Profile.StyleMetadata.Tonality != "casual" {
		t.Fatalf("profile = %+v", full.Profile)
	}
	if len(full.StyleExamples) != 1 || len(full.KnowledgeSources) != 1 || len(full.CampaignFormats) != 1 || len(full.Accounts) != 1 {
		t.Fatalf("brand data missing: %+v", full)
	}
	if len(full.RecentPosts) != 1 || len(full.UpcomingPosts) != 1 {
		t.Fatalf("posts: recent=%d upcoming=%d", len(full.RecentPosts), len(full.UpcomingPosts))
	}
	if got := full.RecentPosts[0].TargetAccounts; len(got) != 1 || got[0] != acc.ID {
		t.Fatalf("recent post targets = %v", got)
	}
	if got := full.UpcomingPosts[0].TargetAccounts; len(got) != 1 || got[0] != acc.ID {
		t.Fatalf("upcoming post targets = %v", got)
	}

	chat, err := aicontext.Build(ctx, s, team.ID, aicontext.Chat())
	if err != nil {
		t.Fatal(err)
	}
	if chat.Profile == nil || len(chat.Accounts) != 1 || len(chat.CampaignFormats) != 1 || len(chat.StyleExamples) != 1 || len(chat.KnowledgeSources) != 1 {
		t.Fatalf("chat must keep brand/accounts/formats: %+v", chat)
	}
	if len(chat.RecentPosts) != 0 || len(chat.UpcomingPosts) != 0 || len(chat.EngagementHours) != 0 {
		t.Fatalf("chat must skip posts and engagement: %+v", chat)
	}
}

type failingSource struct {
	aicontext.Source
	err error
}

func (f failingSource) ListRecentPostedPosts(context.Context, string, int) ([]domain.ScheduledPost, error) {
	return nil, f.err
}

func TestBuild_propagatesPostErrorsOnlyWhenRequested(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	_, team := newTeam(t, s)
	boom := errors.New("boom")
	src := failingSource{Source: s, err: boom}

	if _, err := aicontext.Build(ctx, src, team.ID, aicontext.Full()); !errors.Is(err, boom) {
		t.Fatalf("Full: want boom, got %v", err)
	}
	if _, err := aicontext.Build(ctx, src, team.ID, aicontext.Chat()); err != nil {
		t.Fatalf("Chat must not touch posts: %v", err)
	}
}
