import { Home, Calendar, Image, ChartBar, Settings, ShieldCheck, Users, RefreshCw, Share2, Archive, type LucideIcon } from 'lucide-react'
import type { AppSection } from '../../types'

export interface NavItem {
  id: AppSection
  label: string
  icon: LucideIcon
}

export const MAIN_NAV: NavItem[] = [
  { id: 'dashboard', label: 'Home', icon: Home },
  { id: 'contentCalendar', label: 'Calendar', icon: Calendar },
  { id: 'mediaLibrary', label: 'Media', icon: Image },
]

/** Additional nav items shown only in the mobile More drawer */
export const MORE_NAV: NavItem[] = [
  { id: 'analytics', label: 'Analytics', icon: ChartBar },
]

export const WORKSPACE_NAV: NavItem[] = [
  { id: 'teams', label: 'Team', icon: Users },
  { id: 'accounts', label: 'Accounts', icon: Share2 },
  { id: 'recurringPosts', label: 'Recurring', icon: RefreshCw },
  { id: 'archive', label: 'Archive', icon: Archive },
]

export const CONFIG_NAV: NavItem[] = [
  { id: 'settings', label: 'Settings', icon: Settings },
  { id: 'admin', label: 'Admin', icon: ShieldCheck },
]
