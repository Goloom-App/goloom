import type { Dispatch, SetStateAction } from 'react'
import { format, isValid, parseISO } from 'date-fns'
import { SettingsCard } from '../../components/settings/SettingsCard'
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
  newTokenPlaintext,
  setNewTokenPlaintext,
  newApiTokenName,
  setNewApiTokenName,
  newApiTokenExpiresYmd,
  setNewApiTokenExpiresYmd,
  onCreateApiToken,
  onRevokeApiToken,
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
  onCreateApiToken: () => void | Promise<void>
  onRevokeApiToken: (tokenID: string) => void | Promise<void>
  apiTokens: BackendAPIToken[]
  apiTokensLoading: boolean
}) {
  return (
    <div className="settings-view two-column-detail">
      <div className="glass-panel">
        <SettingsCard title="Browser session">
          <label className="field">
            <span>API base URL (optional)</span>
            <input value={settings.general.apiBaseUrl} onChange={(event) => updateAPIBaseURL(event.target.value)} />
          </label>
          <label className="field">
            <span>Bearer token (OIDC ID token, bootstrap, or API token)</span>
            <input
              type="password"
              value={settings.general.bearerToken}
              onChange={(event) => setSettings((current) => ({ ...current, general: { ...current.general, bearerToken: event.target.value } }))}
            />
          </label>
          <div className="inline-cluster mt-1">
            <button type="button" className="button button--primary" onClick={connectBackend}>
              Apply session
            </button>
            <button type="button" className="button button--secondary" onClick={() => void loadDashboard()} disabled={!apiPresent || syncing}>
              Refresh data
            </button>
          </div>
        </SettingsCard>
      </div>

      <div className="glass-panel">
        <h2 className="section-card__title">API tokens</h2>
        <p className="hint">
          Tokens authenticate as <strong>you</strong>, not a team. Team access follows your memberships. Use{' '}
          <code className="inline-code">Authorization: Bearer &lt;token&gt;</code> on every request. Create automation tokens here; each value is
          shown only once.
        </p>
        {newTokenPlaintext ? (
          <div className="token-reveal">
            <p className="hint">Copy this secret now:</p>
            <code className="token-reveal__value">{newTokenPlaintext}</code>
            <button type="button" className="button button--secondary" onClick={() => setNewTokenPlaintext(null)}>
              Dismiss
            </button>
          </div>
        ) : null}
        <div className="flex-row--wrap mt-1">
          <label className="field min-w-12">
            <span>Label</span>
            <input value={newApiTokenName} onChange={(event) => setNewApiTokenName(event.target.value)} placeholder="e.g. CI, laptop" />
          </label>
          <label className="field min-w-11">
            <span>Expires (UTC end of day)</span>
            <input type="date" value={newApiTokenExpiresYmd} onChange={(event) => setNewApiTokenExpiresYmd(event.target.value)} />
          </label>
          <button type="button" className="button button--primary" onClick={() => void onCreateApiToken()} disabled={syncing || !newApiTokenName.trim()}>
            Create token
          </button>
        </div>
        <p className="hint">Expiry uses end of the selected calendar day in UTC (default picker value is 90 days ahead).</p>
        {apiTokensLoading ? <p className="hint">Loading tokens…</p> : null}
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Created</th>
              <th>Expires</th>
              <th>Last used</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {apiTokens.map((t) => (
              <tr key={t.id}>
                <td>{t.name}</td>
                <td>{format(parseISO(t.created_at), 'PPp')}</td>
                <td>{t.expires_at && isValid(parseISO(t.expires_at)) ? format(parseISO(t.expires_at), 'PPp') : '—'}</td>
                <td>{t.last_used_at ? format(parseISO(t.last_used_at), 'PPp') : '—'}</td>
                <td>
                  <button
                    type="button"
                    className="button button--secondary"
                    onClick={() => {
                      if (window.confirm(`Are you sure you want to revoke the token "${t.name}"?`)) {
                        void onRevokeApiToken(t.id)
                      }
                    }}
                    disabled={syncing}
                  >
                    Revoke
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {apiTokens.length === 0 && !apiTokensLoading ? <p className="hint">No API tokens yet.</p> : null}
      </div>
    </div>
  )
}
