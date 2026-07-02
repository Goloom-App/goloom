import { useTranslation } from 'react-i18next'

import { AI_SERVICE_NAV_DEF, localizeNav, MAIN_NAV_DEF, WORKSPACE_NAV_DEF, type NavItem } from '../../i18n/nav'
import type { AppSection, TeamRecord, UserRecord } from '../../types'
import { Plus, ChevronDown, PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { NavReviewCount, NavReviewIcon } from './NavReviewIndicator'
import { UserMenu } from './UserMenu'

interface SidebarProps {
  currentSection: AppSection
  setSection: (section: AppSection) => void
  teams: TeamRecord[]
  selectedTeamId: string
  reviewQueueCount?: number
  reviewQueueOverdueCount?: number
  onSelectTeam: (id: string) => void
  onCreateTeam: () => void
  user: UserRecord | null
  onSignOut: () => void
  openComposer: () => void
  collapsed: boolean
  onToggleCollapsed: () => void
}

export function Sidebar({
  currentSection,
  setSection,
  teams,
  selectedTeamId,
  reviewQueueCount = 0,
  reviewQueueOverdueCount = 0,
  onSelectTeam,
  onCreateTeam,
  user,
  onSignOut,
  openComposer,
  collapsed,
  onToggleCollapsed,
}: SidebarProps) {
  const { t } = useTranslation()
  const mainNav = localizeNav(MAIN_NAV_DEF, t)
  const workspaceNav = localizeNav(WORKSPACE_NAV_DEF, t)
  const aiServiceNav = localizeNav(AI_SERVICE_NAV_DEF, t)
  const selectedTeam = teams.find(t => t.id === selectedTeamId)

  const renderNavItem = (item: NavItem) => (
    <button
      key={item.id}
      type="button"
      data-testid={item.id === 'reviewQueue' ? 'nav-review-queue' : undefined}
      data-tour={`nav-${item.id}`}
      className={`sidebar-nav-item ${currentSection === item.id ? 'sidebar-nav-item--active' : ''}`}
      onClick={() => setSection(item.id)}
      title={collapsed ? item.label : undefined}
      aria-label={item.label}
    >
      <NavReviewIcon
        sectionId={item.id}
        icon={item.icon}
        count={reviewQueueCount}
        overdueCount={reviewQueueOverdueCount}
      />
      <span className="sidebar-nav-item__label">{item.label}</span>
      <NavReviewCount
        sectionId={item.id}
        count={reviewQueueCount}
        overdueCount={reviewQueueOverdueCount}
      />
    </button>
  )

  return (
    <aside className={`sidebar ${collapsed ? 'sidebar--collapsed' : ''}`}>
      <div className="sidebar-header">
        <div className="sidebar-logo">
          <div className="sidebar-logo__mark">
            <span className="sidebar-logo__mark-layer sidebar-logo__mark-layer--a" />
            <span className="sidebar-logo__mark-layer sidebar-logo__mark-layer--b" />
            <span className="sidebar-logo__mark-layer sidebar-logo__mark-layer--c" />
          </div>
          <span className="sidebar-logo__text">goloom</span>
          <button
            type="button"
            className="sidebar-collapse-toggle"
            data-testid="sidebar-collapse-toggle"
            onClick={onToggleCollapsed}
            title={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
            aria-label={collapsed ? t('sidebar.expand') : t('sidebar.collapse')}
          >
            {collapsed ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
          </button>
        </div>

        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              className="sidebar-team-selector"
              title={collapsed ? (selectedTeam?.name || t('sidebarShell.selectTeam')) : undefined}
            >
              <span className="sidebar-team-name">
                {collapsed
                  ? (selectedTeam?.name || t('sidebarShell.selectTeam')).slice(0, 2).toUpperCase()
                  : selectedTeam?.name || t('sidebarShell.selectTeam')}
              </span>
              {!collapsed && <ChevronDown size={14} />}
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content className="radix-dropdown-content" align="start">
              {teams.map((team) => (
                <DropdownMenu.Item
                  key={team.id}
                  className="radix-dropdown-item"
                  onSelect={() => onSelectTeam(team.id)}
                >
                  {team.name}
                </DropdownMenu.Item>
              ))}
              <DropdownMenu.Separator className="divider" />
              <DropdownMenu.Item
                className="radix-dropdown-item"
                data-testid="sidebar-create-team"
                onSelect={() => onCreateTeam()}
              >
                {t('sidebarShell.createTeam')}
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>

      <button
        className="btn btn--primary btn--full sidebar-composer-cta"
        data-tour="new-post"
        onClick={openComposer}
        title={collapsed ? t('sidebarShell.newPost') : undefined}
        aria-label={t('sidebarShell.newPost')}
      >
        <Plus size={18} />
        <span className="sidebar-composer-cta__label">{t('sidebarShell.newPost')}</span>
      </button>

      <nav className="sidebar-nav">
        <div className="sidebar-section">
          <p className="sidebar-section__label">{t('sidebarShell.main')}</p>
          <div className="sidebar-nav">{mainNav.map(renderNavItem)}</div>
        </div>

        <div className="sidebar-section">
          <p className="sidebar-section__label">{t('sidebar.workspace')}</p>
          <div className="sidebar-nav">{workspaceNav.map(renderNavItem)}</div>
        </div>

        {selectedTeam?.isAiEnabled && (
          <div className="sidebar-section">
            <p className="sidebar-section__label">{t('sidebar.aiService')}</p>
            <div className="sidebar-nav">{aiServiceNav.map(renderNavItem)}</div>
          </div>
        )}
      </nav>

      <div className="sidebar-footer">
        <UserMenu user={user} collapsed={collapsed} setSection={setSection} onSignOut={onSignOut} />
      </div>
    </aside>
  )
}
