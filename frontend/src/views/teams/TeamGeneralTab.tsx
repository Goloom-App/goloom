import { Palette, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { SectionCard } from '../../components/ui'
import type { TeamSettingsViewProps } from './teamSettingsTypes'

const DEFAULT_PICKER_COLOR = '#8b5cf6'

export function TeamGeneralTab(props: TeamSettingsViewProps) {
  const { t } = useTranslation()
  const { selectedTeam, syncing } = props
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
    </>
  )
}
