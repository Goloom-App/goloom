import { useState } from 'react'
import { Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { SectionCard } from '../../components/ui'
import type { TeamSettingsViewProps } from './teamSettingsTypes'
import type { TeamRole, UserRecord } from '../../types'
import { TeamInvitationsSection } from './TeamInvitationsSection'

function MemberIdentity({ user, userId }: { user?: UserRecord; userId: string }) {
  const [failedPicture, setFailedPicture] = useState<string | null>(null)
  const label = user?.name || user?.email || userId
  const initials = label.trim().split(/\s+/).slice(0, 2).map((part) => part[0]).join('').toUpperCase()
  return (
    <div className="member-identity">
      <span className="avatar avatar--sm" aria-hidden="true">
        {user?.pictureUrl && failedPicture !== user.pictureUrl ? (
          <img className="avatar__img" src={user.pictureUrl} alt="" referrerPolicy="no-referrer" onError={() => setFailedPicture(user.pictureUrl!)} />
        ) : <span className="avatar__placeholder">{initials}</span>}
      </span>
      <span className="member-identity__text">
        <strong>{label}</strong>
        {user?.email && <span className="eyebrow">{user.email}</span>}
      </span>
    </div>
  )
}

export function TeamMembersTab(props: TeamSettingsViewProps) {
  const { t } = useTranslation()
  const {
    selectedTeam,
    myRoleInSelectedTeam,
    principalUser,
    directoryUsers,
    syncing,
    memberRoleEdits,
    setMemberRoleEdits,
    addMemberUserId,
    setAddMemberUserId,
  } = props
  const isOwner = myRoleInSelectedTeam === 'owner'
  const [addMemberOpen, setAddMemberOpen] = useState(false)
  const availableUsers = directoryUsers.filter((user) => !selectedTeam.members.some((member) => member.userId === user.id))

  function toggleAddMember() {
    setAddMemberUserId('')
    setAddMemberOpen((open) => !open)
  }

  return isOwner ? (
    <>
      <SectionCard
        icon={<Users size={18} />}
        title={t('common.members')}
        headerExtra={
          <button
            type="button"
            className="btn btn--secondary btn--sm"
            onClick={toggleAddMember}
            disabled={syncing}
            aria-expanded={addMemberOpen}
          >
            {t('teams.addMember')}
          </button>
        }
      >
        <div className="stack stack--sm">
          {addMemberOpen && (
            <div className="glass-panel glass-panel--compact" data-testid="team-add-member-panel">
              {availableUsers.length === 0 && <p className="hint">{t('teams.noUsersToAdd')}</p>}
              <div className="inline-cluster flex-wrap">
                <select
                  className="grow"
                  aria-label={t('teams.selectUser')}
                  value={addMemberUserId}
                  onChange={(event) => setAddMemberUserId(event.target.value)}
                  disabled={syncing || availableUsers.length === 0}
                >
                  <option value="">{t('teams.selectUser')}</option>
                  {availableUsers.map((user) => (
                    <option key={user.id} value={user.id}>
                      {user.name} ({user.email})
                    </option>
                  ))}
                </select>
                <button
                  type="button"
                  className="btn btn--primary"
                  onClick={() => props.onAddTeamMember(() => setAddMemberOpen(false))}
                  disabled={syncing || !availableUsers.some((user) => user.id === addMemberUserId)}
                >
                  {t('teams.addToTeam')}
                </button>
              </div>
            </div>
          )}
          {selectedTeam.members.map((m) => (
            <div key={m.userId} className="glass-panel glass-panel--compact flex-row--between team-member-row">
              <MemberIdentity user={directoryUsers.find((u) => u.id === m.userId)} userId={m.userId} />
              <div className="inline-cluster">
                <select
                  className="select--sm"
                  value={memberRoleEdits[m.userId] ?? m.role}
                  onChange={(event) =>
                    setMemberRoleEdits((current) => ({ ...current, [m.userId]: event.target.value as TeamRole }))
                  }
                >
                  <option value="owner">{t('roles.owner')}</option>
                  <option value="editor">{t('roles.editor')}</option>
                  <option value="viewer">{t('roles.viewer')}</option>
                </select>
                <button
                  className="btn btn--ghost btn--xs"
                  onClick={() => props.onChangeTeamMemberRole(m.userId)}
                  disabled={syncing || (memberRoleEdits[m.userId] ?? m.role) === m.role}
                >
                  {t('common.apply')}
                </button>
                {m.userId !== principalUser?.id && (
                  <button className="btn btn--xs btn--danger-ghost" onClick={() => props.onRemoveTeamMember(m.userId)}>
                    {t('common.remove')}
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      </SectionCard>

      <TeamInvitationsSection api={props.api} teamId={selectedTeam.id} />
    </>
  ) : (
    <SectionCard icon={<Users size={18} />} title={t('common.members')}>
      <p className="hint">{t('teams.memberViewHint')}</p>
      <div className="stack stack--sm">
        {selectedTeam.members.map((m) => (
          <div key={m.userId} className="member-list-item">
            <MemberIdentity user={directoryUsers.find((u) => u.id === m.userId)} userId={m.userId} />
            <span className="eyebrow">{m.role}</span>
          </div>
        ))}
      </div>
    </SectionCard>
  )
}
