import React, { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { Plus, Menu, X, Bot } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CONFIG_NAV_DEF, localizeNav, MAIN_NAV_DEF, MORE_NAV_DEF, WORKSPACE_NAV_DEF } from '../../i18n/nav'
import type { AppSection, TeamRecord, UserRecord } from '../../types'
import { NavReviewCount, NavReviewIcon } from './NavReviewIndicator'

interface BottomNavProps {
  currentSection: AppSection
  setSection: (section: AppSection) => void
  openComposer: () => void
  openDrawer: () => void
}

export function BottomNav({ currentSection, setSection, openComposer, openDrawer }: BottomNavProps) {
  const { t } = useTranslation()
  const [homeNav, calendarNav, mediaNav] = localizeNav(MAIN_NAV_DEF, t)

  return (
    <nav className="bottom-nav">
      <button
        className={`nav-item ${currentSection === homeNav.id ? 'nav-item--active' : ''}`}
        onClick={() => setSection(homeNav.id)}
      >
        <homeNav.icon />
        <span>{homeNav.label}</span>
      </button>

      <button
        className={`nav-item ${currentSection === calendarNav.id ? 'nav-item--active' : ''}`}
        onClick={() => setSection(calendarNav.id)}
      >
        <calendarNav.icon />
        <span>{calendarNav.label}</span>
      </button>

      <button className="nav-item nav-item--fab" onClick={openComposer}>
        <Plus size={32} strokeWidth={3} />
        <span>{t('sidebarShell.post')}</span>
      </button>

      <button
        className={`nav-item ${currentSection === mediaNav.id ? 'nav-item--active' : ''}`}
        onClick={() => setSection(mediaNav.id)}
      >
        <mediaNav.icon />
        <span>{mediaNav.label}</span>
      </button>

      <button className="nav-item" onClick={openDrawer}>
        <Menu />
        <span>{t('nav.more')}</span>
      </button>
    </nav>
  )
}

interface MobileDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentSection: AppSection
  setSection: (section: AppSection) => void
  teams: TeamRecord[]
  selectedTeamId: string
  reviewQueueCount?: number
  reviewQueueOverdueCount?: number
  onSelectTeam: (id: string) => void
  user: UserRecord | null
  onSignOut: () => void
}

export function MobileDrawer({ 
  open, 
  onOpenChange, 
  currentSection, 
  setSection, 
  teams, 
  selectedTeamId,
  reviewQueueCount = 0,
  reviewQueueOverdueCount = 0,
  onSelectTeam,
  user,
  onSignOut
}: MobileDrawerProps) {
  const { t } = useTranslation()
  const moreNav = localizeNav(MORE_NAV_DEF, t)
  const workspaceNav = localizeNav(WORKSPACE_NAV_DEF, t)
  const configNav = localizeNav(CONFIG_NAV_DEF, t)
  const [touchStart, setTouchStart] = useState<number | null>(null)
  const [translateY, setTranslateY] = useState(0)

  const handleTouchStart = (e: React.TouchEvent) => {
    setTouchStart(e.touches[0].clientY)
  }

  const handleTouchMove = (e: React.TouchEvent) => {
    if (touchStart === null) return
    const delta = e.touches[0].clientY - touchStart
    if (delta > 0) {
      setTranslateY(delta)
    }
  }

  const handleTouchEnd = () => {
    if (translateY > 150) {
      onOpenChange(false)
    }
    setTouchStart(null)
    setTranslateY(0)
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay 
          className="dialog-overlay" 
          style={{ opacity: Math.max(0, 1 - translateY / 300) }}
        />
        <Dialog.Content 
          className="dialog-content" 
          data-side="bottom"
          style={{ 
            transform: `translateY(${translateY}px)`,
            transition: touchStart === null ? 'transform 0.3s cubic-bezier(0.2, 0.8, 0.2, 1)' : 'none'
          }}
          onTouchStart={handleTouchStart}
          onTouchMove={handleTouchMove}
          onTouchEnd={handleTouchEnd}
        >
          <div className="drawer-handle" aria-hidden="true" />
          <div className="drawer-header">
            <Dialog.Title className="drawer-title">{t('sidebarShell.menu')}</Dialog.Title>
            <Dialog.Close asChild>
              <button className="btn btn--ghost btn--icon-sm">
                <X size={20} />
              </button>
            </Dialog.Close>
          </div>

          <div className="drawer-body">
            <section>
              <p className="eyebrow drawer-section-label">{t('mobileNav.workspaces')}</p>
              <div className="drawer-list">
                {teams.map((team) => (
                  <button
                    key={team.id}
                    className={`btn btn--ghost btn--full btn--justify-start ${team.id === selectedTeamId ? 'btn--active' : ''}`}
                    onClick={() => {
                      onSelectTeam(team.id)
                      onOpenChange(false)
                    }}
                  >
                    {team.name}
                  </button>
                ))}
              </div>
            </section>

            <section>
              <p className="eyebrow drawer-section-label">{t('nav.analytics')}</p>
              <div className="drawer-list">
                {moreNav.map((item) => (
                  <button
                    key={item.id}
                    className={`btn btn--ghost btn--full btn--justify-start ${currentSection === item.id ? 'btn--active' : ''}`}
                    onClick={() => {
                      setSection(item.id)
                      onOpenChange(false)
                    }}
                  >
                    <item.icon size={18} />
                    <span>{item.label}</span>
                  </button>
                ))}
              </div>
            </section>

            <section>
              <p className="eyebrow drawer-section-label">{t('sidebarShell.management')}</p>
              <div className="drawer-grid">
                {workspaceNav.map((item) => (
                  <button
                    key={item.id}
                    className={`btn btn--ghost btn--justify-start ${currentSection === item.id ? 'btn--active' : ''}`}
                    onClick={() => {
                      setSection(item.id)
                      onOpenChange(false)
                    }}
                  >
                    <NavReviewIcon
                      sectionId={item.id}
                      icon={item.icon}
                      count={reviewQueueCount}
                      overdueCount={reviewQueueOverdueCount}
                    />
                    <span className="drawer-item-label">{item.label}</span>
                    <NavReviewCount
                      sectionId={item.id}
                      count={reviewQueueCount}
                      overdueCount={reviewQueueOverdueCount}
                    />
                  </button>
                ))}
                {teams.find(t => t.id === selectedTeamId)?.isAiEnabled && (
                  <>
                    <button
                      className={`btn btn--ghost btn--justify-start ${currentSection === 'aiProfile' ? 'btn--active' : ''}`}
                      onClick={() => {
                        setSection('aiProfile')
                        onOpenChange(false)
                      }}
                    >
                      <Bot size={18} />
                      <span className="drawer-item-label">AI Profile</span>
                    </button>
                    <button
                      className={`btn btn--ghost btn--justify-start ${currentSection === 'aiCampaigns' ? 'btn--active' : ''}`}
                      onClick={() => {
                        setSection('aiCampaigns')
                        onOpenChange(false)
                      }}
                    >
                      <Bot size={18} />
                      <span className="drawer-item-label">Campaign Formats</span>
                    </button>
                  </>
                )}
              </div>
            </section>

            <section>
              <p className="eyebrow drawer-section-label">{t('mobileNav.settings')}</p>
              <div className="drawer-list">
                {configNav.map((item) => (
                  <button
                    key={item.id}
                    className={`btn btn--ghost btn--full btn--justify-start ${currentSection === item.id ? 'btn--active' : ''}`}
                    onClick={() => {
                      setSection(item.id)
                      onOpenChange(false)
                    }}
                  >
                    <item.icon size={18} />
                    <span>{item.label}</span>
                  </button>
                ))}
              </div>
            </section>

            <footer className="drawer-footer">
              <div className="drawer-user">
                <div className="avatar avatar--sm">
                  {user?.name?.[0] || '?'}
                </div>
                <div className="drawer-user-info">
                  <span className="drawer-user-name">{user?.name}</span>
                  <span className="drawer-user-email">{user?.email}</span>
                </div>
              </div>
              <button className="btn btn--ghost btn--danger-ghost" onClick={onSignOut}>
                Sign Out
              </button>
            </footer>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
