package api

import "testing"

func TestIsMastodonCompatibleOAuthProvider(t *testing.T) {
	cases := map[string]bool{
		"mastodon":  true,
		"Mastodon":  true,
		" pixelfed": true,
		"pixelfed":  true,
		"bluesky":   false,
		"friendica": false,
		"":          false,
	}
	for name, want := range cases {
		if got := isMastodonCompatibleOAuthProvider(name); got != want {
			t.Errorf("isMastodonCompatibleOAuthProvider(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestTitleCaseProvider(t *testing.T) {
	if got := titleCaseProvider("pixelfed"); got != "Pixelfed" {
		t.Errorf("titleCaseProvider(pixelfed) = %q, want Pixelfed", got)
	}
	if got := titleCaseProvider(""); got != "" {
		t.Errorf("titleCaseProvider(empty) = %q, want empty", got)
	}
}
