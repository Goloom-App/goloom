import { Icon } from '../../icons'
import type { IconName } from '../../icons'
import type { AppSection, TeamRecord, UserRecord } from '../../types'

export function AppSidebar({
  section,
  setSection,
  sidebarContentNav,
  sidebarWorkspaceNav,
  sidebarConfigNav,
  principalUser,
  selectedTeam,
  syncing,
  selectedTeamPresent,
  onCreatePost,
  onSignOut,
}: {
  section: AppSection
  setSection: (s: AppSection) => void
  sidebarContentNav: { id: AppSection; label: string; icon: IconName }[]
  sidebarWorkspaceNav: { id: AppSection; label: string; icon: IconName }[]
  sidebarConfigNav: { id: AppSection; label: string; icon: IconName }[]
  principalUser: UserRecord | null
  selectedTeam: TeamRecord | null
  syncing: boolean
  selectedTeamPresent: boolean
  onCreatePost: () => void
  onSignOut: () => void
}) {
  return (
    <aside className="app-sidebar" aria-label="Main navigation">
      <div className="app-sidebar__header">
        <div className="app-sidebar__logo" title="goloom" aria-hidden="true">
          <span className="app-sidebar__logo-layer app-sidebar__logo-layer--a" />
          <span className="app-sidebar__logo-layer app-sidebar__logo-layer--b" />
          <span className="app-sidebar__logo-layer app-sidebar__logo-layer--c" />
        </div>
        <span className="app-sidebar__title">goloom</span>
      </div>

      <button type="button" className="app-sidebar__cta" onClick={onCreatePost} disabled={!selectedTeamPresent || syncing}>
        <span className="app-sidebar__cta-icon" aria-hidden="true">
          <Icon name="plus" className="inline-icon" />
        </span>
        <span>Create post</span>
      </button>

      <nav className="app-sidebar__nav" aria-label="Sections">
        <div className="app-sidebar__nav-group">
          <p className="app-sidebar__nav-heading">Content</p>
          <ul className="app-sidebar__nav-list">
            {sidebarContentNav.map((item) => (
              <li key={item.id}>
                <button
                  type="button"
                  className={`app-sidebar__link ${section === item.id ? 'app-sidebar__link--active' : ''}`}
                  onClick={() => setSection(item.id)}
                >
                  <Icon name={item.icon} className="app-sidebar__link-icon" />
                  <span>{item.label}</span>
                </button>
              </li>
            ))}
          </ul>
        </div>

        <div className="app-sidebar__divider" role="presentation" />

        <div className="app-sidebar__nav-group">
          <p className="app-sidebar__nav-heading">Workspace</p>
          <ul className="app-sidebar__nav-list">
            {sidebarWorkspaceNav.map((item) => (
              <li key={item.id}>
                <button
                  type="button"
                  className={`app-sidebar__link ${section === item.id ? 'app-sidebar__link--active' : ''}`}
                  onClick={() => setSection(item.id)}
                >
                  <Icon name={item.icon} className="app-sidebar__link-icon" />
                  <span>{item.label}</span>
                </button>
              </li>
            ))}
          </ul>
        </div>

        <div className="app-sidebar__divider" role="presentation" />

        <div className="app-sidebar__nav-group">
          <p className="app-sidebar__nav-heading">Configuration</p>
          <ul className="app-sidebar__nav-list">
            {sidebarConfigNav.map((item) => (
              <li key={item.id}>
                <button
                  type="button"
                  className={`app-sidebar__link ${section === item.id ? 'app-sidebar__link--active' : ''}`}
                  onClick={() => setSection(item.id)}
                >
                  <Icon name={item.icon} className="app-sidebar__link-icon" />
                  <span>{item.label}</span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      </nav>

      <div className="app-sidebar__footer">
        <div className="app-sidebar__user">
          <div className="app-sidebar__avatar" aria-hidden="true">
            {initialsFromName(principalUser?.name ?? '')}
          </div>
          <div className="app-sidebar__user-text">
            <span className="app-sidebar__user-name">{principalUser?.name ?? 'Signed in'}</span>
            <span className="app-sidebar__user-meta">{selectedTeam?.name ?? principalUser?.email ?? '—'}</span>
          </div>
        </div>
        <button type="button" className="app-sidebar__theme" onClick={onSignOut} title="Sign out" aria-label="Sign out">
          <Icon name="lock" className="inline-icon" />
        </button>
      </div>
    </aside>
  )
}

function initialsFromName(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return '?'
  }
  if (parts.length === 1) {
    return parts[0]!.slice(0, 2).toUpperCase()
  }
  return `${parts[0]![0] ?? ''}${parts[parts.length - 1]![0] ?? ''}`.toUpperCase() || '?'
}
