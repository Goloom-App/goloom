import * as Dialog from '@radix-ui/react-dialog'
import { Plus, Menu, X } from 'lucide-react'
import { MAIN_NAV, WORKSPACE_NAV, CONFIG_NAV } from './NavItems'
import type { AppSection, TeamRecord, UserRecord } from '../../types'

interface BottomNavProps {
  currentSection: AppSection
  setSection: (section: AppSection) => void
  openComposer: () => void
  openDrawer: () => void
}

export function BottomNav({ currentSection, setSection, openComposer, openDrawer }: BottomNavProps) {
  return (
    <nav className="bottom-nav">
      {MAIN_NAV.slice(0, 2).map((item) => (
        <button
          key={item.id}
          className={`nav-item ${currentSection === item.id ? 'nav-item--active' : ''}`}
          onClick={() => setSection(item.id)}
        >
          <item.icon />
          <span>{item.label}</span>
        </button>
      ))}
      
      <button className="nav-item" onClick={openComposer} style={{ color: 'var(--accent)' }}>
        <Plus size={32} strokeWidth={3} />
        <span>Post</span>
      </button>

      {MAIN_NAV.slice(2, 4).map((item) => (
        <button
          key={item.id}
          className={`nav-item ${currentSection === item.id ? 'nav-item--active' : ''}`}
          onClick={() => setSection(item.id)}
        >
          <item.icon />
          <span>{item.label}</span>
        </button>
      ))}

      <button className="nav-item" onClick={openDrawer}>
        <Menu />
        <span>More</span>
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
  onSelectTeam,
  user,
  onSignOut
}: MobileDrawerProps) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content" data-side="bottom">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-6)' }}>
            <Dialog.Title style={{ fontSize: '1.25rem' }}>Menu</Dialog.Title>
            <Dialog.Close asChild>
              <button className="btn btn--ghost" style={{ padding: '0.5rem' }}>
                <X size={20} />
              </button>
            </Dialog.Close>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-8)' }}>
            <section>
              <p className="eyebrow" style={{ marginBottom: 'var(--space-2)' }}>Workspaces</p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                {teams.map((team) => (
                  <button
                    key={team.id}
                    className="btn btn--ghost"
                    style={{ 
                      justifyContent: 'flex-start', 
                      width: '100%',
                      background: team.id === selectedTeamId ? 'var(--accent-soft)' : 'transparent',
                      color: team.id === selectedTeamId ? 'var(--accent)' : 'var(--text)'
                    }}
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
              <p className="eyebrow" style={{ marginBottom: 'var(--space-2)' }}>Management</p>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-2)' }}>
                {WORKSPACE_NAV.map((item) => (
                  <button
                    key={item.id}
                    className="btn btn--ghost"
                    style={{ 
                      justifyContent: 'flex-start',
                      background: currentSection === item.id ? 'var(--accent-soft)' : 'var(--surface-muted)' 
                    }}
                    onClick={() => {
                      setSection(item.id)
                      onOpenChange(false)
                    }}
                  >
                    <item.icon size={18} />
                    <span style={{ fontSize: '0.85rem' }}>{item.label}</span>
                  </button>
                ))}
              </div>
            </section>

            <section>
              <p className="eyebrow" style={{ marginBottom: 'var(--space-2)' }}>Settings</p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                {CONFIG_NAV.map((item) => (
                  <button
                    key={item.id}
                    className="btn btn--ghost"
                    style={{ 
                      justifyContent: 'flex-start',
                      background: currentSection === item.id ? 'var(--accent-soft)' : 'transparent'
                    }}
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

            <footer style={{ paddingTop: 'var(--space-4)', borderTop: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }}>
                <div style={{ width: 32, height: 32, borderRadius: '50%', background: 'var(--accent)', display: 'grid', placeItems: 'center', fontWeight: 'bold', fontSize: '0.8rem' }}>
                  {user?.name?.[0] || '?'}
                </div>
                <div style={{ display: 'flex', flexDirection: 'column' }}>
                  <span style={{ fontSize: '0.9rem', fontWeight: 600 }}>{user?.name}</span>
                  <span style={{ fontSize: '0.75rem', color: 'var(--text-dim)' }}>{user?.email}</span>
                </div>
              </div>
              <button className="btn btn--ghost" onClick={onSignOut} style={{ color: 'var(--danger)' }}>
                Sign Out
              </button>
            </footer>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
