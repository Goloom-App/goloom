import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Icon } from '../../icons'
import type { BackendAdminMetrics, BackendAdminSyncStatus, BackendPublishFailure } from '../../api'
import type { RuntimeConfigRecord } from '../../types'

export function AdminStatusTab({
  adminMetrics,
  adminMetricsLoading,
  adminRuntime,
  adminSyncStatus,
  adminSyncLoading,
  syncing,
  onTriggerMetricsSync,
  publishFailures,
  onAcknowledgePublishFailure,
  onRetryPublishFailure,
}: {
  adminMetrics: BackendAdminMetrics | null
  adminMetricsLoading: boolean
  adminRuntime: RuntimeConfigRecord | null
  adminSyncStatus: BackendAdminSyncStatus | null
  adminSyncLoading: boolean
  syncing: boolean
  onTriggerMetricsSync: () => void | Promise<void>
  publishFailures: BackendPublishFailure[]
  onAcknowledgePublishFailure: (postID: string) => void | Promise<void>
  onRetryPublishFailure: (postID: string) => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [showFailures, setShowFailures] = useState(false)
  const [busyFailureId, setBusyFailureId] = useState<string | null>(null)

  const runFailureAction = async (postID: string, action: (id: string) => void | Promise<void>) => {
    setBusyFailureId(postID)
    try {
      await action(postID)
    } finally {
      setBusyFailureId(null)
    }
  }

  // Health signals drive the top-of-page banner so operators see problems at a
  // glance instead of scanning every metric.
  const schedulerOk = adminRuntime != null
  const failedPosts = adminMetrics?.posts_failed ?? 0
  const pendingSync = adminSyncStatus?.posted_targets_pending_sync ?? 0
  const overallOk = schedulerOk && failedPosts === 0

  return (
    <div className="admin-tab-panel stack stack--lg">
      <section className={`admin-health glass-panel ${overallOk ? 'admin-health--ok' : 'admin-health--attention'}`}>
        <div className="admin-health__primary">
          <span className={`admin-health__pulse status-dot ${overallOk ? 'status-dot--active' : 'status-dot--warning'}`} aria-hidden />
          <div>
            <strong className="admin-health__headline">
              {overallOk ? t('admin.systemHealthy') : t('admin.systemAttention')}
            </strong>
            <p className="hint m-0">{t('admin.healthSubtitle')}</p>
          </div>
        </div>

        <div className="admin-health__signals">
          <span className="admin-health__signal">
            <span className={`status-dot ${schedulerOk ? 'status-dot--active' : 'status-dot--neutral'}`} aria-hidden />
            {schedulerOk ? t('admin.schedulerRunning') : t('admin.schedulerUnknown')}
          </span>
          {failedPosts > 0 ? (
            <button
              type="button"
              className="admin-health__signal"
              data-testid="admin-failed-toggle"
              onClick={() => setShowFailures((v) => !v)}
              aria-expanded={showFailures}
              style={{ background: 'none', border: 'none', cursor: 'pointer', textDecoration: 'underline' }}
            >
              <span className="status-dot status-dot--error" aria-hidden />
              {t('admin.failedPostsAlert', { count: failedPosts })}
              <span aria-hidden>{showFailures ? ' ▴' : ' ▾'}</span>
            </button>
          ) : (
            <span className="admin-health__signal">
              <span className="status-dot status-dot--active" aria-hidden />
              {t('admin.noFailedPosts')}
            </span>
          )}
          <span className="admin-health__signal">
            <span className={`status-dot ${pendingSync > 0 ? 'status-dot--warning' : 'status-dot--active'}`} aria-hidden />
            {pendingSync > 0 ? t('admin.syncBacklog', { count: pendingSync }) : t('admin.syncUpToDate')}
          </span>
        </div>
      </section>

      {showFailures && failedPosts > 0 ? (
        <section className="admin-section glass-panel" data-testid="admin-failures-panel">
          <header className="admin-section__head">
            <div className="admin-section__heading">
              <span className="admin-section__icon" aria-hidden><Icon name="refresh" /></span>
              <div>
                <h2 className="admin-section__title">{t('admin.failuresTitle')}</h2>
                <p className="hint admin-section__hint">{t('admin.failuresHint')}</p>
              </div>
            </div>
          </header>
          {publishFailures.length === 0 ? (
            <p className="hint">{t('common.loadingMetrics')}</p>
          ) : (
            <ul className="stack stack--sm" style={{ listStyle: 'none', margin: 0, padding: 0 }}>
              {publishFailures.map((f) => (
                <li
                  key={f.post_id}
                  className="admin-kv-card"
                  data-testid="admin-failure-item"
                  style={{ display: 'block' }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, flexWrap: 'wrap' }}>
                    <strong>{f.title?.trim() || t('admin.failureUntitled')}</strong>
                    <span className="hint">
                      {f.team_name} · {new Date(f.scheduled_at).toLocaleString()} · {t('admin.failureAttempts', { count: f.attempt_count })}
                    </span>
                  </div>
                  {f.last_error ? (
                    <p className="hint" style={{ whiteSpace: 'pre-wrap', marginTop: 6 }}>{f.last_error}</p>
                  ) : null}
                  {f.targets?.some((tg) => tg.status === 'failed' || tg.last_error) ? (
                    <ul style={{ listStyle: 'none', margin: '8px 0 0', padding: 0, display: 'grid', gap: 4 }}>
                      {f.targets
                        .filter((tg) => tg.status === 'failed' || tg.last_error)
                        .map((tg) => (
                          <li key={tg.account_id} style={{ display: 'flex', gap: 6, alignItems: 'baseline', flexWrap: 'wrap' }}>
                            <span className="status-dot status-dot--error" aria-hidden />
                            <span>{tg.account_name || tg.account_id} <span className="hint">({tg.provider})</span></span>
                            {tg.last_error ? <span className="hint" style={{ whiteSpace: 'pre-wrap' }}>— {tg.last_error}</span> : null}
                          </li>
                        ))}
                    </ul>
                  ) : null}
                  <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
                    <button
                      type="button"
                      className="button button--primary"
                      disabled={busyFailureId === f.post_id}
                      onClick={() => void runFailureAction(f.post_id, onRetryPublishFailure)}
                    >
                      {t('admin.failureRetry')}
                    </button>
                    <button
                      type="button"
                      className="button"
                      disabled={busyFailureId === f.post_id}
                      onClick={() => void runFailureAction(f.post_id, onAcknowledgePublishFailure)}
                    >
                      {t('admin.failureAcknowledge')}
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>
      ) : null}

      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <div className="admin-section__heading">
            <span className="admin-section__icon" aria-hidden><Icon name="chart" /></span>
            <div>
              <h2 className="admin-section__title">{t('admin.overview')}</h2>
              <p className="hint admin-section__hint">{t('admin.overviewHint')}</p>
            </div>
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
            <PostPipelineBar metrics={adminMetrics} t={t} />
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
            <div className="admin-section__heading">
              <span className="admin-section__icon" aria-hidden><Icon name="settings" /></span>
              <div>
                <h2 className="admin-section__title">{t('admin.schedulerServer')}</h2>
                <p className="hint admin-section__hint">{t('admin.schedulerHint')}</p>
              </div>
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
          <div className="admin-section__heading">
            <span className="admin-section__icon" aria-hidden><Icon name="refresh" /></span>
            <div>
              <h2 className="admin-section__title">{t('admin.metricsSync')}</h2>
              <p className="hint admin-section__hint">{t('admin.metricsSyncHint')}</p>
            </div>
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

const PIPELINE_SEGMENTS: { key: keyof BackendAdminMetrics; cls: string; label: string }[] = [
  { key: 'posts_draft', cls: 'admin-pipeline__seg--draft', label: 'admin.drafts' },
  { key: 'posts_pending', cls: 'admin-pipeline__seg--pending', label: 'admin.queuedPending' },
  { key: 'posts_processing', cls: 'admin-pipeline__seg--processing', label: 'admin.publishing' },
  { key: 'posts_posted', cls: 'admin-pipeline__seg--posted', label: 'admin.posted' },
  { key: 'posts_failed', cls: 'admin-pipeline__seg--failed', label: 'admin.failed' },
  { key: 'posts_cancelled', cls: 'admin-pipeline__seg--cancelled', label: 'admin.cancelled' },
]

function PostPipelineBar({ metrics, t }: { metrics: BackendAdminMetrics; t: (key: string, opts?: Record<string, unknown>) => string }) {
  const segments = PIPELINE_SEGMENTS.map((seg) => ({ ...seg, value: Number(metrics[seg.key] ?? 0) }))
  const total = segments.reduce((sum, seg) => sum + seg.value, 0)
  if (total === 0) {
    return null
  }
  return (
    <div className="admin-pipeline" role="img" aria-label={t('admin.groupPostPipeline')}>
      <div className="admin-pipeline__bar">
        {segments
          .filter((seg) => seg.value > 0)
          .map((seg) => (
            <span
              key={seg.key}
              className={`admin-pipeline__seg ${seg.cls}`}
              style={{ width: `${(seg.value / total) * 100}%` }}
              title={`${t(seg.label)}: ${seg.value}`}
            />
          ))}
      </div>
    </div>
  )
}
