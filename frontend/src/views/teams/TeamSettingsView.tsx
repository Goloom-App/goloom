import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { TeamSettingsViewProps } from './teamSettingsTypes'
import { TeamGeneralTab } from './TeamGeneralTab'
import { TeamMembersTab } from './TeamMembersTab'
import { TeamAITab } from './TeamAITab'
import { TeamIntegrationsTab } from './TeamIntegrationsTab'
import { TeamAuditTab } from './TeamAuditTab'

type TeamTab = 'general' | 'members' | 'ai' | 'integrations' | 'audit'

export function TeamSettingsView(props: TeamSettingsViewProps) {
  const { t } = useTranslation()
  const isOwner = props.myRoleInSelectedTeam === 'owner'
  const [requestedTab, setRequestedTab] = useState<TeamTab>('general')
  const tabs: { id: TeamTab; label: string }[] = isOwner ? [
    { id: 'general', label: t('teams.tabGeneral') },
    { id: 'members', label: t('teams.tabMembers') },
    { id: 'ai', label: t('teams.tabAI') },
    { id: 'integrations', label: t('teams.tabIntegrations') },
    { id: 'audit', label: t('teams.tabAudit') },
  ] : [{ id: 'members', label: t('teams.tabMembers') }]
  // Resolve permissions synchronously so a team switch never renders owner-only content.
  const activeTab = tabs.some((tab) => tab.id === requestedTab) ? requestedTab : tabs[0].id
  if (activeTab !== requestedTab) {
    setRequestedTab(activeTab)
  }

  return (
    <div className="brand-wizard stack stack--lg">
      <nav className="view-toggle view-toggle--scrollable" role="tablist" aria-label={t('teams.tabsAria')}>
        {tabs.map((tab) => (
          <button
            key={tab.id}
            id={`team-tab-${tab.id}`}
            type="button"
            role="tab"
            aria-selected={activeTab === tab.id}
            aria-controls="team-tab-panel"
            data-tour={`team-tab-${tab.id}`}
            className={`view-toggle__btn ${activeTab === tab.id ? 'view-toggle__btn--active' : ''}`}
            onClick={() => setRequestedTab(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </nav>
      <div id="team-tab-panel" role="tabpanel" aria-labelledby={`team-tab-${activeTab}`} className="stack stack--lg">
        {activeTab === 'general' && <TeamGeneralTab {...props} />}
        {activeTab === 'members' && <TeamMembersTab key={props.selectedTeam.id} {...props} />}
        {activeTab === 'ai' && <TeamAITab {...props} />}
        {activeTab === 'integrations' && <TeamIntegrationsTab {...props} />}
        {activeTab === 'audit' && <TeamAuditTab key={props.selectedTeam.id} api={props.api} teamId={props.selectedTeam.id} />}
      </div>
    </div>
  )
}
