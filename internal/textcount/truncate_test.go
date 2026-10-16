package textcount

import (
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"short unchanged", "Grüße", 10, "Grüße"},
		{"exact unchanged", "Grüße", 5, "Grüße"},
		{"umlauts keep beginning", "Größenwahn", 5, "Größe…"},
		{"emoji", "a😀b😀c", 3, "a😀b…"},
		{"zwj sequence stays whole", "x👨‍👩‍👧‍👦y", 2, "x👨‍👩‍👧‍👦…"},
		{"zero", "abc", 0, ""},
	}
	for _, tc := range cases {
		got := Truncate(tc.in, tc.max)
		if got != tc.want {
			t.Errorf("%s: Truncate(%q, %d) = %q, want %q", tc.name, tc.in, tc.max, got, tc.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("%s: invalid UTF-8 %q", tc.name, got)
		}
	}
}
