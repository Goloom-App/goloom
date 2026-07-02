// Package textcount measures post content the way each social platform's
// character limit does. Platforms do not count Go runes: Mastodon counts every
// URL as a fixed 23 characters and remote mentions without their domain, and
// Bluesky's 300-character limit is defined in grapheme clusters, so an emoji
// sequence like 👨‍👩‍👧‍👦 is one character even though it is many runes.
package textcount

import (
	"regexp"
	"strings"

	"github.com/rivo/uniseg"
)

// mastodonURLLength is the fixed cost Mastodon assigns to every URL,
// regardless of the URL's real length (see Mastodon's StatusLengthValidator).
const mastodonURLLength = 23

// urlPattern approximates Mastodon's URL detection: an http(s) URL up to the
// next whitespace.
var urlPattern = regexp.MustCompile(`https?://\S+`)

// remoteMentionPattern matches remote mentions (@user@domain). The leading @
// must not follow a word character or slash, so e-mail addresses and URL
// fragments are left alone — mirrors Mastodon's Account::MENTION_RE.
var remoteMentionPattern = regexp.MustCompile(`(^|[^/\w])@(\w+(?:[\w.-]*\w)?)@[\w.-]*\w`)

// Graphemes counts user-perceived characters (Unicode grapheme clusters).
func Graphemes(s string) int {
	return uniseg.GraphemeClusterCount(s)
}

// ProviderLength returns the number of characters content occupies against the
// given provider's character limit, using that platform's counting rules.
// Unknown providers are counted in plain grapheme clusters.
func ProviderLength(provider, content string) int {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "mastodon":
		return mastodonLength(content)
	default:
		// Bluesky, Friendica and Pixelfed count URLs at full length; Bluesky
		// explicitly defines its limit in grapheme clusters.
		return Graphemes(content)
	}
}

func mastodonLength(content string) int {
	replaced := urlPattern.ReplaceAllString(content, strings.Repeat("x", mastodonURLLength))
	replaced = remoteMentionPattern.ReplaceAllString(replaced, "$1@$2")
	return Graphemes(replaced)
}
