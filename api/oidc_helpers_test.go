package api

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.f4mily.net/goloom/internal/config"
)

// testOIDCAPI builds a minimal *API sufficient for testing OIDC login helper methods.
func testOIDCAPI() *API {
	return &API{
		config: config.Config{
			EncryptionKey:  "test-oidc-key-32-bytes-exactly!!",
			PublicBaseURL:  "https://app.example.test",
			AllowedOrigins: []string{"https://frontend.example.test"},
		},
	}
}

func validOIDCState(expiresIn time.Duration) oidcLoginState {
	return oidcLoginState{
		Version:       1,
		ReturnTo:      "https://app.example.test/",
		Nonce:         "nonce-value-123",
		PKCEVerifier:  "pkce-verifier-value",
		ExpiresAtUnix: time.Now().UTC().Add(expiresIn).Unix(),
	}
}

// ── signOIDCLoginState / parseOIDCLoginState ──────────────────────────────────

func TestSignParseOIDCLoginState_Roundtrip(t *testing.T) {
	a := testOIDCAPI()
	state := validOIDCState(10 * time.Minute)

	signed, err := a.signOIDCLoginState(state)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if strings.Count(signed, ".") != 1 {
		t.Fatalf("signed state must have exactly one dot, got %q", signed)
	}

	parsed, err := a.parseOIDCLoginState(signed)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.ReturnTo != state.ReturnTo {
		t.Errorf("ReturnTo: got %q, want %q", parsed.ReturnTo, state.ReturnTo)
	}
	if parsed.Nonce != state.Nonce {
		t.Errorf("Nonce: got %q, want %q", parsed.Nonce, state.Nonce)
	}
	if parsed.PKCEVerifier != state.PKCEVerifier {
		t.Errorf("PKCEVerifier: got %q, want %q", parsed.PKCEVerifier, state.PKCEVerifier)
	}
}

func TestParseOIDCLoginState_WrongNumberOfParts(t *testing.T) {
	a := testOIDCAPI()
	cases := []string{"noparts", "a.b.c", "", "."}
	for _, raw := range cases {
		if _, err := a.parseOIDCLoginState(raw); err == nil {
			t.Errorf("parseOIDCLoginState(%q): expected error, got nil", raw)
		}
	}
}

func TestParseOIDCLoginState_SignatureMismatch(t *testing.T) {
	a := testOIDCAPI()
	signed, _ := a.signOIDCLoginState(validOIDCState(10 * time.Minute))
	parts := strings.SplitN(signed, ".", 2)
	wrongSig := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if _, err := a.parseOIDCLoginState(parts[0] + "." + wrongSig); err == nil {
		t.Error("expected error for wrong signature")
	}
}

func TestParseOIDCLoginState_InvalidBase64Signature(t *testing.T) {
	a := testOIDCAPI()
	signed, _ := a.signOIDCLoginState(validOIDCState(10 * time.Minute))
	parts := strings.SplitN(signed, ".", 2)
	if _, err := a.parseOIDCLoginState(parts[0] + ".!!!!"); err == nil {
		t.Error("expected error for invalid base64 signature")
	}
}

func TestParseOIDCLoginState_Expired(t *testing.T) {
	a := testOIDCAPI()
	state := validOIDCState(-1 * time.Hour)
	signed, _ := a.signOIDCLoginState(state)
	if _, err := a.parseOIDCLoginState(signed); err == nil {
		t.Error("expected error for expired state")
	}
}

func TestParseOIDCLoginState_IncompleteFields(t *testing.T) {
	a := testOIDCAPI()
	// Missing Nonce
	incomplete := oidcLoginState{
		Version:       1,
		ReturnTo:      "https://app.example.test/",
		Nonce:         "", // missing
		PKCEVerifier:  "v",
		ExpiresAtUnix: time.Now().UTC().Add(10 * time.Minute).Unix(),
	}
	signed, _ := a.signOIDCLoginState(incomplete)
	if _, err := a.parseOIDCLoginState(signed); err == nil {
		t.Error("expected error for missing Nonce")
	}
}

func TestParseOIDCLoginState_DifferentKey(t *testing.T) {
	// State signed with key A should not parse with key B.
	aA := &API{config: config.Config{
		EncryptionKey: "key-A-32-bytes-exactly----------",
		PublicBaseURL: "https://app.example.test",
	}}
	aB := &API{config: config.Config{
		EncryptionKey: "key-B-32-bytes-exactly----------",
		PublicBaseURL: "https://app.example.test",
	}}

	state := validOIDCState(10 * time.Minute)
	signed, _ := aA.signOIDCLoginState(state)
	if _, err := aB.parseOIDCLoginState(signed); err == nil {
		t.Error("state signed with key A should not parse with key B")
	}
}

// ── randomURLSafeString ───────────────────────────────────────────────────────

func TestRandomURLSafeString(t *testing.T) {
	t.Run("NonEmpty", func(t *testing.T) {
		got, err := randomURLSafeString(32)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) == 0 {
			t.Error("expected non-empty string")
		}
	})

	t.Run("URLSafeCharacters", func(t *testing.T) {
		got, err := randomURLSafeString(32)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, c := range got {
			if c == '+' || c == '/' || c == '=' {
				t.Errorf("non-URL-safe character %q found in %q", c, got)
			}
		}
	})

	t.Run("Unique", func(t *testing.T) {
		a, _ := randomURLSafeString(32)
		b, _ := randomURLSafeString(32)
		if a == b {
			t.Error("expected different strings on successive calls (collision extremely unlikely)")
		}
	})

	t.Run("LongerInput", func(t *testing.T) {
		got, err := randomURLSafeString(64)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// base64 of 64 bytes → ≥ 86 characters
		if len(got) < 80 {
			t.Errorf("expected longer output for 64-byte input, got len=%d", len(got))
		}
	})
}

// ── oidcURLOrigin ─────────────────────────────────────────────────────────────

func TestOIDCURLOrigin(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"https://authentik.example.test/application/o/", "https://authentik.example.test"},
		{"http://localhost:8080/path", "http://localhost:8080"},
		{"", ""},
		{"/relative", ""},
		{"https://", ""},
		{"not-a-url-at-all", ""},
	}
	for _, tc := range cases {
		got := oidcURLOrigin(tc.raw)
		if got != tc.want {
			t.Errorf("oidcURLOrigin(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// ── sortedQueryKeys ───────────────────────────────────────────────────────────

func TestSortedQueryKeys(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		got := sortedQueryKeys(url.Values{})
		if len(got) != 0 {
			t.Errorf("expected empty slice, got %v", got)
		}
	})

	t.Run("Sorted", func(t *testing.T) {
		values := url.Values{
			"zebra":  {"1"},
			"apple":  {"2"},
			"mango":  {"3"},
			"cherry": {"4"},
		}
		got := sortedQueryKeys(values)
		want := []string{"apple", "cherry", "mango", "zebra"}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d", len(got), len(want))
		}
		for i, w := range want {
			if got[i] != w {
				t.Errorf("got[%d] = %q, want %q", i, got[i], w)
			}
		}
	})

	t.Run("Single", func(t *testing.T) {
		values := url.Values{"key": {"val"}}
		got := sortedQueryKeys(values)
		if len(got) != 1 || got[0] != "key" {
			t.Errorf("single key: got %v", got)
		}
	})
}

// ── truncateForOIDCLog ────────────────────────────────────────────────────────

func TestTruncateForOIDCLog(t *testing.T) {
	t.Run("UnderLimit", func(t *testing.T) {
		s := "short message"
		got := truncateForOIDCLog(s, 100)
		if got != s {
			t.Errorf("expected unchanged string, got %q", got)
		}
	})

	t.Run("AtLimit", func(t *testing.T) {
		s := strings.Repeat("x", 50)
		got := truncateForOIDCLog(s, 50)
		if got != s {
			t.Errorf("at-limit string should be unchanged")
		}
	})

	t.Run("OverLimit", func(t *testing.T) {
		s := strings.Repeat("a", 200)
		got := truncateForOIDCLog(s, 100)
		if !strings.HasPrefix(got, strings.Repeat("a", 100)) {
			t.Errorf("unexpected prefix in truncated string")
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("truncated string should end with ellipsis, got %q", got[:min(len(got), 20)])
		}
	})

	t.Run("Empty", func(t *testing.T) {
		got := truncateForOIDCLog("", 10)
		if got != "" {
			t.Errorf("empty string should stay empty, got %q", got)
		}
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
