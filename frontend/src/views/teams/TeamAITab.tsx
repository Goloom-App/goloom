import { Bot } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { SectionCard, ToggleSwitch } from '../../components/ui'
import type { TeamSettingsViewProps } from './teamSettingsTypes'

export function TeamAITab(props: TeamSettingsViewProps) {
  const { t } = useTranslation()
  const { syncing } = props
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
          placeholder={props.teamAiProvider === 'anthropic' ? 'claude-sonnet-5-5' : 'gpt-5'}
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

  return (
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
  )
}
