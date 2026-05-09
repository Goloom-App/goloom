import type { PostRecord, TeamRecord } from '../../types'

export function TeamsView({
  teams,
  posts,
  effectiveSelectedTeamId,
  selectedTeam,
  onSelectTeam,
}: {
  teams: TeamRecord[]
  posts: PostRecord[]
  effectiveSelectedTeamId: string
  selectedTeam: TeamRecord | null
  onSelectTeam: (teamId: string) => void
}) {
  return (
    <div className="teams-view two-column-detail">
      <div className="glass-panel">
        <h2 className="section-card__title">Your teams</h2>
        <p className="hint">Each card shows members, connected accounts, and post activity for that workspace.</p>
        <div className="team-grid">
          {teams.map((team) => {
            const teamPosts = posts.filter((p) => p.teamId === team.id)
            const plannedCount = teamPosts.filter((p) => p.status === 'scheduled').length
            const publishedCount = teamPosts.filter((p) => p.status === 'posted').length
            return (
              <button
                key={team.id}
                type="button"
                className={`team-card ${team.id === effectiveSelectedTeamId ? 'team-card--active' : ''}`}
                onClick={() => onSelectTeam(team.id)}
              >
                <strong>
                  {team.name}
                  {team.isPersonal ? ' · Personal' : ''}
                </strong>
                <small>
                  {team.members.length} members · {team.accountIds.length} accounts
                </small>
                <div className="team-card__stats">
                  <span>{plannedCount} planned</span>
                  <span>{publishedCount} published</span>
                </div>
              </button>
            )
          })}
        </div>
      </div>

      {selectedTeam ? (
        <div className="glass-panel">
          <h2 className="section-card__title">{selectedTeam.name}</h2>
          <p className="hint">{selectedTeam.description || 'No description'}</p>
          {(() => {
            const teamPosts = posts.filter((p) => p.teamId === selectedTeam.id)
            const plannedCount = teamPosts.filter((p) => p.status === 'scheduled').length
            const publishedCount = teamPosts.filter((p) => p.status === 'posted').length
            return (
              <div className="stat-grid mt-1">
                <div className="stat-tile">
                  <span className="stat-tile__label">Planned posts</span>
                  <span className="stat-tile__value">{plannedCount}</span>
                </div>
                <div className="stat-tile">
                  <span className="stat-tile__label">Published</span>
                  <span className="stat-tile__value">{publishedCount}</span>
                </div>
                <div className="stat-tile">
                  <span className="stat-tile__label">Members</span>
                  <span className="stat-tile__value">{selectedTeam.members.length}</span>
                </div>
                <div className="stat-tile">
                  <span className="stat-tile__label">Accounts</span>
                  <span className="stat-tile__value">{selectedTeam.accountIds.length}</span>
                </div>
              </div>
            )
          })()}
          {selectedTeam.isPersonal ? (
            <p className="hint mt-1">
              This is your personal workspace. Invite other users from a shared team instead.
            </p>
          ) : (
            <p className="hint mt-1">
              Open workspace selector in sidebar to create a team or update members, access levels, ownership, and description.
            </p>
          )}
        </div>
      ) : null}
    </div>
  )
}
