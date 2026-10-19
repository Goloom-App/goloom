import { Activity } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { SectionCard, ToggleSwitch } from '../../components/ui'
import type { TeamSettingsViewProps } from './teamSettingsTypes'

export function TeamIntegrationsTab(props: TeamSettingsViewProps) {
  const { t } = useTranslation()
  return (
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
}
