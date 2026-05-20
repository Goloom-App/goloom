import { useTranslation } from 'react-i18next'

import { Icon } from '../../icons'
import type { BackendAdminMetrics, BackendAdminSyncStatus } from '../../api'
import type { RuntimeConfigRecord } from '../../types'

export function AdminStatusTab({
  adminMetrics,
  adminMetricsLoading,
  adminRuntime,
  adminSyncStatus,
  adminSyncLoading,
  syncing,
  onTriggerMetricsSync,
}: {
  adminMetrics: BackendAdminMetrics | null
  adminMetricsLoading: boolean
  adminRuntime: RuntimeConfigRecord | null
  adminSyncStatus: BackendAdminSyncStatus | null
  adminSyncLoading: boolean
  syncing: boolean
  onTriggerMetricsSync: () => void | Promise<void>
}) {
  const { t } = useTranslation()

  return (
    <div className="admin-tab-panel stack stack--lg">
      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <div>
            <h2 className="admin-section__title">{t('admin.overview')}</h2>
            <p className="hint admin-section__hint">{t('admin.overviewHint')}</p>
          </div>
          {adminMetricsLoading ? <span className="admin-section__badge hint">{t('common.loadingMetrics')}</span> : null}
        </header>

        {adminMetrics ? (
          <>
            <p className="eyebrow admin-metric-group__label">{t('admin.groupDirectory')}</p>
            <div className="admin-stat-grid">
              <div className="admin-stat-card">
                <span className="admin-stat-card__label">{t('admin.users')}</span>
                <span className="admin-stat-card__value">{adminMetrics.users_count}</span>
              </div>
              <div className="admin-stat-card">
                <span className="admin-stat-card__label">{t('admin.teams')}</span>
                <span className="admin-stat-card__value">{adminMetrics.teams_count}</span>
              </div>
              <div className="admin-stat-card">
                <span className="admin-stat-card__label">{t('admin.providerInstances')}</span>
                <span className="admin-stat-card__value">{adminMetrics.provider_instances_count}</span>
              </div>
            </div>

            <p className="eyebrow admin-metric-group__label">{t('admin.groupPostPipeline')}</p>
            <div className="admin-stat-grid admin-stat-grid--wide">
              <div className="admin-stat-card">
                <span className="admin-stat-card__label">{t('admin.drafts')}</span>
                <span className="admin-stat-card__value">{adminMetrics.posts_draft ?? 0}</span>
              </div>
              <div className="admin-stat-card">
                <span className="admin-stat-card__label">{t('admin.queuedPending')}</span>
                <span className="admin-stat-card__value">{adminMetrics.posts_pending}</span>
              </div>
              <div className="admin-stat-card admin-stat-card--accent">
                <span className="admin-stat-card__label">{t('admin.publishing')}</span>
                <span className="admin-stat-card__value">{adminMetrics.posts_processing}</span>
              </div>
              <div className="admin-stat-card admin-stat-card--success">
                <span className="admin-stat-card__label">{t('admin.posted')}</span>
                <span className="admin-stat-card__value">{adminMetrics.posts_posted}</span>
              </div>
              <div className="admin-stat-card admin-stat-card--warn">
                <span className="admin-stat-card__label">{t('admin.failed')}</span>
                <span className="admin-stat-card__value">{adminMetrics.posts_failed}</span>
              </div>
              <div className="admin-stat-card">
                <span className="admin-stat-card__label">{t('admin.cancelled')}</span>
                <span className="admin-stat-card__value">{adminMetrics.posts_cancelled}</span>
              </div>
            </div>
          </>
        ) : !adminMetricsLoading ? (
          <p className="hint">{t('admin.noMetrics')}</p>
        ) : null}
      </section>

      {adminRuntime ? (
        <section className="admin-section glass-panel">
          <header className="admin-section__head">
            <div>
              <h2 className="admin-section__title">{t('admin.schedulerServer')}</h2>
              <p className="hint admin-section__hint">{t('admin.schedulerHint')}</p>
            </div>
            <span className="admin-pill admin-pill--ok">
              <span className="status-dot status-dot--active" aria-hidden />
              {t('admin.schedulerRunning')}
            </span>
          </header>
          <div className="admin-kv-grid">
            <div className="admin-kv-card">
              <span className="admin-kv-card__label">{t('admin.workerProcesses')}</span>
              <span className="admin-kv-card__value">{adminRuntime.scheduler.workers}</span>
            </div>
            <div className="admin-kv-card">
              <span className="admin-kv-card__label">{t('admin.pollInterval')}</span>
              <span className="admin-kv-card__value">{adminRuntime.scheduler.pollInterval}</span>
            </div>
            <div className="admin-kv-card">
              <span className="admin-kv-card__label">{t('admin.postMetricsSync')}</span>
              <span className="admin-kv-card__value">{adminRuntime.scheduler.metricsSyncInterval ?? t('common.emDash')}</span>
            </div>
            <div className="admin-kv-card">
              <span className="admin-kv-card__label">{t('admin.accountHealthCheck')}</span>
              <span className="admin-kv-card__value">{adminRuntime.scheduler.accountHealthInterval ?? t('common.emDash')}</span>
            </div>
            <div className="admin-kv-card admin-kv-card--wide">
              <span className="admin-kv-card__label">{t('admin.httpListen')}</span>
              <code className="inline-code admin-kv-card__code">{adminRuntime.general.httpAddr}</code>
            </div>
          </div>
        </section>
      ) : null}

      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <div>
            <h2 className="admin-section__title">{t('admin.metricsSync')}</h2>
            <p className="hint admin-section__hint">{t('admin.metricsSyncHint')}</p>
          </div>
          {adminSyncLoading ? <span className="admin-section__badge hint">{t('common.loadingSync')}</span> : null}
        </header>

        {adminSyncStatus ? (
          <>
            <div className="admin-kv-grid">
              <div className="admin-kv-card">
                <span className="admin-kv-card__label">{t('admin.postEngagementInterval')}</span>
                <span className="admin-kv-card__value">{adminSyncStatus.post_metrics_sync_interval}</span>
              </div>
              <div className="admin-kv-card">
                <span className="admin-kv-card__label">{t('admin.accountFollowersInterval')}</span>
                <span className="admin-kv-card__value">{adminSyncStatus.account_metrics_sync_interval}</span>
              </div>
              <div className="admin-kv-card">
                <span className="admin-kv-card__label">{t('admin.targetsWaitingSync')}</span>
                <span className="admin-kv-card__value">{adminSyncStatus.posted_targets_pending_sync}</span>
              </div>
              <div className="admin-kv-card">
                <span className="admin-kv-card__label">{t('admin.neverSyncedPosted')}</span>
                <span className="admin-kv-card__value">{adminSyncStatus.posted_targets_never_synced}</span>
              </div>
              <div className="admin-kv-card">
                <span className="admin-kv-card__label">{t('admin.targetsWithMetrics')}</span>
                <span className="admin-kv-card__value">{adminSyncStatus.posted_targets_with_metrics}</span>
              </div>
              <div className="admin-kv-card">
                <span className="admin-kv-card__label">{t('admin.accountsWithFollowers')}</span>
                <span className="admin-kv-card__value">{adminSyncStatus.accounts_with_follower_metrics}</span>
              </div>
            </div>
            <div className="admin-action-bar">
              <button type="button" className="button button--primary" onClick={() => void onTriggerMetricsSync()} disabled={syncing}>
                <Icon name="refresh" className="inline-icon" />
                {t('admin.syncMetricsNow')}
              </button>
              <p className="hint m-0">{t('admin.syncMetricsActionHint')}</p>
            </div>
          </>
        ) : !adminSyncLoading ? (
          <p className="hint">{t('admin.noSyncStatus')}</p>
        ) : null}
      </section>
    </div>
  )
}
