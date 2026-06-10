import type { Dispatch, SetStateAction } from 'react'
import { format, isValid, parseISO } from 'date-fns'
import { useTranslation } from 'react-i18next'

import { setAppLanguage, supportedLanguages, type SupportedLanguage } from '../../i18n'
import { SettingsCard } from '../../components/settings/SettingsCard'
import { OptionPill } from '../../components/ui'
import type { BackendAPIToken } from '../../api'
import { apiTokenDisplayName, isApiTokenExpired } from './apiTokens'
import type { SettingsState } from '../../types'

const AI_SCOPES = ['ai:read:context', 'ai:write:drafts', 'ai:trigger:jobs'] as const

function scopeLabel(scope: string): string {
  const map: Record<string, string> = {
    'ai:read:context': 'Read AI context',
    'ai:write:drafts': 'Create post drafts',
    'ai:trigger:jobs': 'Trigger AI jobs',
  }
  return map[scope] ?? scope
}

export function SettingsView({
  settings,
  setSettings,
  updateAPIBaseURL,
  connectBackend,
  loadDashboard,
  apiPresent,
  syncing,
  newTokenPlaintext,
  setNewTokenPlaintext,
  newApiTokenName,
  setNewApiTokenName,
  newApiTokenExpiresYmd,
  setNewApiTokenExpiresYmd,
  newApiTokenScopes,
  setNewApiTokenScopes,
  onCreateApiToken,
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
  newTokenPlaintext: string | null
  setNewTokenPlaintext: (v: string | null) => void
  newApiTokenName: string
  setNewApiTokenName: (v: string) => void
  newApiTokenExpiresYmd: string
  setNewApiTokenExpiresYmd: (v: string) => void
  newApiTokenScopes: string[]
  setNewApiTokenScopes: (v: string[]) => void
  onCreateApiToken: () => void | Promise<void>
  onRemoveApiToken: (tokenID: string, expired: boolean) => void | Promise<void>
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

      <div className="glass-panel">
        <h2 className="section-card__title">{tr('settings.apiTokens')}</h2>
        <p className="hint">{tr('settings.apiTokensHint')}</p>
        {newTokenPlaintext ? (
          <div className="token-reveal">
            <p className="hint">{tr('settings.copySecret')}</p>
            <code className="token-reveal__value">{newTokenPlaintext}</code>
            <button type="button" className="button button--secondary" onClick={() => setNewTokenPlaintext(null)}>
              {tr('common.dismiss')}
            </button>
          </div>
        ) : null}
        <div className="flex-row--wrap mt-1">
          <label className="field min-w-12">
            <span>{tr('settings.label')}</span>
            <input
              value={newApiTokenName}
              onChange={(event) => setNewApiTokenName(event.target.value)}
              placeholder={tr('settings.labelPlaceholder')}
            />
          </label>
          <label className="field min-w-11">
            <span>{tr('settings.expiresUtc')}</span>
            <input type="date" value={newApiTokenExpiresYmd} onChange={(event) => setNewApiTokenExpiresYmd(event.target.value)} />
          </label>
          <button type="button" className="button button--primary" onClick={() => void onCreateApiToken()} disabled={syncing || !newApiTokenName.trim()}>
            {tr('settings.createToken')}
          </button>
        </div>
        <div className="field">
          <span>Scopes (optional)</span>
          <div className="brand-option-grid">
            {AI_SCOPES.map((scope) => {
              const checked = newApiTokenScopes.includes(scope)
              return (
                <OptionPill
                  key={scope}
                  active={checked}
                  onClick={() =>
                    setNewApiTokenScopes(
                      checked
                        ? newApiTokenScopes.filter((s) => s !== scope)
                        : [...newApiTokenScopes, scope],
                    )
                  }
                >
                  {scopeLabel(scope)}
                </OptionPill>
              )
            })}
          </div>
        </div>
        {apiTokensLoading ? <p className="hint">{tr('common.loadingTokens')}</p> : null}
        <table className="data-table">
          <thead>
            <tr>
              <th>{tr('settings.label')}</th>
              <th>Scopes</th>
              <th>Team</th>
              <th>{tr('settings.created')}</th>
              <th>{tr('settings.expires')}</th>
              <th>{tr('settings.lastUsed')}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {apiTokens.map((t) => {
              const expired = isApiTokenExpired(t)
              const label = apiTokenDisplayName(t.name)
              return (
                <tr key={t.id}>
                  <td>{label}</td>
                  <td>{t.scopes && t.scopes.length > 0 ? t.scopes.map(scopeLabel).join(', ') : '—'}</td>
                  <td>{t.team_id ?? '—'}</td>
                  <td>{format(parseISO(t.created_at), 'PPp')}</td>
                  <td>{t.expires_at && isValid(parseISO(t.expires_at)) ? format(parseISO(t.expires_at), 'PPp') : '—'}</td>
                  <td>{t.last_used_at ? format(parseISO(t.last_used_at), 'PPp') : '—'}</td>
                  <td>
                    <button
                      type="button"
                      className="button button--secondary"
                      onClick={() => {
                        if (
                          window.confirm(
                            expired
                              ? tr('settings.confirmDelete', { label })
                              : tr('settings.confirmRevoke', { label }),
                          )
                        ) {
                          void onRemoveApiToken(t.id, expired)
                        }
                      }}
                      disabled={syncing}
                    >
                      {expired ? tr('common.delete') : tr('settings.revoke')}
                    </button>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
        {apiTokens.length === 0 && !apiTokensLoading ? <p className="hint">{tr('settings.noTokens')}</p> : null}
      </div>
    </div>
  )
}
