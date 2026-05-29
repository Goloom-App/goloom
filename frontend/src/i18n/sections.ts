import type { TFunction } from 'i18next'

import type { AppSection } from '../types'

const sectionKeys: Record<AppSection, string> = {
  dashboard: 'section.dashboard',
  calendar: 'section.calendar',
  contentCalendar: 'section.contentCalendar',
  archive: 'section.archive',
  analytics: 'section.analytics',
  mediaLibrary: 'section.mediaLibrary',
  management: 'section.management',
  teams: 'section.teams',
  recurringPosts: 'section.recurringPosts',
  accounts: 'section.accounts',
  composer: 'section.composer',
  settings: 'section.settings',
  admin: 'section.admin',
  aiProfile: 'section.aiProfile',
  aiCampaigns: 'section.aiCampaigns',
  aiGenerate: 'section.aiGenerate',
  aiProactive: 'section.aiProactive',
}

export function sectionHeading(t: TFunction, section: AppSection): string {
  return t(sectionKeys[section])
}

export function isAppSection(value: string): value is AppSection {
  return value in sectionKeys
}
