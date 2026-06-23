package postvalidate

import (
	"context"
	"strings"
	"testing"

	"git.f4mily.net/goloom/internal/domain"
	"git.f4mily.net/goloom/internal/provider"
)

func testRegistry(t *testing.T) *provider.Registry {
	t.Helper()
	return provider.NewRegistry(
		provider.NewBlueskyProvider(),
		provider.NewMastodonProvider(provider.MastodonRegistrationConfig{}),
	)
}

func acc(id, prov string) domain.SocialAccount {
	return domain.SocialAccount{ID: id, Provider: prov, TeamID: "team-1", Username: id}
}

func TestCheck_AllWithinLimits(t *testing.T) {
	reg := testRegistry(t)
	accounts := []domain.SocialAccount{acc("bsky", "bluesky"), acc("masto", "mastodon")}
	res, err := Check(context.Background(), reg, accounts, domain.CreatePostInput{
		Content:        "short",
		TargetAccounts: []string{"bsky", "masto"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid {
		t.Fatalf("expected valid, got %+v", res)
	}
	if res.MaxChars != 300 {
		t.Errorf("MaxChars = %d, want 300 (min across bluesky/mastodon)", res.MaxChars)
	}
}

func TestCheck_ExceedsBlueskyButOverrideFits(t *testing.T) {
	reg := testRegistry(t)
	accounts := []domain.SocialAccount{acc("bsky", "bluesky"), acc("masto", "mastodon")}
	long := strings.Repeat("x", 400) // > 300 (bluesky) but < 500 (mastodon)

	// Without override: bluesky invalid.
	res, err := Check(context.Background(), reg, accounts, domain.CreatePostInput{
		Content:        long,
		TargetAccounts: []string{"bsky", "masto"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected invalid: bluesky limit exceeded")
	}
	if !strings.Contains(res.Problems(), "bsky") {
		t.Errorf("Problems() should name the offending account: %q", res.Problems())
	}

	// With a bluesky-specific override that fits: valid.
	res, err = Check(context.Background(), reg, accounts, domain.CreatePostInput{
		Content:                long,
		TargetAccounts:         []string{"bsky", "masto"},
		AccountContentOverride: map[string]string{"bsky": "short bluesky text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid {
		t.Fatalf("expected valid with override, got problems: %q", res.Problems())
	}
}

func TestCheck_UnsupportedProvider(t *testing.T) {
	reg := testRegistry(t)
	accounts := []domain.SocialAccount{acc("x", "myspace")}
	if _, err := Check(context.Background(), reg, accounts, domain.CreatePostInput{Content: "hi", TargetAccounts: []string{"x"}}); err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestCheckLimits_PreResolvedLimits(t *testing.T) {
	limits := []AccountLimit{
		{AccountID: "bsky", Username: "team.bsky", Provider: "bluesky", MaxChars: 300},
		{AccountID: "masto", Username: "team", Provider: "mastodon", MaxChars: 500},
	}
	long := strings.Repeat("x", 400)

	res := CheckLimits(long, nil, limits)
	if res.Valid {
		t.Fatal("expected invalid: bluesky limit exceeded")
	}
	if res.MaxChars != 300 {
		t.Errorf("MaxChars = %d, want 300", res.MaxChars)
	}
	if !strings.Contains(res.Problems(), "bsky") || strings.Contains(res.Problems(), "masto") {
		t.Errorf("Problems() should name only the violating account: %q", res.Problems())
	}

	// Override that fits flips the result to valid.
	res = CheckLimits(long, map[string]string{"bsky": "short"}, limits)
	if !res.Valid {
		t.Fatalf("override within limit should be valid, got %q", res.Problems())
	}

	// A zero/unknown limit is treated as unlimited.
	res = CheckLimits(long, nil, []AccountLimit{{AccountID: "x", MaxChars: 0}})
	if !res.Valid {
		t.Fatal("zero limit must be treated as unlimited")
	}
}

func TestCheck_PerAccountOverrideStillTooLong(t *testing.T) {
	reg := testRegistry(t)
	accounts := []domain.SocialAccount{acc("bsky", "bluesky")}
	res, err := Check(context.Background(), reg, accounts, domain.CreatePostInput{
		Content:                "fine",
		TargetAccounts:         []string{"bsky"},
		AccountContentOverride: map[string]string{"bsky": strings.Repeat("y", 301)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected invalid: override itself exceeds bluesky limit")
	}
}
