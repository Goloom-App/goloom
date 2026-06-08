import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import en from '@locales/en.json'
import de from '@locales/de.json'

export const supportedLanguages = [
  { code: 'en', label: 'English' },
  { code: 'de', label: 'Deutsch' },
] as const

export type SupportedLanguage = (typeof supportedLanguages)[number]['code']

const STORAGE_KEY = 'goloom-ui-settings'

export function readStoredLanguage(): SupportedLanguage | undefined {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return undefined
    const parsed = JSON.parse(raw) as { ui?: { language?: string } }
    const lang = parsed.ui?.language
    if (lang && supportedLanguages.some((l) => l.code === lang)) {
      return lang as SupportedLanguage
    }
  } catch {
    /* ignore */
  }
  return undefined
}

function detectBrowserLanguage(): SupportedLanguage {
  const tags = navigator.languages?.length ? navigator.languages : [navigator.language]
  for (const tag of tags) {
    const primary = tag.split('-')[0]?.toLowerCase()
    if (primary && supportedLanguages.some((l) => l.code === primary)) {
      return primary as SupportedLanguage
    }
  }
  return 'en'
}

void i18n.use(initReactI18next).init({
  resources: {
    en: { translation: { ...en.ui, api: en.api } },
    de: { translation: { ...de.ui, api: de.api } },
  },
  lng: readStoredLanguage() ?? detectBrowserLanguage(),
  fallbackLng: 'en',
  interpolation: { escapeValue: false },
  returnNull: false,
})

export function setAppLanguage(code: SupportedLanguage) {
  void i18n.changeLanguage(code)
  if (typeof document !== 'undefined') {
    document.documentElement.lang = code
  }
}

if (typeof document !== 'undefined') {
  document.documentElement.lang = i18n.language
}

export default i18n
