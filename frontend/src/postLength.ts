// Post length measured the way each social platform's character limit counts,
// mirroring the backend's internal/textcount package (issue #22):
// - Mastodon counts every URL as a fixed 23 characters and remote mentions
//   (@user@domain) without their domain.
// - Everything is counted in Unicode grapheme clusters, so an emoji sequence
//   like 👨‍👩‍👧‍👦 is one character (Bluesky defines its 300-char limit that way).
// - Bluesky, Friendica and Pixelfed give URLs no discount.

const MASTODON_URL_LENGTH = 23
const URL_PATTERN = /https?:\/\/\S+/g
// The leading @ must not follow a word character or slash, so e-mail addresses
// and URL fragments are left alone.
const REMOTE_MENTION_PATTERN = /(^|[^/\w])@(\w+(?:[\w.-]*\w)?)@[\w.-]*\w/g

const segmenter = new Intl.Segmenter(undefined, { granularity: 'grapheme' })

/** Counts user-perceived characters (Unicode grapheme clusters). */
export function graphemeLength(text: string): number {
  if (!text) {
    return 0
  }
  return [...segmenter.segment(text)].length
}

/** Effective character count of `text` against `provider`'s limit. */
export function providerPostLength(provider: string, text: string): number {
  if (provider.trim().toLowerCase() === 'mastodon') {
    const replaced = text
      .replace(URL_PATTERN, 'x'.repeat(MASTODON_URL_LENGTH))
      .replace(REMOTE_MENTION_PATTERN, '$1@$2')
    return graphemeLength(replaced)
  }
  return graphemeLength(text)
}
