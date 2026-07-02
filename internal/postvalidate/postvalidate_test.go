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

func TestCheck_MastodonCountsURLsAsFixedLength(t *testing.T) {
	reg := testRegistry(t)
	accounts := []domain.SocialAccount{acc("masto", "mastodon")}
	// 450 plain characters plus a 200-character URL: raw length is far over
	// Mastodon's 500-char limit, but Mastodon counts the URL as 23 characters
	// (450 + 1 + 23 = 474), so the post is valid.
	content := strings.Repeat("a", 450) + " https://example.com/" + strings.Repeat("x", 179)

	res, err := Check(context.Background(), reg, accounts, domain.CreatePostInput{
		Content:        content,
		TargetAccounts: []string{"masto"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid {
		t.Fatalf("expected valid (Mastodon counts URLs as 23 chars), got %q", res.Problems())
	}
	if got := res.Destinations[0].Length; got != 474 {
		t.Errorf("mastodon destination length = %d, want 474", got)
	}
}

func TestCheck_BlueskyCountsGraphemesAndFullURLs(t *testing.T) {
	reg := testRegistry(t)
	accounts := []domain.SocialAccount{acc("bsky", "bluesky")}

	// 299 characters + one multi-rune emoji: exactly 300 graphemes, valid.
	content := strings.Repeat("a", 299) + "👨‍👩‍👧‍👦"
	res, err := Check(context.Background(), reg, accounts, domain.CreatePostInput{
		Content:        content,
		TargetAccounts: []string{"bsky"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid {
		t.Fatalf("expected valid (300 graphemes), got %q with length %d", res.Problems(), res.Destinations[0].Length)
	}

	// Bluesky gives URLs no discount: 290 chars + 23-char URL = 313 > 300.
	content = strings.Repeat("a", 290) + "https://example.com/xyz"
	res, err = Check(context.Background(), reg, accounts, domain.CreatePostInput{
		Content:        content,
		TargetAccounts: []string{"bsky"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Valid {
		t.Fatal("expected invalid: bluesky counts URLs at full length")
	}
}

func TestCheckLimits_ProviderAwareCounting(t *testing.T) {
	// 470 plain chars + space + long URL: 470 + 1 + 23 = 494 for Mastodon.
	content := strings.Repeat("a", 470) + " https://example.com/" + strings.Repeat("x", 100)
	limits := []AccountLimit{{AccountID: "masto", Provider: "mastodon", MaxChars: 500}}
	if res := CheckLimits(content, nil, limits); !res.Valid {
		t.Fatalf("expected valid (Mastodon URL discount), got %q", res.Problems())
	}
	// The same text on a 500-char provider without URL discount is invalid.
	limits = []AccountLimit{{AccountID: "pixel", Provider: "pixelfed", MaxChars: 500}}
	if res := CheckLimits(content, nil, limits); res.Valid {
		t.Fatal("expected invalid: pixelfed counts URLs at full length")
	}
}
