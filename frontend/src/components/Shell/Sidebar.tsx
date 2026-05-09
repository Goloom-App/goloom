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
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: 'var(--space-6)' }}>
          <div style={{ width: 32, height: 32, background: 'var(--accent)', borderRadius: 'var(--radius-md)' }} />
          <h1 style={{ fontSize: '1.25rem' }}>goloom</h1>
        </div>

        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button className="btn btn--ghost" style={{ width: '100%', justifyContent: 'space-between', border: '1px solid var(--border)' }}>
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {selectedTeam?.name || 'Select Team'}
              </span>
              <ChevronDown size={16} />
            </button>
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content className="glass-panel" style={{ minWidth: 200, zIndex: 100 }}>
              {teams.map((team) => (
                <DropdownMenu.Item 
                  key={team.id} 
                  className="nav-item" 
                  style={{ flexDirection: 'row', justifyContent: 'flex-start', cursor: 'pointer' }}
                  onSelect={() => onSelectTeam(team.id)}
                >
                  {team.name}
                </DropdownMenu.Item>
              ))}
              <DropdownMenu.Separator style={{ height: 1, background: 'var(--border)', margin: '4px 0' }} />
              <DropdownMenu.Item className="nav-item" style={{ flexDirection: 'row', justifyContent: 'flex-start' }}>
                + Create Team
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      </div>

      <button className="btn btn--primary" onClick={openComposer} style={{ width: '100%' }}>
        <Plus size={18} />
        New Post
      </button>

      <nav style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 'var(--space-6)' }}>
        <section>
          <p className="eyebrow" style={{ marginBottom: 'var(--space-2)', paddingLeft: 'var(--space-2)' }}>Main</p>
          {MAIN_NAV.map(item => (
            <button
              key={item.id}
              className={`nav-item ${currentSection === item.id ? 'nav-item--active' : ''}`}
              style={{ flexDirection: 'row', width: '100%', justifyContent: 'flex-start', gap: 'var(--space-3)' }}
              onClick={() => setSection(item.id)}
            >
              <item.icon size={20} />
              <span>{item.label}</span>
            </button>
          ))}
        </section>

        <section>
          <p className="eyebrow" style={{ marginBottom: 'var(--space-2)', paddingLeft: 'var(--space-2)' }}>Workspace</p>
          {WORKSPACE_NAV.map(item => (
            <button
              key={item.id}
              className={`nav-item ${currentSection === item.id ? 'nav-item--active' : ''}`}
              style={{ flexDirection: 'row', width: '100%', justifyContent: 'flex-start', gap: 'var(--space-3)' }}
              onClick={() => setSection(item.id)}
            >
              <item.icon size={20} />
              <span>{item.label}</span>
            </button>
          ))}
        </section>
      </nav>

      <div className="sidebar-footer" style={{ borderTop: '1px solid var(--border)', paddingTop: 'var(--space-4)' }}>
        {CONFIG_NAV.map(item => (
          <button
            key={item.id}
            className={`nav-item ${currentSection === item.id ? 'nav-item--active' : ''}`}
            style={{ flexDirection: 'row', width: '100%', justifyContent: 'flex-start', gap: 'var(--space-3)' }}
            onClick={() => setSection(item.id)}
          >
            <item.icon size={20} />
            <span>{item.label}</span>
          </button>
        ))}
        <button 
          className="nav-item" 
          style={{ flexDirection: 'row', width: '100%', justifyContent: 'flex-start', gap: 'var(--space-3)', color: 'var(--danger)' }}
          onClick={onSignOut}
        >
          <LogOut size={20} />
          <span>Sign Out</span>
        </button>
      </div>
    </aside>
  )
}
