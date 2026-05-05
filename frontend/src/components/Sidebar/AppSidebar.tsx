import { GoloomLogo } from '../brand/GoloomLogo'
import { Icon } from '../../icons'
import type { IconName } from '../../icons'
import type { AppSection, TeamRecord, UserRecord } from '../../types'

export function AppSidebar({
  section,
  setSection,
  sidebarContentNav,
  sidebarWorkspaceNav,
  sidebarConfigNav,
  teams,
  principalUser,
  effectiveSelectedTeamId,
  selectedTeam,
  syncing,
  selectedTeamPresent,
  onSelectTeam,
  onOpenTeamSettings,
  onCreatePost,
  onSignOut,
}: {
  section: AppSection
  setSection: (s: AppSection) => void
  sidebarContentNav: { id: AppSection; label: string; icon: IconName }[]
  sidebarWorkspaceNav: { id: AppSection; label: string; icon: IconName }[]
  sidebarConfigNav: { id: AppSection; label: string; icon: IconName }[]
  teams: TeamRecord[]
  principalUser: UserRecord | null
  effectiveSelectedTeamId: string
  selectedTeam: TeamRecord | null
  syncing: boolean
  selectedTeamPresent: boolean
  onSelectTeam: (teamId: string) => void
  onOpenTeamSettings: () => void
  onCreatePost: () => void
  onSignOut: () => void
}) {
  return (
    <aside className="app-sidebar" aria-label="Main navigation">
      <div className="app-sidebar__header">
        <GoloomLogo />
        <span className="app-sidebar__title">goloom</span>
      </div>

      <div className="app-sidebar__workspace-picker">
        <label className="app-sidebar__workspace-label" htmlFor="workspace-select">
          Workspace
        </label>
        <div className="app-sidebar__workspace-row">
          <select
            id="workspace-select"
            className="app-sidebar__workspace-select"
            value={effectiveSelectedTeamId}
            onChange={(event) => {
              const value = event.target.value
              if (value === '__create_team__') {
                onOpenTeamSettings()
                return
              }
              onSelectTeam(value)
            }}
            disabled={teams.length === 0}
          >
            {teams.length === 0 ? (
              <option value="">No team loaded</option>
            ) : (
              <>
                {teams.map((team) => (
                  <option key={team.id} value={team.id}>
                    {team.name}
                  </option>
                ))}
                <option value="__create_team__">+ Create new team…</option>
              </>
            )}
          </select>
          <button
            type="button"
            className="app-sidebar__workspace-add"
            onClick={onOpenTeamSettings}
            aria-label="Create or manage teams"
            title="Create or manage teams"
          >
            <Icon name="plus-circle" className="inline-icon" />
          </button>
        </div>
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
