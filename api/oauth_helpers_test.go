package api

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/config"
)

// testOAuthAPI builds a minimal *API sufficient for testing oauth helper methods.
// It has a fixed 32-byte encryption key, a known public base URL, and one
// additional allowed origin.
func testOAuthAPI() *API {
	return &API{
		config: config.Config{
			EncryptionKey:  "test-oauth-key-32-bytes-exactly!",
			PublicBaseURL:  "https://app.example.test",
			AllowedOrigins: []string{"https://frontend.example.test"},
		},
	}
}

func validMastodonState(expiresIn time.Duration) mastodonOAuthState {
	return mastodonOAuthState{
		Version:            1,
		Provider:           "mastodon",
		UserID:             "user-1",
		TeamID:             "team-1",
		ProviderInstanceID: "inst-1",
		ReturnTo:           "https://app.example.test/settings",
		ExpiresAtUnix:      time.Now().UTC().Add(expiresIn).Unix(),
	}
}

// ── signMastodonOAuthState / parseMastodonOAuthState ──────────────────────────

func TestSignParseMastodonOAuthState_Roundtrip(t *testing.T) {
	a := testOAuthAPI()
	state := validMastodonState(10 * time.Minute)

	signed, err := a.signMastodonOAuthState(state)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if strings.Count(signed, ".") != 1 {
		t.Fatalf("signed state must have exactly one dot separator, got %q", signed)
	}

	parsed, err := a.parseMastodonOAuthState(signed)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.UserID != state.UserID {
		t.Errorf("UserID: got %q, want %q", parsed.UserID, state.UserID)
	}
	if parsed.TeamID != state.TeamID {
		t.Errorf("TeamID: got %q, want %q", parsed.TeamID, state.TeamID)
	}
	if parsed.Provider != state.Provider {
		t.Errorf("Provider: got %q, want %q", parsed.Provider, state.Provider)
	}
	if parsed.ProviderInstanceID != state.ProviderInstanceID {
		t.Errorf("ProviderInstanceID: got %q, want %q", parsed.ProviderInstanceID, state.ProviderInstanceID)
	}
	if parsed.ReturnTo != state.ReturnTo {
		t.Errorf("ReturnTo: got %q, want %q", parsed.ReturnTo, state.ReturnTo)
	}
}

func TestParseMastodonOAuthState_WrongNumberOfParts(t *testing.T) {
	a := testOAuthAPI()
	cases := []string{
		"nodotatall",
		"a.b.c",
		"",
		".",
	}
	for _, raw := range cases {
		if _, err := a.parseMastodonOAuthState(raw); err == nil {
			t.Errorf("parseMastodonOAuthState(%q): expected error, got nil", raw)
		}
	}
}

func TestParseMastodonOAuthState_SignatureMismatch(t *testing.T) {
	a := testOAuthAPI()
	signed, _ := a.signMastodonOAuthState(validMastodonState(10 * time.Minute))

	parts := strings.SplitN(signed, ".", 2)
	// Replace signature with all-zero bytes (valid base64 but wrong HMAC).
	wrongSig := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	mangled := parts[0] + "." + wrongSig

	if _, err := a.parseMastodonOAuthState(mangled); err == nil {
		t.Error("expected error for wrong signature, got nil")
	}
}

func TestParseMastodonOAuthState_InvalidBase64Signature(t *testing.T) {
	a := testOAuthAPI()
	signed, _ := a.signMastodonOAuthState(validMastodonState(10 * time.Minute))
	parts := strings.SplitN(signed, ".", 2)
	// "!!!!" is not valid base64.
	mangled := parts[0] + ".!!!!"
	if _, err := a.parseMastodonOAuthState(mangled); err == nil {
		t.Error("expected error for invalid base64 signature")
	}
}

func TestParseMastodonOAuthState_Expired(t *testing.T) {
	a := testOAuthAPI()
	state := validMastodonState(-1 * time.Hour) // already expired
	signed, err := a.signMastodonOAuthState(state)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := a.parseMastodonOAuthState(signed); err == nil {
		t.Error("expected error for expired state")
	}
}

func TestParseMastodonOAuthState_IncompleteFields(t *testing.T) {
	a := testOAuthAPI()
	incomplete := mastodonOAuthState{
		Version:            1,
		Provider:           "mastodon",
		UserID:             "", // missing
		TeamID:             "t1",
		ProviderInstanceID: "i1",
		ReturnTo:           "https://app.example.test/",
		ExpiresAtUnix:      time.Now().UTC().Add(10 * time.Minute).Unix(),
	}
	signed, err := a.signMastodonOAuthState(incomplete)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := a.parseMastodonOAuthState(signed); err == nil {
		t.Error("expected error for incomplete state (missing UserID)")
	}
}

func TestParseMastodonOAuthState_UnsupportedProvider(t *testing.T) {
	a := testOAuthAPI()
	bad := mastodonOAuthState{
		Version:            1,
		Provider:           "bluesky", // not mastodon-compatible
		UserID:             "u1",
		TeamID:             "t1",
		ProviderInstanceID: "i1",
		ReturnTo:           "https://app.example.test/",
		ExpiresAtUnix:      time.Now().UTC().Add(10 * time.Minute).Unix(),
	}
	signed, err := a.signMastodonOAuthState(bad)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := a.parseMastodonOAuthState(signed); err == nil {
		t.Error("expected error for unsupported provider bluesky")
	}
}

func TestParseMastodonOAuthState_PixelfedAllowed(t *testing.T) {
	a := testOAuthAPI()
	state := mastodonOAuthState{
		Version:            1,
		Provider:           "pixelfed",
		UserID:             "u1",
		TeamID:             "t1",
		ProviderInstanceID: "i1",
		ReturnTo:           "https://app.example.test/",
		ExpiresAtUnix:      time.Now().UTC().Add(10 * time.Minute).Unix(),
	}
	signed, _ := a.signMastodonOAuthState(state)
	parsed, err := a.parseMastodonOAuthState(signed)
	if err != nil {
		t.Fatalf("pixelfed should be allowed: %v", err)
	}
	if parsed.Provider != "pixelfed" {
		t.Errorf("provider = %q, want pixelfed", parsed.Provider)
	}
}

// ── normalizeOAuthReturnURL ───────────────────────────────────────────────────

func TestNormalizeOAuthReturnURL(t *testing.T) {
	a := testOAuthAPI()

	t.Run("EmptyFallsBackToPublicBaseURL", func(t *testing.T) {
		got, err := a.normalizeOAuthReturnURL("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should be the public base URL (trailing slash normalized)
		if !strings.HasPrefix(got, "https://app.example.test") {
			t.Errorf("got %q, expected app.example.test prefix", got)
		}
	})

	t.Run("ValidAllowedOrigin", func(t *testing.T) {
		got, err := a.normalizeOAuthReturnURL("https://frontend.example.test/callback")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "https://frontend.example.test/callback" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("ValidPublicBaseURLOrigin", func(t *testing.T) {
		got, err := a.normalizeOAuthReturnURL("https://app.example.test/oauth/done")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "https://app.example.test/oauth/done" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("FragmentStripped", func(t *testing.T) {
		got, err := a.normalizeOAuthReturnURL("https://app.example.test/path#hash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(got, "#") {
			t.Errorf("fragment should be stripped, got %q", got)
		}
	})

	t.Run("NonHTTPSchemeRejected", func(t *testing.T) {
		_, err := a.normalizeOAuthReturnURL("ftp://frontend.example.test/callback")
		if err == nil {
			t.Error("expected error for ftp scheme")
		}
	})

	t.Run("NoHostRejected", func(t *testing.T) {
		_, err := a.normalizeOAuthReturnURL("https://")
		if err == nil {
			t.Error("expected error for URL with no host")
		}
	})

	t.Run("ForeignOriginRejected", func(t *testing.T) {
		_, err := a.normalizeOAuthReturnURL("https://evil.example.test/steal")
		if err == nil {
			t.Error("expected error for disallowed origin")
		}
	})

	t.Run("InvalidURLRejected", func(t *testing.T) {
		_, err := a.normalizeOAuthReturnURL("not a url")
		if err == nil {
			t.Error("expected error for non-http scheme")
		}
	})
}

// ── isAllowedOAuthOrigin ──────────────────────────────────────────────────────

func TestIsAllowedOAuthOrigin(t *testing.T) {
	a := testOAuthAPI()

	t.Run("PublicBaseURLOriginAllowed", func(t *testing.T) {
		if !a.isAllowedOAuthOrigin("https://app.example.test") {
			t.Error("public base URL origin should be allowed")
		}
	})

	t.Run("ExplicitAllowedOrigin", func(t *testing.T) {
		if !a.isAllowedOAuthOrigin("https://frontend.example.test") {
			t.Error("explicit allowed origin should be allowed")
		}
	})

	t.Run("WildcardAllowsAll", func(t *testing.T) {
		aWild := &API{
			config: config.Config{
				EncryptionKey:  "test-oauth-key-32-bytes-exactly!",
				AllowedOrigins: []string{"*"},
			},
		}
		if !aWild.isAllowedOAuthOrigin("https://anything.example.test") {
			t.Error("wildcard should allow any origin")
		}
	})

	t.Run("EmptyOriginRejected", func(t *testing.T) {
		if a.isAllowedOAuthOrigin("") {
			t.Error("empty origin should not be allowed")
		}
	})

	t.Run("WhitespaceOnlyRejected", func(t *testing.T) {
		if a.isAllowedOAuthOrigin("   ") {
			t.Error("whitespace-only origin should not be allowed")
		}
	})

	t.Run("UnknownOriginRejected", func(t *testing.T) {
		if a.isAllowedOAuthOrigin("https://unknown.example.test") {
			t.Error("unknown origin should not be allowed")
		}
	})
}

// ── appendQueryParams ─────────────────────────────────────────────────────────

func TestAppendQueryParams(t *testing.T) {
	got, err := appendQueryParams("https://example.test/callback", map[string]string{
		"status":  "success",
		"message": "all good",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("result is not a valid URL: %v", err)
	}
	q := u.Query()
	if q.Get("status") != "success" {
		t.Errorf("status param: got %q", q.Get("status"))
	}
	if q.Get("message") != "all good" {
		t.Errorf("message param: got %q", q.Get("message"))
	}
}

func TestAppendQueryParams_PreservesExistingParams(t *testing.T) {
	got, err := appendQueryParams("https://example.test/cb?existing=1", map[string]string{"new": "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u, _ := url.Parse(got)
	if u.Query().Get("existing") != "1" {
		t.Errorf("existing param lost: %q", got)
	}
	if u.Query().Get("new") != "2" {
		t.Errorf("new param missing: %q", got)
	}
}

func TestAppendQueryParams_InvalidURL(t *testing.T) {
	_, err := appendQueryParams("://bad-url", map[string]string{"k": "v"})
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

// ── oauthOriginForURL ─────────────────────────────────────────────────────────

func TestOAuthOriginForURL(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"https://mastodon.social/timeline", "https://mastodon.social"},
		{"http://social.example.org:8080/api", "http://social.example.org:8080"},
		{"", ""},
		{"/relative-path", ""},
		{"https://", ""},
		{"not-a-url", ""},
	}
	for _, tc := range cases {
		got := oauthOriginForURL(tc.raw)
		if got != tc.want {
			t.Errorf("oauthOriginForURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}
