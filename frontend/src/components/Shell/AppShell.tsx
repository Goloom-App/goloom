import React, { useState } from 'react'
import { Sidebar } from './Sidebar'
import { BottomNav, MobileDrawer } from './MobileNav'
import { PullToRefresh } from '../ui/PullToRefresh'
import type { AppSection, TeamRecord, UserRecord } from '../../types'

interface AppShellProps {
  children: React.ReactNode
  section: AppSection
  setSection: (section: AppSection) => void
  teams: TeamRecord[]
  selectedTeamId: string
  onSelectTeam: (id: string) => void
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
}

export function AppShell({
  children,
  section,
  setSection,
  teams,
  selectedTeamId,
  onSelectTeam,
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
}: AppShellProps) {
  const [drawerOpen, setDrawerOpen] = useState(false)

  return (
    <div 
      className={`app-shell ${showPreviewColumn ? 'app-shell--triple' : 'app-shell--double'} ${isComposer ? 'app-shell--composer' : ''}`} 
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
        onSignOut={onSignOut}
        openComposer={openComposer}
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
