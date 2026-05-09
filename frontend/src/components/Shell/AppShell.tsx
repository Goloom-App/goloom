import React, { useState } from 'react'
import { Sidebar } from './Sidebar'
import { BottomNav, MobileDrawer } from './MobileNav'
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
  previewColumn
}: AppShellProps) {
  const [drawerOpen, setDrawerOpen] = useState(false)

  return (
    <div 
      className={`app-shell ${showPreviewColumn ? 'app-shell--triple' : 'app-shell--double'}`} 
      data-theme={resolvedTheme}
    >
      <div className="mesh-bg" />
      
      <Sidebar 
        currentSection={section}
        setSection={setSection}
        teams={teams}
        selectedTeamId={selectedTeamId}
        onSelectTeam={onSelectTeam}
        onSignOut={onSignOut}
        openComposer={openComposer}
      />

      <main className="app-main">
        {children}
      </main>

      {showPreviewColumn && (
        <aside className="preview-column">
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
        onSelectTeam={onSelectTeam}
        user={user}
        onSignOut={onSignOut}
      />
    </div>
  )
}
