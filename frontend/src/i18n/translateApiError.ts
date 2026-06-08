import type { TFunction } from 'i18next'

import en from '../../../locales/en.json'

/** Maps exact English API bodies (and keys) to catalog api.* keys. */
const englishToKey: Record<string, string> = {}
for (const [key, text] of Object.entries(en.api)) {
  englishToKey[text] = key
  englishToKey[key] = key
}

/**
 * Localizes a plain-text API error body. Unknown text is returned unchanged.
 */
export function translateApiError(message: string, t: TFunction): string {
  const trimmed = message.trim()
  if (!trimmed) return message
  const key = englishToKey[trimmed]
  if (key) {
    const translated = t(`api.${key}`, { defaultValue: '' })
    if (translated) return translated
  }
  return message
}
