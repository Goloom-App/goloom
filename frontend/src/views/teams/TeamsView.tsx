import { useTranslation } from 'react-i18next'

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
  const { t } = useTranslation()

  return (
    <div className="teams-view two-column-detail">
      <div className="glass-panel">
        <h2 className="section-card__title">{t('teams.yourTeams')}</h2>
        <p className="hint">{t('teams.cardsHint')}</p>
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
                  {team.isPersonal ? t('teams.personalSuffix') : ''}
                </strong>
                <small>
                  {t('common.membersCount', { count: team.members.length })} · {t('common.accountsCount', { count: team.accountIds.length })}
                </small>
                <div className="team-card__stats">
                  <span>
                    {plannedCount} {t('common.planned')}
                  </span>
                  <span>
                    {publishedCount} {t('common.published')}
                  </span>
                </div>
              </button>
            )
          })}
        </div>
      </div>

      {selectedTeam ? (
        <div className="glass-panel">
          <h2 className="section-card__title">{selectedTeam.name}</h2>
          <p className="hint">{selectedTeam.description || t('common.noDescription')}</p>
          {(() => {
            const teamPosts = posts.filter((p) => p.teamId === selectedTeam.id)
            const plannedCount = teamPosts.filter((p) => p.status === 'scheduled').length
            const publishedCount = teamPosts.filter((p) => p.status === 'posted').length
            return (
              <div className="stat-grid mt-1">
                <div className="stat-tile">
                  <span className="stat-tile__label">{t('teams.plannedPosts')}</span>
                  <span className="stat-tile__value">{plannedCount}</span>
                </div>
                <div className="stat-tile">
                  <span className="stat-tile__label">{t('teams.published')}</span>
                  <span className="stat-tile__value">{publishedCount}</span>
                </div>
                <div className="stat-tile">
                  <span className="stat-tile__label">{t('common.members')}</span>
                  <span className="stat-tile__value">{selectedTeam.members.length}</span>
                </div>
                <div className="stat-tile">
                  <span className="stat-tile__label">{t('nav.accounts')}</span>
                  <span className="stat-tile__value">{selectedTeam.accountIds.length}</span>
                </div>
              </div>
            )
          })()}
          {selectedTeam.isPersonal ? <p className="hint mt-1">{t('teams.personalWorkspaceHint')}</p> : null}
        </div>
      ) : null}
    </div>
  )
}
