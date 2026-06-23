package postservice

import (
	"context"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
)

type fakeAccounts map[string]domain.SocialAccount

func (f fakeAccounts) GetAccountsByIDsGlobal(_ context.Context, ids []string) ([]domain.SocialAccount, error) {
	var out []domain.SocialAccount
	for _, id := range ids {
		if a, ok := f[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func newService(accs ...domain.SocialAccount) *Service {
	store := fakeAccounts{}
	for _, a := range accs {
		store[a.ID] = a
	}
	reg := provider.NewRegistry(
		provider.NewBlueskyProvider(),
		provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}),
	)
	return New(store, reg)
}

func acc(id, prov, team string) domain.SocialAccount {
	return domain.SocialAccount{ID: id, Provider: prov, TeamID: team, Username: id}
}

func base(content string, targets ...string) domain.CreatePostInput {
	return domain.CreatePostInput{Title: "T", Content: content, TargetAccounts: targets}
}

func TestPrepare_ValidScheduled(t *testing.T) {
	s := newService(acc("bsky", "bluesky", "team-1"))
	res, err := s.Prepare(context.Background(), "team-1", base("short", "bsky"), Options{CheckLimits: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Validation.Valid || res.EffectiveTeam != "team-1" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestPrepare_RejectsShape(t *testing.T) {
	s := newService(acc("bsky", "bluesky", "team-1"))
	if _, err := s.Prepare(context.Background(), "team-1", base("", "bsky"), Options{CheckLimits: true}); err == nil {
		t.Fatal("empty content must be rejected")
	}
	noTitle := base("c", "bsky")
	noTitle.Title = "  "
	if _, err := s.Prepare(context.Background(), "team-1", noTitle, Options{CheckLimits: true}); err == nil {
		t.Fatal("missing title must be rejected")
	}
}

func TestPrepare_UnknownAndCrossTeamTargets(t *testing.T) {
	s := newService(acc("bsky", "bluesky", "team-1"), acc("foreign", "mastodon", "team-2"))
	if _, err := s.Prepare(context.Background(), "team-1", base("c", "ghost"), Options{}); err == nil {
		t.Fatal("unknown target must be rejected")
	}
	_, err := s.Prepare(context.Background(), "team-1", base("c", "foreign"), Options{})
	if err == nil || !strings.Contains(err.Error(), "one team") && !strings.Contains(err.Error(), "belong") {
		t.Fatalf("cross-team target must be rejected, got %v", err)
	}
}

func TestPrepare_OverrideKeyMismatch(t *testing.T) {
	s := newService(acc("bsky", "bluesky", "team-1"))
	in := base("c", "bsky")
	in.AccountContentOverride = map[string]string{"other": "x"}
	if _, err := s.Prepare(context.Background(), "team-1", in, Options{}); err == nil {
		t.Fatal("override for a non-target account must be rejected")
	}
}

func TestPrepare_CharLimitsReportedNotErrored(t *testing.T) {
	s := newService(acc("bsky", "bluesky", "team-1"))
	res, err := s.Prepare(context.Background(), "team-1", base(strings.Repeat("x", 400), "bsky"), Options{CheckLimits: true})
	if err != nil {
		t.Fatalf("oversize is a soft result, not a hard error: %v", err)
	}
	if res.Validation.Valid {
		t.Fatal("expected Validation.Valid=false for oversize content")
	}
	if ValidationError(res.Validation) == nil {
		t.Fatal("ValidationError must turn an invalid result into an error")
	}
}

func TestPrepare_DraftSkipsCharLimits(t *testing.T) {
	s := newService(acc("bsky", "bluesky", "team-1"))
	in := base(strings.Repeat("x", 400), "bsky")
	in.Draft = true
	res, err := s.Prepare(context.Background(), "team-1", in, Options{CheckLimits: false})
	if err != nil {
		t.Fatalf("oversized draft must be allowed: %v", err)
	}
	if !res.Validation.Valid {
		t.Fatal("draft validation should default valid (limits skipped)")
	}
}

func TestPrepare_OverrideFlipsValidity(t *testing.T) {
	s := newService(acc("bsky", "bluesky", "team-1"))
	in := base(strings.Repeat("x", 400), "bsky")
	in.AccountContentOverride = map[string]string{"bsky": "short"}
	res, err := s.Prepare(context.Background(), "team-1", in, Options{CheckLimits: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Validation.Valid {
		t.Fatalf("valid override should make it valid: %q", res.Validation.Problems())
	}
	if !res.Input.UseVersions {
		t.Error("UseVersions should be derived from the surviving override")
	}
}
