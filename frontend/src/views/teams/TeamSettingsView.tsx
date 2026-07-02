import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'
import { Activity, Bot, Palette, Settings, Users } from 'lucide-react'

import { SectionCard, ToggleSwitch } from '../../components/ui'
import type { createApiClient } from '../../api'
import type { TeamRecord, TeamRole, UserRecord } from '../../types'
import { TeamAuditLogSection } from './TeamAuditLogSection'
import { TeamInvitationsSection } from './TeamInvitationsSection'

interface TeamSettingsViewProps {
  api: ReturnType<typeof createApiClient> | null
  selectedTeam: TeamRecord
  myRoleInSelectedTeam: TeamRole | null
  principalUser: UserRecord | null
  directoryUsers: UserRecord[]
  syncing: boolean
  teamSettingsName: string
  setTeamSettingsName: (value: string) => void
  teamSettingsDescription: string
  setTeamSettingsDescription: (value: string) => void
  teamAiEnabled: boolean
  setTeamAiEnabled: (value: boolean) => void
  teamAiProvider: string
  setTeamAiProvider: (value: string) => void
  teamAiModel: string
  setTeamAiModel: (value: string) => void
  teamAiApiKey: string
  setTeamAiApiKey: (value: string) => void
  teamAiBaseUrl: string
  setTeamAiBaseUrl: (value: string) => void
  teamAiKeySet: boolean
  teamBrandColor: string
  setTeamBrandColor: (value: string) => void
  externalPostMonitorEnabled: boolean
  onToggleExternalPostMonitor: (enabled: boolean) => void
  onOpenImportOldPosts: () => void
  memberRoleEdits: Record<string, TeamRole>
  setMemberRoleEdits: Dispatch<SetStateAction<Record<string, TeamRole>>>
  addMemberUserId: string
  setAddMemberUserId: (value: string) => void
  onUpdateTeam: () => void
  onSaveAiServiceConfig: () => void
  onSaveBrandColor: (color: string) => void
  onAddTeamMember: () => void
  onRemoveTeamMember: (userId: string) => void
  onChangeTeamMemberRole: (userId: string) => void
}

const DEFAULT_PICKER_COLOR = '#8b5cf6'

export function TeamSettingsView(props: TeamSettingsViewProps) {
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

  function directoryUserLabel(userId: string) {
    const user = directoryUsers.find((u) => u.id === userId)
    return user ? `${user.name} · ${user.email}` : userId
  }

  const aiConfigFields = (
    <div className="stack stack--sm mt-1">
      <h4 className="subsection-title">{t('teams.aiProviderTitle')}</h4>
      <p className="hint">{t('teams.aiProviderHint')}</p>
      <label className="field">
        <span>{t('teams.aiProviderLabel')}</span>
        <select value={props.teamAiProvider} onChange={(event) => props.setTeamAiProvider(event.target.value)}>
          <option value="openai">OpenAI</option>
          <option value="anthropic">Anthropic</option>
        </select>
      </label>
      <label className="field">
        <span>{t('teams.aiModelLabel')}</span>
        <input
          value={props.teamAiModel}
          onChange={(event) => props.setTeamAiModel(event.target.value)}
          placeholder={props.teamAiProvider === 'anthropic' ? 'claude-opus-4-8' : 'gpt-4o'}
        />
      </label>
      <label className="field">
        <span>{t('teams.aiApiKeyLabel')}</span>
        <input
          type="password"
          value={props.teamAiApiKey}
          onChange={(event) => props.setTeamAiApiKey(event.target.value)}
          placeholder={props.teamAiKeySet ? t('teams.aiApiKeyStored') : 'sk-…'}
          autoComplete="off"
        />
      </label>
      <label className="field">
        <span>{t('teams.aiBaseUrlLabel')}</span>
        <input
          value={props.teamAiBaseUrl}
          onChange={(event) => props.setTeamAiBaseUrl(event.target.value)}
          placeholder={t('teams.aiBaseUrlPlaceholder')}
        />
      </label>
    </div>
  )

  const externalPostMonitorCard = (
    <SectionCard
      icon={<Activity size={18} />}
      title={t('teams.externalPostMonitorTitle')}
      subtitle={t('teams.externalPostMonitorHint')}
    >
      <ToggleSwitch
        checked={props.externalPostMonitorEnabled}
        onChange={(next) => props.onToggleExternalPostMonitor(next)}
        title={t('teams.externalPostMonitorLabel')}
        testId="team-external-monitor-toggle"
      />
      {props.externalPostMonitorEnabled && (
        <div>
          <button type="button" className="btn btn--secondary btn--sm" onClick={props.onOpenImportOldPosts}>
            {t('teams.importOldPostsButton', 'Import old posts')}
          </button>
        </div>
      )}
    </SectionCard>
  )

  const savedColor = selectedTeam.brandColor ?? ''
  const brandingCard = (
    <SectionCard icon={<Palette size={18} />} title={t('teams.brandingTitle')} subtitle={t('teams.brandingHint')}>
      <div className="brand-color-row">
        <input
          type="color"
          value={props.teamBrandColor || DEFAULT_PICKER_COLOR}
          onChange={(event) => props.setTeamBrandColor(event.target.value)}
          aria-label={t('teams.brandColorLabel')}
          data-testid="team-brand-color-input"
        />
        <span className="brand-color-value">{props.teamBrandColor || '—'}</span>
        <button
          type="button"
          className="btn btn--primary btn--sm"
          onClick={() => props.onSaveBrandColor(props.teamBrandColor)}
          disabled={syncing || !props.teamBrandColor || props.teamBrandColor === savedColor}
          data-testid="team-brand-color-save"
        >
          {t('teams.saveChanges')}
        </button>
        <button
          type="button"
          className="btn btn--ghost btn--sm"
          onClick={() => {
            props.setTeamBrandColor('')
            props.onSaveBrandColor('')
          }}
          disabled={syncing || !savedColor}
        >
          {t('teams.brandColorReset')}
        </button>
      </div>
    </SectionCard>
  )

  return (
    <div className="brand-wizard stack stack--lg">
      <SectionCard
        icon={<Settings size={18} />}
        title={t('teams.settingsTitle')}
        subtitle={t('teams.settingsHint', { teamName: selectedTeam.name })}
      >
        {!isOwner ? <p className="hint">{t('teams.memberViewHint')}</p> : null}
      </SectionCard>

      {isOwner ? (
        <>
          <SectionCard icon={<Settings size={18} />} title={t('common.general')}>
            <label className="field">
              <span>{t('teams.teamName')}</span>
              <input value={props.teamSettingsName} onChange={(event) => props.setTeamSettingsName(event.target.value)} />
            </label>
            <label className="field">
              <span>{t('common.description')}</span>
              <textarea
                rows={3}
                value={props.teamSettingsDescription}
                onChange={(event) => props.setTeamSettingsDescription(event.target.value)}
                placeholder={t('teams.descriptionPlaceholder')}
              />
            </label>
            <div>
              <button
                type="button"
                className="btn btn--primary"
                onClick={props.onUpdateTeam}
                disabled={syncing || !props.teamSettingsName.trim()}
              >
                {t('teams.saveChanges')}
              </button>
            </div>
          </SectionCard>

          {brandingCard}

          {externalPostMonitorCard}

          <SectionCard icon={<Bot size={18} />} title={t('teams.aiAgentTitle')} subtitle={t('teams.aiAgentHint')}>
            <ToggleSwitch
              checked={props.teamAiEnabled}
              onChange={props.setTeamAiEnabled}
              title={t('teams.aiFeaturesEnabled')}
              testId="team-ai-toggle"
            />

            {props.teamAiEnabled && (
              <>
                {aiConfigFields}
                <div className="mt-1">
                  <button
                    type="button"
                    className="btn btn--primary"
                    onClick={props.onSaveAiServiceConfig}
                    disabled={syncing || (!props.teamAiApiKey.trim() && !props.teamAiKeySet)}
                  >
                    {t('teams.saveAiConfig')}
                  </button>
                </div>
              </>
            )}
          </SectionCard>

          <SectionCard icon={<Users size={18} />} title={t('common.members')}>
            <div className="stack stack--sm">
              {selectedTeam.members.map((m) => (
                <div key={m.userId} className="glass-panel glass-panel--compact flex-row--between">
                  <div>
                    <strong>{directoryUserLabel(m.userId)}</strong>
                    <p className="eyebrow">{m.role}</p>
                  </div>
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

          <SectionCard icon={<Users size={18} />} title={t('teams.addMember')}>
            <div className="inline-cluster flex-wrap">
              <select className="grow" value={addMemberUserId} onChange={(event) => setAddMemberUserId(event.target.value)}>
                <option value="">{t('teams.selectUser')}</option>
                {directoryUsers
                  .filter((u) => !selectedTeam.members.some((m) => m.userId === u.id))
                  .map((u) => (
                    <option key={u.id} value={u.id}>
                      {u.name} ({u.email})
                    </option>
                  ))}
              </select>
              <button className="btn btn--primary" onClick={props.onAddTeamMember} disabled={syncing || !addMemberUserId}>
                {t('teams.addToTeam')}
              </button>
            </div>
          </SectionCard>

          <TeamInvitationsSection api={props.api} teamId={selectedTeam.id} />

          <TeamAuditLogSection api={props.api} teamId={selectedTeam.id} />
        </>
      ) : (
        <SectionCard icon={<Users size={18} />} title={t('common.members')}>
          <div className="stack stack--sm">
            {selectedTeam.members.map((m) => (
              <div key={m.userId} className="member-list-item">
                {directoryUserLabel(m.userId)} ({m.role})
              </div>
            ))}
          </div>
        </SectionCard>
      )}
    </div>
  )
}
