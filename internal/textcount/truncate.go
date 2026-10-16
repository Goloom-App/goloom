package textcount

import "github.com/rivo/uniseg"

// Truncate keeps the first max grapheme clusters of s and appends "…" when
// anything was cut. Cutting on cluster boundaries never splits a multi-byte
// rune or an emoji sequence, so the result is always valid UTF-8.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if uniseg.GraphemeClusterCount(s) <= max {
		return s
	}
	end, n := 0, 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		if n == max {
			break
		}
		_, end = g.Positions()
		n++
	}
	return s[:end] + "…"
}
