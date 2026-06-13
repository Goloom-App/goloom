import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'

import { setAppLanguage, supportedLanguages, type SupportedLanguage } from '../../i18n'
import { SettingsCard } from '../../components/settings/SettingsCard'
import { ApiTokenManager, type ApiTokenManagerTeam, type CreateApiTokenPayload } from '../../components/settings/ApiTokenManager'
import type { BackendAPIToken } from '../../api'
import type { SettingsState } from '../../types'

export function SettingsView({
  settings,
  setSettings,
  updateAPIBaseURL,
  connectBackend,
  loadDashboard,
  apiPresent,
  syncing,
  teams,
  createApiToken,
  onRemoveApiToken,
  apiTokens,
  apiTokensLoading,
}: {
  settings: SettingsState
  setSettings: Dispatch<SetStateAction<SettingsState>>
  updateAPIBaseURL: (value: string) => void
  connectBackend: () => void
  loadDashboard: () => void | Promise<void>
  apiPresent: boolean
  syncing: boolean
  teams: ApiTokenManagerTeam[]
  createApiToken: (payload: CreateApiTokenPayload) => Promise<string>
  onRemoveApiToken: (tokenID: string, expired: boolean) => Promise<void>
  apiTokens: BackendAPIToken[]
  apiTokensLoading: boolean
}) {
  const { t: tr } = useTranslation()

  return (
    <div className="settings-view two-column-detail">
      <div className="glass-panel">
        <SettingsCard title={tr('language.label')}>
          <label className="field">
            <span>{tr('language.label')}</span>
            <select
              value={settings.ui.language ?? 'en'}
              onChange={(event) => {
                const language = event.target.value as SupportedLanguage
                setSettings((current) => ({ ...current, ui: { ...current.ui, language } }))
                setAppLanguage(language)
              }}
            >
              {supportedLanguages.map((lang) => (
                <option key={lang.code} value={lang.code}>
                  {lang.label}
                </option>
              ))}
            </select>
          </label>
          <p className="hint">{tr('language.hint')}</p>
        </SettingsCard>
      </div>

      <div className="glass-panel">
        <SettingsCard title={tr('settings.browserSession')}>
          <label className="field">
            <span>{tr('settings.apiBaseUrl')}</span>
            <input value={settings.general.apiBaseUrl} onChange={(event) => updateAPIBaseURL(event.target.value)} />
          </label>
          <label className="field">
            <span>{tr('settings.bearerToken')}</span>
            <input
              type="password"
              value={settings.general.bearerToken}
              onChange={(event) => setSettings((current) => ({ ...current, general: { ...current.general, bearerToken: event.target.value } }))}
            />
          </label>
          <div className="inline-cluster mt-1">
            <button type="button" className="button button--primary" onClick={connectBackend}>
              {tr('settings.applySession')}
            </button>
            <button type="button" className="button button--secondary" onClick={() => void loadDashboard()} disabled={!apiPresent || syncing}>
              {tr('settings.refreshData')}
            </button>
          </div>
        </SettingsCard>
      </div>

      <ApiTokenManager
        teams={teams}
        tokens={apiTokens}
        loading={apiTokensLoading}
        syncing={syncing}
        createToken={createApiToken}
        removeToken={onRemoveApiToken}
      />
    </div>
  )
}
