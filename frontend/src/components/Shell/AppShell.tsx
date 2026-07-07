import React, { useState } from 'react'
import { Sidebar } from './Sidebar'
import { BottomNav, MobileDrawer } from './MobileNav'
import { PullToRefresh } from '../ui/PullToRefresh'
import type { AppSection, TeamRecord, UserRecord } from '../../types'
import type { BackendVersionInfo } from '../../api'

const SIDEBAR_COLLAPSED_STORAGE_KEY = 'goloom.sidebar.collapsed.v1'

function loadSidebarCollapsed(): boolean {
  if (typeof window === 'undefined') {
    return false
  }
  return window.localStorage.getItem(SIDEBAR_COLLAPSED_STORAGE_KEY) === '1'
}

interface AppShellProps {
  children: React.ReactNode
  section: AppSection
  setSection: (section: AppSection) => void
  teams: TeamRecord[]
  selectedTeamId: string
  onSelectTeam: (id: string) => void
  onCreateTeam: () => void
  user: UserRecord | null
  onSignOut: () => void
  openComposer: () => void
  resolvedTheme: 'light' | 'dark'
  showPreviewColumn?: boolean
  previewColumn?: React.ReactNode
  isComposer?: boolean
  onRefresh?: () => void | Promise<void>
  pullToRefreshDisabled?: boolean
  reviewQueueCount?: number
  reviewQueueOverdueCount?: number
  versionInfo?: BackendVersionInfo | null
}

export function AppShell({
  children,
  section,
  setSection,
  teams,
  selectedTeamId,
  onSelectTeam,
  onCreateTeam,
  user,
  onSignOut,
  openComposer,
  resolvedTheme,
  showPreviewColumn,
  previewColumn,
  isComposer,
  onRefresh,
  pullToRefreshDisabled = true,
  reviewQueueCount = 0,
  reviewQueueOverdueCount = 0,
  versionInfo,
}: AppShellProps) {
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [sidebarCollapsed, setSidebarCollapsed] = useState(loadSidebarCollapsed)

  const toggleSidebarCollapsed = () => {
    setSidebarCollapsed((current) => {
      const next = !current
      if (typeof window !== 'undefined') {
        window.localStorage.setItem(SIDEBAR_COLLAPSED_STORAGE_KEY, next ? '1' : '0')
      }
      return next
    })
  }

  return (
    <div
      className={`app-shell ${showPreviewColumn ? 'app-shell--triple' : 'app-shell--double'} ${isComposer ? 'app-shell--composer' : ''} ${sidebarCollapsed ? 'app-shell--sidebar-collapsed' : ''}`}
      data-theme={resolvedTheme}
    >
      <div className="mesh-bg" />

      <Sidebar
        currentSection={section}
        setSection={setSection}
        teams={teams}
        selectedTeamId={selectedTeamId}
        reviewQueueCount={reviewQueueCount}
        reviewQueueOverdueCount={reviewQueueOverdueCount}
        onSelectTeam={onSelectTeam}
        onCreateTeam={onCreateTeam}
        user={user}
        onSignOut={onSignOut}
        openComposer={openComposer}
        collapsed={sidebarCollapsed}
        onToggleCollapsed={toggleSidebarCollapsed}
        versionInfo={versionInfo}
      />

      <PullToRefresh
        onRefresh={onRefresh ?? (() => {})}
        disabled={pullToRefreshDisabled || !onRefresh}
        className="app-main"
      >
        {children}
      </PullToRefresh>

      {showPreviewColumn && (
        <aside className={`preview-column ${section === 'composer' ? 'preview-column--composer' : ''}`}>
          {previewColumn}
        </aside>
      )}

      <BottomNav 
        currentSection={section}
        setSection={setSection}
        openComposer={openComposer}
        openDrawer={() => setDrawerOpen(true)}
      />

      <MobileDrawer 
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        currentSection={section}
        setSection={setSection}
        teams={teams}
        selectedTeamId={selectedTeamId}
        reviewQueueCount={reviewQueueCount}
        reviewQueueOverdueCount={reviewQueueOverdueCount}
        onSelectTeam={onSelectTeam}
        user={user}
        onSignOut={onSignOut}
      />
    </div>
  )
}
