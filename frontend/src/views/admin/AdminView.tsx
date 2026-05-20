import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'
import { format, isValid, parseISO } from 'date-fns'
import { Icon } from '../../icons'
import type { BackendAdminMetrics, BackendAdminSyncStatus } from '../../api'
import type { AccountRecord, ProviderInstanceRecord, ProviderName, RuntimeConfigRecord, UserRecord } from '../../types'
import type { AdminProviderDraft } from './adminTypes'
import { defaultAdminProviderDraft } from './adminTypes'

export function AdminView({
  adminMetrics,
  adminMetricsLoading,
  adminRuntime,
  adminSyncStatus,
  adminSyncLoading,
  onTriggerMetricsSync,
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
  adminSyncStatus: BackendAdminSyncStatus | null
  adminSyncLoading: boolean
  onTriggerMetricsSync: () => void | Promise<void>
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
  const { t } = useTranslation()
  return (
    <div className="admin-view two-column-detail">
      <div className="glass-panel">
        <h2 className="section-card__title">{t('admin.overview')}</h2>
        {adminMetricsLoading ? <p className="hint">{t('common.loadingMetrics')}</p> : null}
        {adminMetrics ? (
          <div className="stat-grid">
            <div className="stat-tile">
              <span className="stat-tile__label">{t('admin.users')}</span>
              <span className="stat-tile__value">{adminMetrics.users_count}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">{t('admin.teams')}</span>
              <span className="stat-tile__value">{adminMetrics.teams_count}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">{t('admin.providerInstances')}</span>
              <span className="stat-tile__value">{adminMetrics.provider_instances_count}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">{t('admin.queuedPending')}</span>
              <span className="stat-tile__value">{adminMetrics.posts_pending}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">{t('admin.drafts')}</span>
              <span className="stat-tile__value">{adminMetrics.posts_draft ?? 0}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">{t('admin.publishing')}</span>
              <span className="stat-tile__value">{adminMetrics.posts_processing}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">{t('admin.posted')}</span>
              <span className="stat-tile__value">{adminMetrics.posts_posted}</span>
            </div>
            <div className="stat-tile stat-tile--warn">
              <span className="stat-tile__label">{t('admin.failed')}</span>
              <span className="stat-tile__value">{adminMetrics.posts_failed}</span>
            </div>
            <div className="stat-tile">
              <span className="stat-tile__label">{t('admin.cancelled')}</span>
              <span className="stat-tile__value">{adminMetrics.posts_cancelled}</span>
            </div>
          </div>
        ) : null}
      </div>

      <div className="glass-panel">
        <h2 className="section-card__title">{t('admin.registeredUsers')}</h2>
        <p className="hint">{t('admin.registeredUsersHint')}</p>
        <table className="data-table">
          <thead>
            <tr>
              <th>{t('common.name')}</th>
              <th>{t('common.email')}</th>
              <th>{t('common.access')}</th>
              <th>{t('common.registered')}</th>
            </tr>
          </thead>
          <tbody>
            {[...directoryUsers]
              .sort((left, right) => left.name.localeCompare(right.name))
              .map((user) => (
                <tr key={user.id}>
                  <td>{user.name}</td>
                  <td>{user.email}</td>
                  <td>{user.globalRole === 'admin' ? t('roles.administrator') : t('roles.member')}</td>
                  <td>{user.createdAt && isValid(parseISO(user.createdAt)) ? format(parseISO(user.createdAt), 'PP') : t('common.emDash')}</td>
                </tr>
              ))}
          </tbody>
        </table>
        {directoryUsers.length === 0 ? <p className="hint">{t('admin.noUsers')}</p> : null}
      </div>

      {adminRuntime ? (
        <div className="glass-panel">
          <h2 className="section-card__title">{t('admin.schedulerServer')}</h2>
          <dl className="kv-list">
            <dt>{t('admin.workerProcesses')}</dt>
            <dd>{adminRuntime.scheduler.workers}</dd>
            <dt>{t('admin.pollInterval')}</dt>
            <dd>{adminRuntime.scheduler.pollInterval}</dd>
            <dt>{t('admin.postMetricsSync')}</dt>
            <dd>{adminRuntime.scheduler.metricsSyncInterval ?? t('common.emDash')}</dd>
            <dt>{t('admin.accountHealthCheck')}</dt>
            <dd>{adminRuntime.scheduler.accountHealthInterval ?? t('common.emDash')}</dd>
            <dt>{t('admin.httpListen')}</dt>
            <dd>
              <code className="inline-code">{adminRuntime.general.httpAddr}</code>
            </dd>
            <dt>{t('admin.rateLimitAnonymous')}</dt>
            <dd>{adminRuntime.security.rateLimitPerMinute}</dd>
            <dt>{t('admin.rateLimitBearer')}</dt>
            <dd>{adminRuntime.security.rateLimitAuthenticatedPerMinute}</dd>
          </dl>
        </div>
      ) : null}

      <div className="glass-panel">
        <h2 className="section-card__title">{t('admin.metricsSync')}</h2>
        <p className="hint">{t('admin.metricsSyncHint')}</p>
        {adminSyncLoading ? <p className="hint">{t('common.loadingSync')}</p> : null}
        {adminSyncStatus ? (
          <>
            <dl className="kv-list">
              <dt>{t('admin.postEngagementInterval')}</dt>
              <dd>{adminSyncStatus.post_metrics_sync_interval}</dd>
              <dt>{t('admin.accountFollowersInterval')}</dt>
              <dd>{adminSyncStatus.account_metrics_sync_interval}</dd>
              <dt>{t('admin.targetsWaitingSync')}</dt>
              <dd>{adminSyncStatus.posted_targets_pending_sync}</dd>
              <dt>{t('admin.neverSyncedPosted')}</dt>
              <dd>{adminSyncStatus.posted_targets_never_synced}</dd>
              <dt>{t('admin.targetsWithMetrics')}</dt>
              <dd>{adminSyncStatus.posted_targets_with_metrics}</dd>
              <dt>{t('admin.accountsWithFollowers')}</dt>
              <dd>{adminSyncStatus.accounts_with_follower_metrics}</dd>
            </dl>
            <button
              type="button"
              className="button button--primary mt-1"
              onClick={() => void onTriggerMetricsSync()}
              disabled={syncing}
            >
              {t('admin.syncMetricsNow')}
            </button>
            <p className="hint mt-1">{t('admin.syncMetricsHint')}</p>
          </>
        ) : null}
      </div>

      <div className="glass-panel">
        <h2 className="section-card__title">{t('admin.providerOnboarding')}</h2>
        <p className="hint">{t('admin.providerOnboardingHint')}</p>

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
            <option value="mastodon">{t('accounts.providerMastodon')}</option>
            <option value="friendica">{t('accounts.providerFriendica')}</option>
            <option value="bluesky">{t('accounts.providerBluesky')}</option>
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
              {t('admin.cancelEdit')}
            </button>
          ) : null}
        </div>

        <label className="field">
          <span>{t('admin.displayName')}</span>
          <input value={adminProviderDraft.name} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, name: event.target.value }))} placeholder={t('admin.placeholderDisplayName')} />
        </label>
        <label className="field">
          <span>{t('admin.instanceUrl')}</span>
          <input
            value={adminProviderDraft.instanceUrl}
            onChange={(event) => setAdminProviderDraft((c) => ({ ...c, instanceUrl: event.target.value }))}
            placeholder={t('admin.placeholderMastodon')}
          />
        </label>

        <details className="advanced-config" open={showAdminProviderAdvanced} onToggle={(event) => setShowAdminProviderAdvanced(event.currentTarget.open)}>
          <summary className="advanced-config__summary">{t('admin.advancedConfig')}</summary>
          <p className="hint mt-1">{t('admin.advancedConfigHint')}</p>
          <label className="field">
            <span>{t('admin.clientId')}</span>
            <input
              value={adminProviderDraft.clientId}
              onChange={(event) => setAdminProviderDraft((c) => ({ ...c, clientId: event.target.value }))}
              placeholder={t('admin.placeholderClientId')}
            />
          </label>
          <label className="field">
            <span>{t('admin.clientSecret')}</span>
            <input
              type="password"
              value={adminProviderDraft.clientSecret}
              onChange={(event) => setAdminProviderDraft((c) => ({ ...c, clientSecret: event.target.value }))}
              placeholder={t('admin.placeholderKeepSecret')}
            />
          </label>
          <label className="field">
            <span>{t('admin.scopes')}</span>
            <input value={adminProviderDraft.scopes} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, scopes: event.target.value }))} />
          </label>
          <label className="field">
            <span>{t('admin.authorizationEndpoint')}</span>
            <input
              value={adminProviderDraft.authorizationEndpoint}
              onChange={(event) => setAdminProviderDraft((c) => ({ ...c, authorizationEndpoint: event.target.value }))}
            />
          </label>
          <label className="field">
            <span>{t('admin.tokenEndpoint')}</span>
            <input value={adminProviderDraft.tokenEndpoint} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, tokenEndpoint: event.target.value }))} />
          </label>
        </details>

        <button type="button" className="button button--primary mt-1" onClick={() => void onSaveAdminProvider()} disabled={syncing}>
          {editingProviderId ? t('admin.updateProvider') : t('admin.registerProvider')}
        </button>

        <h3 className="subsection-title mt-2">
          {t('admin.registeredInstances')}
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
                    {onboarded === 1
                      ? t('admin.accountsOnboarded', { count: onboarded })
                      : t('admin.accountsOnboarded_plural', { count: onboarded })}
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
                    {t('common.edit')}
                  </button>
                  <button
                    type="button"
                    className="button button--secondary"
                    onClick={() => {
                      if (window.confirm(t('admin.confirmRemoveInstance', { name: p.name }))) {
                        void onDeleteProviderInstance(p.id)
                      }
                    }}
                    disabled={syncing || onboarded > 0}
                    title={onboarded > 0 ? t('admin.disconnectBeforeRemove') : t('admin.removeProviderInstance')}
                  >
                    <Icon name="trash" className="inline-icon" />
                    <span>{t('common.remove')}</span>
                  </button>
                </div>
              </li>
            )
          })}
        </ul>
        {providerInstances.length === 0 ? <p className="hint">{t('admin.noProviderInstances')}</p> : null}
      </div>
    </div>
  )
}
