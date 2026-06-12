import { initialSettings } from './data'
import { isAppSection } from './i18n/sections'
import type { AppSection, SettingsState } from './types'

export const SETTINGS_STORAGE_KEY = 'goloom-ui-settings'
export const LAST_SECTION_STORAGE_KEY = 'goloom.last_section.v1'
export const LAST_TEAM_STORAGE_KEY = 'goloom.last_team.v1'

export function loadStoredSettings(): SettingsState {
  if (typeof window === 'undefined') {
    return initialSettings
  }
  const raw = window.localStorage.getItem(SETTINGS_STORAGE_KEY)
  if (!raw) {
    return initialSettings
  }
  try {
    const parsed = JSON.parse(raw) as Partial<SettingsState>
    return {
      ...initialSettings,
      ...parsed,
      ui: { ...initialSettings.ui, ...parsed.ui },
      general: { ...initialSettings.general, ...parsed.general },
      oidc: { ...initialSettings.oidc, ...parsed.oidc },
      security: { ...initialSettings.security, ...parsed.security },
      scheduler: { ...initialSettings.scheduler, ...parsed.scheduler },
      providers: { ...initialSettings.providers, ...parsed.providers },
    }
  } catch {
    return initialSettings
  }
}

export function writeStoredSettings(settings: SettingsState) {
  if (typeof window === 'undefined') {
    return
  }
  window.localStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(settings))
}

export function loadInitialSection(): AppSection {
  if (typeof window === 'undefined') {
    return 'dashboard'
  }
  const sectionFromQuery = new URLSearchParams(window.location.search).get('section')?.trim() ?? ''
  if (sectionFromQuery && isAppSection(sectionFromQuery)) {
    return sectionFromQuery
  }
  const stored = window.localStorage.getItem(LAST_SECTION_STORAGE_KEY)?.trim() ?? ''
  if (stored && isAppSection(stored)) {
    return stored
  }
  return 'dashboard'
}

export function loadInitialTeamId(): string {
  if (typeof window === 'undefined') {
    return ''
  }
  const teamFromQuery = new URLSearchParams(window.location.search).get('team')?.trim() ?? ''
  if (teamFromQuery) {
    return teamFromQuery
  }
  return window.localStorage.getItem(LAST_TEAM_STORAGE_KEY)?.trim() ?? ''
}
