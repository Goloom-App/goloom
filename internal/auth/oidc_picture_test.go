package auth

import (
	"encoding/json"
	"testing"
)

func TestOIDCClaimsPicture(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
	}{
		{"supplied", `{"sub":"user-1","picture":"https://example.test/avatar.png"}`, "https://example.test/avatar.png"},
		{"absent", `{"sub":"user-1"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var claims oidcClaims
			if err := json.Unmarshal([]byte(tc.raw), &claims); err != nil {
				t.Fatal(err)
			}
			if claims.Picture != tc.want {
				t.Fatalf("picture = %q, want %q", claims.Picture, tc.want)
			}
		})
	}
}
