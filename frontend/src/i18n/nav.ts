import type { LucideIcon } from 'lucide-react'
import { Home, Calendar, Image, ChartBar, Settings, ShieldCheck, Users, RefreshCw, Share2, Archive, Bot } from 'lucide-react'
import type { TFunction } from 'i18next'

import type { AppSection } from '../types'

export interface NavItemDef {
  id: AppSection
  labelKey: string
  icon: LucideIcon
}

export interface NavItem {
  id: AppSection
  label: string
  icon: LucideIcon
}

export const MAIN_NAV_DEF: NavItemDef[] = [
  { id: 'dashboard', labelKey: 'nav.home', icon: Home },
  { id: 'contentCalendar', labelKey: 'nav.calendar', icon: Calendar },
  { id: 'mediaLibrary', labelKey: 'nav.media', icon: Image },
]

export const MORE_NAV_DEF: NavItemDef[] = [{ id: 'analytics', labelKey: 'nav.analytics', icon: ChartBar }]

export const WORKSPACE_NAV_DEF: NavItemDef[] = [
  { id: 'analytics', labelKey: 'nav.analytics', icon: ChartBar },
  { id: 'teams', labelKey: 'nav.team', icon: Users },
  { id: 'accounts', labelKey: 'nav.accounts', icon: Share2 },
  { id: 'recurringPosts', labelKey: 'nav.recurring', icon: RefreshCw },
  { id: 'archive', labelKey: 'nav.archive', icon: Archive },
]

export const AI_SERVICE_NAV_DEF: NavItemDef[] = [
  { id: 'aiProfile', labelKey: 'nav.aiProfile', icon: Bot },
  { id: 'aiCampaigns', labelKey: 'nav.aiCampaigns', icon: Bot },
  { id: 'aiGenerate', labelKey: 'nav.aiGenerate', icon: Bot },
  { id: 'aiProactive', labelKey: 'nav.aiProactive', icon: Bot },
]

export const CONFIG_NAV_DEF: NavItemDef[] = [
  { id: 'settings', labelKey: 'nav.settings', icon: Settings },
  { id: 'admin', labelKey: 'nav.admin', icon: ShieldCheck },
]

export function localizeNav(def: NavItemDef[], t: TFunction): NavItem[] {
  return def.map((item) => ({ id: item.id, label: t(item.labelKey), icon: item.icon }))
}
