package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

func signHS256(t *testing.T, secret, payload string) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte(secret)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	jws, err := signer.Sign([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	compact, err := jws.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return compact
}

func TestPeekJWTSigningAlg(t *testing.T) {
	jwt := signHS256(t, "hs256-unit-test-secret-32-bytes!", `{"sub":"x"}`)
	alg, err := peekJWTSigningAlg(jwt)
	if err != nil || alg != "HS256" {
		t.Fatalf("alg = %q err=%v", alg, err)
	}

	if _, err := peekJWTSigningAlg("!!!not-base64!!!.x.y"); err == nil {
		t.Fatal("invalid header must error")
	}
	if _, err := peekJWTSigningAlg("e30.x.y"); err == nil { // {} → no alg
		t.Fatal("missing alg must error")
	}
}

func TestHybridJWTKeySetHMAC(t *testing.T) {
	ctx := context.Background()
	keySet := newHybridJWTKeySet(nil, "hs256-unit-test-secret-32-bytes!")

	jwt := signHS256(t, "hs256-unit-test-secret-32-bytes!", `{"sub":"u1"}`)
	payload, err := keySet.VerifySignature(ctx, jwt)
	if err != nil {
		t.Fatalf("VerifySignature: %v", err)
	}
	if string(payload) != `{"sub":"u1"}` {
		t.Fatalf("payload = %s", payload)
	}

	if _, err := keySet.VerifySignature(ctx, signHS256(t, "other-hs256-test-secret-32-bytes", `{}`)); err == nil {
		t.Fatal("wrong secret must fail verification")
	}

	empty := newHybridJWTKeySet(nil, "")
	if _, err := empty.VerifySignature(ctx, jwt); err == nil || !strings.Contains(err.Error(), "OIDC_CLIENT_SECRET") {
		t.Fatalf("HS256 without client secret must fail clearly, got %v", err)
	}
}

func TestBuildSupportedIDTokenAlgorithms(t *testing.T) {
	// Without client secret: HS* filtered out, unknown algs dropped.
	algs := buildSupportedIDTokenAlgorithms([]string{"RS256", "HS256", "weird"}, "")
	if len(algs) != 1 || algs[0] != oidc.RS256 {
		t.Fatalf("algs = %v", algs)
	}

	// With client secret: discovered HS256 kept, missing HS* appended.
	algs = buildSupportedIDTokenAlgorithms([]string{"RS256", "HS256"}, "secret")
	want := map[string]bool{"RS256": true, "HS256": true, "HS384": true, "HS512": true}
	if len(algs) != len(want) {
		t.Fatalf("algs = %v", algs)
	}
	for _, a := range algs {
		if !want[a] {
			t.Fatalf("unexpected alg %q in %v", a, algs)
		}
	}

	// Empty discovery falls back to RS256.
	algs = buildSupportedIDTokenAlgorithms(nil, "")
	if len(algs) != 1 || algs[0] != oidc.RS256 {
		t.Fatalf("fallback algs = %v", algs)
	}
}
