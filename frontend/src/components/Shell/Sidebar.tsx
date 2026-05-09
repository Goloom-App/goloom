import { MAIN_NAV, WORKSPACE_NAV, CONFIG_NAV } from './NavItems'
import type { AppSection, TeamRecord } from '../../types'
import { LogOut, Plus, ChevronDown } from 'lucide-react'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'

interface SidebarProps {
  currentSection: AppSection
  setSection: (section: AppSection) => void
  teams: TeamRecord[]
  selectedTeamId: string
  onSelectTeam: (id: string) => void
  onSignOut: () => void
  openComposer: () => void
}

export function Sidebar({ 
  currentSection, 
  setSection, 
  teams, 
  selectedTeamId, 
  onSelectTeam,
  onSignOut,
  openComposer
}: SidebarProps) {
  const selectedTeam = teams.find(t => t.id === selectedTeamId)

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <div className="sidebar-logo">
          <div className="sidebar-logo__mark">
            <span className="sidebar-logo__mark-layer sidebar-logo__mark-layer--a" />
            <span className="sidebar-logo__mark-layer sidebar-logo__mark-layer--b" />
            <span className="sidebar-logo__mark-layer sidebar-logo__mark-layer--c" />
          </div>
          <span className="sidebar-logo__text">goloom</span>
        </div>

        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button className="sidebar-team-selector">
              <span className="sidebar-team-name">
                {selectedTeam?.name || 'Select Team'}
              </span>
              <ChevronDown size={14} />
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
              <DropdownMenu.Item className="radix-dropdown-item">
                + Create Team
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>

      <button className="btn btn--primary btn--full" onClick={openComposer}>
        <Plus size={18} />
        New Post
      </button>

      <nav className="sidebar-nav">
        <div className="sidebar-section">
          <p className="sidebar-section__label">Main</p>
          <div className="sidebar-nav">
            {MAIN_NAV.map(item => (
              <button
                key={item.id}
                className={`sidebar-nav-item ${currentSection === item.id ? 'sidebar-nav-item--active' : ''}`}
                onClick={() => setSection(item.id)}
              >
                <item.icon size={18} />
                <span>{item.label}</span>
              </button>
            ))}
          </div>
        </div>

        <div className="sidebar-section">
          <p className="sidebar-section__label">Workspace</p>
          <div className="sidebar-nav">
            {WORKSPACE_NAV.map(item => (
              <button
                key={item.id}
                className={`sidebar-nav-item ${currentSection === item.id ? 'sidebar-nav-item--active' : ''}`}
                onClick={() => setSection(item.id)}
              >
                <item.icon size={18} />
                <span>{item.label}</span>
              </button>
            ))}
          </div>
        </div>
      </nav>

      <div className="sidebar-footer">
        {CONFIG_NAV.map(item => (
          <button
            key={item.id}
            className={`sidebar-footer-item ${currentSection === item.id ? 'sidebar-nav-item--active' : ''}`}
            onClick={() => setSection(item.id)}
          >
            <item.icon size={18} />
            <span>{item.label}</span>
          </button>
        ))}
        <button
          className="sidebar-footer-item btn--danger-ghost"
          onClick={onSignOut}
        >
          <LogOut size={18} />
          <span>Sign Out</span>
        </button>
      </div>
    </aside>
  )
}
