import type { Dispatch, SetStateAction } from 'react'
import { format, isValid, parseISO } from 'date-fns'
import { Icon } from '../../icons'
import type { BackendAdminMetrics } from '../../api'
import type { AccountRecord, ProviderInstanceRecord, ProviderName, RuntimeConfigRecord, UserRecord } from '../../types'
import type { AdminProviderDraft } from './adminTypes'
import { defaultAdminProviderDraft } from './adminTypes'

export function AdminView({
  adminMetrics,
  adminMetricsLoading,
  adminRuntime,
  directoryUsers,
  providerInstances,
  accounts,
  adminProviderDraft,
  setAdminProviderDraft,
  editingProviderId,
  setEditingProviderId,
  showAdminProviderAdvanced,
  setShowAdminProviderAdvanced,
  syncing,
  onSaveAdminProvider,
  onDeleteProviderInstance,
}: {
  adminMetrics: BackendAdminMetrics | null
  adminMetricsLoading: boolean
  adminRuntime: RuntimeConfigRecord | null
  directoryUsers: UserRecord[]
  providerInstances: ProviderInstanceRecord[]
  accounts: AccountRecord[]
  adminProviderDraft: AdminProviderDraft
  setAdminProviderDraft: Dispatch<SetStateAction<AdminProviderDraft>>
  editingProviderId: string | null
  setEditingProviderId: (id: string | null) => void
  showAdminProviderAdvanced: boolean
  setShowAdminProviderAdvanced: (open: boolean) => void
  syncing: boolean
  onSaveAdminProvider: () => void | Promise<void>
  onDeleteProviderInstance: (instanceId: string) => void | Promise<void>
}) {
  return (
    <div className="admin-view two-column-detail">
      <div className="glass-panel">
        <h2 className="section-card__title">Overview</h2>
        {adminMetricsLoading ? <p className="hint">Loading metrics…</p> : null}
        {adminMetrics ? (
          <div className="stat-grid">
            <div className="stat-tile">
              <span className="stat-tile__label">Users</span>
              <span className="stat-tile__value">{adminMetrics.users_count}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">Teams</span>
              <span className="stat-tile__value">{adminMetrics.teams_count}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">Provider instances</span>
              <span className="stat-tile__value">{adminMetrics.provider_instances_count}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">Queued / pending</span>
              <span className="stat-tile__value">{adminMetrics.posts_pending}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">Drafts</span>
              <span className="stat-tile__value">{adminMetrics.posts_draft ?? 0}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">Publishing</span>
              <span className="stat-tile__value">{adminMetrics.posts_processing}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">Posted</span>
              <span className="stat-tile__value">{adminMetrics.posts_posted}</span>
            </div>
            <div className="stat-tile stat-tile--warn">
              <span className="stat-tile__label">Failed</span>
              <span className="stat-tile__value">{adminMetrics.posts_failed}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">Cancelled</span>
              <span className="stat-tile__value">{adminMetrics.posts_cancelled}</span>
            </div>
          </div>
        ) : null}
      </div>

      <div className="glass-panel">
        <h2 className="section-card__title">Registered users</h2>
        <p className="hint">Everyone who can sign in to this deployment.</p>
        <table className="data-table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Email</th>
              <th>Access</th>
              <th>Registered</th>
            </tr>
          </thead>
          <tbody>
            {[...directoryUsers]
              .sort((left, right) => left.name.localeCompare(right.name))
              .map((user) => (
                <tr key={user.id}>
                  <td>{user.name}</td>
                  <td>{user.email}</td>
                  <td>{user.globalRole === 'admin' ? 'Administrator' : 'Member'}</td>
                  <td>{user.createdAt && isValid(parseISO(user.createdAt)) ? format(parseISO(user.createdAt), 'PP') : '—'}</td>
                </tr>
              ))}
          </tbody>
        </table>
        {directoryUsers.length === 0 ? <p className="hint">No users returned from the directory.</p> : null}
      </div>

      {adminRuntime ? (
        <div className="glass-panel">
          <h2 className="section-card__title">Scheduler &amp; server</h2>
          <dl className="kv-list">
            <dt>Worker processes</dt>
            <dd>{adminRuntime.scheduler.workers}</dd>
            <dt>Poll interval</dt>
            <dd>{adminRuntime.scheduler.pollInterval}</dd>
            <dt>HTTP listen</dt>
            <dd>
              <code className="inline-code">{adminRuntime.general.httpAddr}</code>
            </dd>
            <dt>Rate limit / min (anonymous)</dt>
            <dd>{adminRuntime.security.rateLimitPerMinute}</dd>
            <dt>Rate limit / min (Bearer)</dt>
            <dd>{adminRuntime.security.rateLimitAuthenticatedPerMinute}</dd>
          </dl>
        </div>
      ) : null}

      <div className="glass-panel">
        <h2 className="section-card__title">Provider onboarding</h2>
        <p className="hint">
          Register Mastodon, Friendica, or Bluesky instances so teams can connect accounts. Mastodon can auto-discover OAuth endpoints from the
          instance URL when credentials are omitted.
        </p>

        <div className="inline-cluster mb-1">
          <select
            value={adminProviderDraft.provider}
            onChange={(event) => {
              const p = event.target.value as ProviderName
              setAdminProviderDraft((current) => ({
                ...current,
                provider: p,
                instanceUrl: p === 'bluesky' ? 'https://bsky.social' : current.instanceUrl,
              }))
            }}
          >
            <option value="mastodon">Mastodon</option>
            <option value="friendica">Friendica</option>
            <option value="bluesky">Bluesky</option>
          </select>
          {editingProviderId ? (
            <button
              type="button"
              className="button button--secondary"
              onClick={() => {
                setEditingProviderId(null)
                setAdminProviderDraft(defaultAdminProviderDraft())
                setShowAdminProviderAdvanced(false)
              }}
            >
              Cancel edit
            </button>
          ) : null}
        </div>

        <label className="field">
          <span>Display name</span>
          <input value={adminProviderDraft.name} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, name: event.target.value }))} placeholder="My instance" />
        </label>
        <label className="field">
          <span>Instance URL</span>
          <input
            value={adminProviderDraft.instanceUrl}
            onChange={(event) => setAdminProviderDraft((c) => ({ ...c, instanceUrl: event.target.value }))}
            placeholder="https://mastodon.social"
          />
        </label>

        <details className="advanced-config" open={showAdminProviderAdvanced} onToggle={(event) => setShowAdminProviderAdvanced(event.currentTarget.open)}>
          <summary className="advanced-config__summary">Advanced configuration</summary>
          <p className="hint mt-1">
            OAuth client credentials, scopes, and token endpoints. Mastodon can auto-register an app when these are left empty; set them manually for custom apps or
            strict instances.
          </p>
          <label className="field">
            <span>Client ID</span>
            <input
              value={adminProviderDraft.clientId}
              onChange={(event) => setAdminProviderDraft((c) => ({ ...c, clientId: event.target.value }))}
              placeholder="Optional for Mastodon auto-register"
            />
          </label>
          <label className="field">
            <span>Client secret</span>
            <input
              type="password"
              value={adminProviderDraft.clientSecret}
              onChange={(event) => setAdminProviderDraft((c) => ({ ...c, clientSecret: event.target.value }))}
              placeholder="Leave blank to keep existing on update"
            />
          </label>
          <label className="field">
            <span>Scopes (comma-separated)</span>
            <input value={adminProviderDraft.scopes} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, scopes: event.target.value }))} />
          </label>
          <label className="field">
            <span>Authorization endpoint</span>
            <input
              value={adminProviderDraft.authorizationEndpoint}
              onChange={(event) => setAdminProviderDraft((c) => ({ ...c, authorizationEndpoint: event.target.value }))}
            />
          </label>
          <label className="field">
            <span>Token endpoint</span>
            <input value={adminProviderDraft.tokenEndpoint} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, tokenEndpoint: event.target.value }))} />
          </label>
        </details>

        <button type="button" className="button button--primary mt-1" onClick={() => void onSaveAdminProvider()} disabled={syncing}>
          {editingProviderId ? 'Update provider' : 'Register provider'}
        </button>

        <h3 className="subsection-title mt-2">
          Registered instances
        </h3>
        <ul className="provider-admin-list">
          {providerInstances.map((p) => {
            const onboarded = accounts.filter((a) => a.providerInstanceId === p.id).length
            return (
              <li key={p.id}>
                <div>
                  <strong>{p.name}</strong>
                  <span className="hint">
                    {' '}
                    {p.provider} · {p.instanceUrl}
                  </span>
                  <span className="provider-admin-list__count">
                    {onboarded} account{onboarded === 1 ? '' : 's'} onboarded
                  </span>
                </div>
                <div className="inline-cluster flex-shrink-0">
                  <button
                    type="button"
                    className="button button--secondary"
                    onClick={() => {
                      setEditingProviderId(p.id)
                      setShowAdminProviderAdvanced(true)
                      setAdminProviderDraft({
                        provider: p.provider,
                        name: p.name,
                        instanceUrl: p.instanceUrl,
                        clientId: p.clientId,
                        clientSecret: '',
                        scopes: p.scopes.join(','),
                        authorizationEndpoint: p.authorizationEndpoint,
                        tokenEndpoint: p.tokenEndpoint,
                      })
                    }}
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    className="button button--secondary"
                    onClick={() => {
                      if (window.confirm(`Are you sure you want to remove the provider instance "${p.name}"?`)) {
                        void onDeleteProviderInstance(p.id)
                      }
                    }}
                    disabled={syncing || onboarded > 0}
                    title={onboarded > 0 ? 'Disconnect every account using this instance first' : 'Remove provider instance'}
                  >
                    <Icon name="trash" className="inline-icon" />
                    <span>Remove</span>
                  </button>
                </div>
              </li>
            )
          })}
        </ul>
        {providerInstances.length === 0 ? <p className="hint">No provider instances yet.</p> : null}
      </div>
    </div>
  )
}
