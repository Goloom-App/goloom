package textcount

import (
	"strings"
	"testing"
)

func TestGraphemes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"ascii", "hello", 5},
		{"umlauts", "grüße", 5},
		{"emoji", "🎉", 1},
		{"zwj family emoji", "👨‍👩‍👧‍👦", 1},
		{"flag", "🇩🇪", 1},
		{"combining mark", "é", 1},
		{"mixed", "hi 🎉👨‍👩‍👧‍👦", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Graphemes(tc.in); got != tc.want {
				t.Fatalf("Graphemes(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestProviderLength_Mastodon(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"plain text", "hello world", 11},
		// Every URL counts as 23 characters, regardless of its real length.
		{"long url", "https://example.com/" + strings.Repeat("x", 200), 23},
		{"short url", "https://a.io", 23},
		{"http url", "http://example.com", 23},
		{"url with text", "look at this: https://example.com/very/long/path?with=query&params=true", 14 + 23},
		{"two urls", "https://one.example https://two.example/" + strings.Repeat("y", 100), 23 + 1 + 23},
		// Remote mentions count only the local part.
		{"remote mention", "hi @user@mastodon.example!", 3 + 5 + 1},
		{"local mention", "hi @user!", 9},
		// Email addresses are not mentions.
		{"email", "mail me at user@example.com", 27},
		// Graphemes, not runes or bytes.
		{"emoji with url", "🎉👨‍👩‍👧‍👦 https://example.com/party", 2 + 1 + 23},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProviderLength("mastodon", tc.in); got != tc.want {
				t.Fatalf("ProviderLength(mastodon, %q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestProviderLength_OtherProviders(t *testing.T) {
	longURL := "https://example.com/" + strings.Repeat("x", 30) // 50 chars
	// Bluesky, Friendica and Pixelfed count URLs at their full length…
	for _, provider := range []string{"bluesky", "friendica", "pixelfed"} {
		if got := ProviderLength(provider, longURL); got != 50 {
			t.Fatalf("ProviderLength(%s, longURL) = %d, want 50", provider, got)
		}
	}
	// …but in graphemes, so emoji sequences count as one character.
	if got := ProviderLength("bluesky", "👨‍👩‍👧‍👦!"); got != 2 {
		t.Fatalf("ProviderLength(bluesky, family emoji) = %d, want 2", got)
	}
	// Unknown providers fall back to plain grapheme counting.
	if got := ProviderLength("", "hello 🎉"); got != 7 {
		t.Fatalf("ProviderLength(unknown) = %d, want 7", got)
	}
	// Provider matching is case-insensitive (mirrors MaxCharsForProvider).
	if got := ProviderLength("Mastodon", "https://example.com/"+strings.Repeat("x", 100)); got != 23 {
		t.Fatalf("ProviderLength(Mastodon, longURL) = %d, want 23", got)
	}
}
