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
      
      <button className="nav-item nav-item--fab" onClick={openComposer}>
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
          <div className="drawer-header">
            <Dialog.Title className="drawer-title">Menu</Dialog.Title>
            <Dialog.Close asChild>
              <button className="btn btn--ghost btn--icon-sm">
                <X size={20} />
              </button>
            </Dialog.Close>
          </div>

          <div className="drawer-body">
            <section>
              <p className="eyebrow drawer-section-label">Workspaces</p>
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
              <p className="eyebrow drawer-section-label">Management</p>
              <div className="drawer-grid">
                {WORKSPACE_NAV.map((item) => (
                  <button
                    key={item.id}
                    className={`btn btn--ghost btn--justify-start ${currentSection === item.id ? 'btn--active' : ''}`}
                    onClick={() => {
                      setSection(item.id)
                      onOpenChange(false)
                    }}
                  >
                    <item.icon size={18} />
                    <span className="drawer-item-label">{item.label}</span>
                  </button>
                ))}
              </div>
            </section>

            <section>
              <p className="eyebrow drawer-section-label">Settings</p>
              <div className="drawer-list">
                {CONFIG_NAV.map((item) => (
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
