// Package hashtag extracts hashtags from post content. Tags are grouped by a
// lowercase-normalized key because Mastodon, Friendica and Bluesky all match
// hashtags case-insensitively; the original casing is kept as display variant.
package hashtag

import (
	"regexp"
	"strings"
)

// Match is a hashtag occurrence in a text. Start and End are UTF-8 byte
// offsets (Start points at '#', End is exclusive), as required by Bluesky
// rich text facets. Display is the tag without the leading '#'.
type Match struct {
	Start   int
	End     int
	Display string
}

// Tag is a deduplicated hashtag of one text.
type Tag struct {
	Norm    string
	Display string
}

var (
	tagRe = regexp.MustCompile(`#[\p{L}\p{N}_]+`)
	urlRe = regexp.MustCompile(`https?://[^\s]+`)
	// letterRe guards against numeric-only tags ("#2024"), which neither
	// Mastodon nor Bluesky treat as hashtags.
	letterRe = regexp.MustCompile(`\p{L}`)
)

// Normalize returns the case-folded grouping key for a tag (without '#').
func Normalize(display string) string {
	return strings.ToLower(strings.TrimPrefix(display, "#"))
}

// URLMatches returns the URL spans of the text (byte offsets), with trailing
// punctuation trimmed. Used for link facets and to exclude URL fragments from
// hashtag matching.
func URLMatches(text string) []Match {
	var out []Match
	for _, loc := range urlRe.FindAllStringIndex(text, -1) {
		raw := text[loc[0]:loc[1]]
		trimmed := strings.TrimRight(raw, `.,;:!?)]}'"`)
		if trimmed == "" {
			continue
		}
		out = append(out, Match{Start: loc[0], End: loc[0] + len(trimmed), Display: trimmed})
	}
	return out
}

// Matches returns hashtag occurrences with byte offsets, skipping '#'
// sequences inside URLs (e.g. https://example.com/#section) and tags glued to
// a preceding word character ("abc#def").
func Matches(text string) []Match {
	urls := URLMatches(text)
	inURL := func(pos int) bool {
		for _, u := range urls {
			if pos >= u.Start && pos < u.End {
				return true
			}
		}
		return false
	}
	var out []Match
	for _, loc := range tagRe.FindAllStringIndex(text, -1) {
		if inURL(loc[0]) {
			continue
		}
		if loc[0] > 0 {
			prev := text[loc[0]-1]
			if prev == '#' || isWordByte(prev) {
				continue
			}
		}
		display := text[loc[0]+1 : loc[1]]
		if !letterRe.MatchString(display) {
			continue
		}
		out = append(out, Match{Start: loc[0], End: loc[1], Display: display})
	}
	return out
}

// isWordByte reports whether b is an ASCII letter, digit or underscore. A '#'
// directly after a non-ASCII rune is still treated as a tag start, matching
// the lenient behavior of the platforms.
func isWordByte(b byte) bool {
	return b == '_' ||
		(b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z')
}

// Extract returns the deduplicated tags of a text. The first occurrence's
// casing wins as display variant.
func Extract(text string) []Tag {
	seen := make(map[string]struct{})
	var out []Tag
	for _, m := range Matches(text) {
		norm := Normalize(m.Display)
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, Tag{Norm: norm, Display: m.Display})
	}
	return out
}
