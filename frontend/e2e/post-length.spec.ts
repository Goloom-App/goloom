import { test, expect } from '@playwright/test'

import { graphemeLength, providerPostLength } from '../src/postLength'

// Pure-logic coverage for platform-aware post length counting (issue #22).
// Mirrors the backend's internal/textcount tests so both counters agree.

test.describe('graphemeLength', () => {
  test('counts user-perceived characters, not code units', () => {
    expect(graphemeLength('')).toBe(0)
    expect(graphemeLength('hello')).toBe(5)
    expect(graphemeLength('🎉')).toBe(1)
    expect(graphemeLength('👨‍👩‍👧‍👦')).toBe(1)
    expect(graphemeLength('🇩🇪')).toBe(1)
    expect(graphemeLength('hi 🎉👨‍👩‍👧‍👦')).toBe(5)
  })
})

test.describe('providerPostLength for Mastodon', () => {
  test('every URL counts as 23 characters regardless of length', () => {
    expect(providerPostLength('mastodon', `https://example.com/${'x'.repeat(200)}`)).toBe(23)
    expect(providerPostLength('mastodon', 'https://a.io')).toBe(23)
    expect(
      providerPostLength('mastodon', 'look at this: https://example.com/very/long/path?with=query'),
    ).toBe(14 + 23)
    expect(
      providerPostLength('mastodon', `https://one.example https://two.example/${'y'.repeat(100)}`),
    ).toBe(23 + 1 + 23)
  })

  test('remote mentions count only the local part', () => {
    expect(providerPostLength('mastodon', 'hi @user@mastodon.example!')).toBe('hi @user!'.length)
    expect(providerPostLength('mastodon', 'hi @user!')).toBe(9)
    // E-mail addresses are not mentions.
    expect(providerPostLength('mastodon', 'mail me at user@example.com')).toBe(27)
  })

  test('provider matching is case-insensitive', () => {
    expect(providerPostLength('Mastodon', `https://example.com/${'x'.repeat(100)}`)).toBe(23)
  })
})

test.describe('providerPostLength for other platforms', () => {
  test('bluesky, friendica and pixelfed count URLs at full length', () => {
    const url = `https://example.com/${'x'.repeat(30)}` // 50 chars
    for (const provider of ['bluesky', 'friendica', 'pixelfed']) {
      expect(providerPostLength(provider, url)).toBe(50)
    }
  })

  test('bluesky counts graphemes (300-char limit is grapheme-based)', () => {
    expect(providerPostLength('bluesky', '👨‍👩‍👧‍👦!')).toBe(2)
  })

  test('unknown providers fall back to grapheme counting', () => {
    expect(providerPostLength('', 'hello 🎉')).toBe(7)
  })
})
