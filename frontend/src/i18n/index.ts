import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

type LocaleFile = { ui: Record<string, unknown>; api: Record<string, unknown> }

// Vite bundles every locales/<code>.json at build time, so adding a new locale
// file (e.g. via Weblate) needs no code change here — it is picked up
// automatically. Keep this in sync with the backend's discoverLanguages.
const localeModules = import.meta.glob<LocaleFile>('../../../locales/*.json', {
  eager: true,
  import: 'default',
})

const localesByCode: Record<string, LocaleFile> = {}
for (const [path, mod] of Object.entries(localeModules)) {
  const code = path.match(/([^/]+)\.json$/)?.[1]
  if (code) localesByCode[code] = mod
}

// Native language name (endonym) for the picker, e.g. "Deutsch", "English".
function localeLabel(code: string): string {
  try {
    const name = new Intl.DisplayNames([code], { type: 'language' }).of(code)
    if (name) return name.charAt(0).toUpperCase() + name.slice(1)
  } catch {
    /* Intl.DisplayNames unavailable or unknown code */
  }
  return code.toUpperCase()
}

export type SupportedLanguage = string

export const supportedLanguages: ReadonlyArray<{ code: SupportedLanguage; label: string }> =
  Object.keys(localesByCode)
    .sort()
    .map((code) => ({ code, label: localeLabel(code) }))

const resources = Object.fromEntries(
  Object.entries(localesByCode).map(([code, data]) => [
    code,
    { translation: { ...data.ui, api: data.api } },
  ]),
)

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
  resources,
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
