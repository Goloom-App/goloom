import { useCallback, useEffect, useMemo, useState } from 'react'
import { format, formatDistanceToNow, isToday, isTomorrow, parseISO } from 'date-fns'
import { de } from 'date-fns/locale/de'
import { enUS } from 'date-fns/locale/en-US'
import { useTranslation } from 'react-i18next'
import type { BackendMetricHistoryPoint, BackendAccountGrowthPoint } from '../../api'
import { DestinationAvatar } from '../../components/post/DestinationAvatar'
import { Icon } from '../../icons'
import { accountConnectionStatus } from '../../mappers'
import type { AccountRecord, PostRecord } from '../../types'
import { sharedAccountLabels } from '../../schedule'
import { Area, AreaChart, ResponsiveContainer, Tooltip } from 'recharts'

const ENGAGEMENT_METRICS = ['likes', 'reposts', 'replies'] as const

type SparkPoint = { date: string; value: number }

function toDailyDeltas(series: BackendMetricHistoryPoint[], clampAtZero = false): SparkPoint[] {
  return series.map((p, i, arr) => {
    const prev = arr[i - 1]
    const delta = prev ? p.value - prev.value : 0
    return {
      date: p.date,
      value: clampAtZero ? Math.max(0, delta) : delta,
    }
  })
}

function formatSparkDate(date: string): string {
  try {
    return format(parseISO(date), 'MMM d, yyyy')
  } catch {
    return date
  }
}

type SparkValueMode = 'total' | 'delta' | 'balance'

function formatSparkValue(value: number, mode: SparkValueMode): string {
  if ((mode === 'delta' || mode === 'balance') && value > 0) {
    return `+${value.toLocaleString()}`
  }
  return value.toLocaleString()
}

function MetricSparkline({
  title,
  subtitle,
  color,
  points,
  loading,
  loadingLabel,
  emptyLabel,
  valueMode = 'delta',
}: {
  title: string
  subtitle: string
  color: string
  points: SparkPoint[]
  loading: boolean
  loadingLabel: string
  emptyLabel: string
  valueMode?: SparkValueMode
}) {
  const data = useMemo(() => points, [points])
  // 'balance' shows the net change across the whole window (last − first), so a
  // rising follower count reads as growth even when today's delta happens to be 0.
  const headlineValue = useMemo(() => {
    if (data.length === 0) return null
    if (valueMode === 'balance') {
      return data[data.length - 1].value - data[0].value
    }
    return data[data.length - 1].value
  }, [data, valueMode])

  return (
    <div className="dashboard-spark">
      <div className="dashboard-spark__head">
        <div className="dashboard-spark__info">
          <span className="dashboard-spark__title">{title}</span>
          <span className="dashboard-spark__subtitle">{subtitle}</span>
        </div>
        {headlineValue !== null && (
          <div
            className="dashboard-spark__value"
            style={
              valueMode === 'delta' || valueMode === 'balance'
                ? { color: headlineValue < 0 ? '#ef4444' : headlineValue > 0 ? '#22c55e' : 'inherit' }
                : undefined
            }
          >
            {formatSparkValue(headlineValue, valueMode)}
          </div>
        )}
      </div>
      <div className="dashboard-spark__chart">
        {loading && data.length === 0 ? (
          <p className="hint dashboard-spark__placeholder">{loadingLabel}</p>
        ) : data.length === 0 ? (
          <p className="hint dashboard-spark__placeholder">{emptyLabel}</p>
        ) : (
          <ResponsiveContainer width="100%" height={88}>
            <AreaChart data={data} margin={{ top: 4, right: 4, left: 0, bottom: 0 }}>
              <Tooltip
                contentStyle={{
                  fontSize: '12px',
                  borderRadius: '8px',
                  background: 'var(--surface-raised)',
                  border: '1px solid var(--border)',
                }}
                labelFormatter={(_, items) => {
                  const point = items?.[0]?.payload as SparkPoint | undefined
                  return point ? formatSparkDate(point.date) : ''
                }}
                formatter={(value) => {
                  const n = typeof value === 'number' ? value : Number(value)
                  return Number.isFinite(n) ? formatSparkValue(n, valueMode) : String(value ?? '')
                }}
              />
              <Area type="monotone" dataKey="value" stroke={color} fill={color} fillOpacity={0.12} strokeWidth={1.75} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  )
}

function formatRelativeTime(iso: string, locale: 'de' | 'en'): string {
  const date = parseISO(iso)
  const localeObj = locale === 'de' ? de : enUS
  const now = Date.now()
  const diffMs = date.getTime() - now
  const diffHours = diffMs / (1000 * 60 * 60)

  if (diffHours > 0 && diffHours <= 2) {
    return formatDistanceToNow(date, { addSuffix: true, locale: localeObj })
  }
  if (isToday(date)) {
    return `${locale === 'de' ? 'heute' : 'today'} ${format(date, 'HH:mm')}`
  }
  if (isTomorrow(date)) {
    return `${locale === 'de' ? 'morgen' : 'tomorrow'} ${format(date, 'HH:mm')}`
  }

  const diffDays = Math.round(diffMs / (1000 * 60 * 60 * 24))
  if (diffDays >= 1 && diffDays <= 6) {
    return format(date, 'EEE HH:mm', { locale: localeObj })
  }

  return format(date, 'MMM d, HH:mm', { locale: localeObj })
}

export function DashboardView({
  teamName,
  upcomingPosts,
  accounts,
  fetchSeries,
  fetchGrowth,
  onOpenPreview,
  onEditPost,
  onOpenSchedule,
  onOpenAccounts,
}: {
  teamName: string
  upcomingPosts: PostRecord[]
  accounts: AccountRecord[]
  fetchSeries: (metric: string) => Promise<{ series: BackendMetricHistoryPoint[] }>
  fetchGrowth: (accountId: string, opts?: { days?: number }) => Promise<{ days: number; account: string; series: import('../../api').BackendAccountGrowthPoint[] }>
  onOpenPreview: (postId: string) => void
  onEditPost?: (postId: string) => void
  onOpenSchedule: () => void
  onOpenAccounts: () => void
}) {
  const { t, i18n } = useTranslation()
  const locale = i18n.language === 'de' ? 'de' : 'en'
  const [seriesByMetric, setSeriesByMetric] = useState<Record<string, SparkPoint[]>>({})
  const [networkSeries, setNetworkSeries] = useState<SparkPoint[]>([])
  const [loadingCharts, setLoadingCharts] = useState(false)

  const loadCharts = useCallback(async () => {
    setLoadingCharts(true)
    try {
      const [engagementResults, growthResult] = await Promise.all([
        Promise.all(
          ENGAGEMENT_METRICS.map(async (m) => {
            const res = await fetchSeries(m)
            return [m, res.series ?? []] as const
          }),
        ),
        fetchGrowth('all', { days: 7 }),
      ])

      // Plot the cumulative network size so the trend line rises with new
      // followers; the headline ('balance' mode) shows the net change over the window.
      const network = (growthResult.series ?? []).map((p: BackendAccountGrowthPoint) => ({
        date: p.date,
        value: p.followers + p.following,
      }))
      setNetworkSeries(network)

      const next: Record<string, SparkPoint[]> = {}
      for (const [m, series] of engagementResults) {
        next[m] = toDailyDeltas(series as BackendMetricHistoryPoint[], true)
      }
      setSeriesByMetric(next)
    } catch {
      setSeriesByMetric({})
      setNetworkSeries([])
    } finally {
      setLoadingCharts(false)
    }
  }, [fetchSeries, fetchGrowth])

  useEffect(() => {
    void loadCharts()
  }, [loadCharts])

  const chartColors: Record<string, string> = {
    likes: 'var(--accent)',
    reposts: '#22c55e',
    replies: '#38bdf8',
  }

  const chartTitles: Record<string, string> = {
    likes: t('dashboard.metricLikes'),
    reposts: t('dashboard.metricShares'),
    replies: t('dashboard.metricReplies'),
  }

  const sparkProps = {
    subtitle: t('dashboard.last7days'),
    loading: loadingCharts,
    loadingLabel: t('common.loading'),
    emptyLabel: t('dashboard.noDataYet'),
  }

  const performancePanel = (
    <section className="glass-panel dashboard-panel">
      <div className="dashboard-panel__header">
        <div>
          <p className="eyebrow">{t('eyebrow.performance')}</p>
          <h2 className="dashboard-panel__title">{t('dashboard.recentEngagement')}</h2>
          <p className="hint">{t('dashboard.dailyChangeHint')}</p>
        </div>
        <button type="button" className="button button--secondary" onClick={() => void loadCharts()} disabled={loadingCharts}>
          {loadingCharts ? <Icon name="loader" className="reload-spin" /> : <Icon name="refresh" />}
          <span>{t('common.refresh')}</span>
        </button>
      </div>
      <div className="dashboard-spark-grid">
        <MetricSparkline
          title={t('dashboard.networkTrend')}
          color="#38bdf8"
          points={networkSeries}
          valueMode="balance"
          {...sparkProps}
        />
        {ENGAGEMENT_METRICS.map((m) => (
          <MetricSparkline
            key={m}
            title={chartTitles[m]}
            color={chartColors[m] ?? 'var(--accent)'}
            points={seriesByMetric[m] ?? []}
            {...sparkProps}
          />
        ))}
      </div>
    </section>
  )

  const accountPanel = (
    <section className="glass-panel dashboard-panel">
      <div className="dashboard-panel__header">
        <div>
          <p className="eyebrow">{t('eyebrow.accountHealth')}</p>
          <h2 className="dashboard-panel__title">{t('dashboard.connectionStatus')}</h2>
          <p className="hint">{t('dashboard.oauthHint')}</p>
        </div>
        <button type="button" className="button button--secondary" onClick={onOpenAccounts}>
          <Icon name="share" className="dashboard-manage-accounts__icon" />
          <span>{t('dashboard.manageAccounts')}</span>
        </button>
      </div>
      {accounts.length === 0 ? (
        <p className="hint dashboard-panel__empty">{t('dashboard.noAccountsLinked')}</p>
      ) : (
        <div className="dashboard-accounts">
          {accounts.map((account) => {
            const status = accountConnectionStatus(account)
            return (
              <div key={account.id} className="dashboard-account-card">
                <DestinationAvatar account={account} />
                <div className="dashboard-account-card__meta">
                  <span className="dashboard-account-card__name">{account.name}</span>
                  <span className="dashboard-account-card__handle">{account.username}</span>
                </div>
                <span className={`dashboard-account-card__pill dashboard-account-card__pill--${status}`}>
                  {status === 'active' ? t('dashboard.active') : t('dashboard.needsReauth')}
                </span>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )

  const scheduledPanel = (
    <section className="glass-panel dashboard-panel">
      <div className="dashboard-panel__header">
        <div>
          <p className="eyebrow">{t('eyebrow.schedule')}</p>
          <h2 className="dashboard-panel__title">{t('dashboard.upcomingPosts')}</h2>
          <p className="hint">{teamName}</p>
        </div>
        <button type="button" className="button button--secondary" onClick={onOpenSchedule}>
          <Icon name="calendar" className="inline-icon" />
          <span>{t('dashboard.openSchedule')}</span>
        </button>
      </div>
      {upcomingPosts.length === 0 ? (
        <p className="hint dashboard-panel__empty">{t('dashboard.noUpcoming')}</p>
      ) : (
        <div className="dashboard-scheduled-horizontal" role="list" aria-label={t('dashboard.upcomingPostsAria')}>
          {upcomingPosts.slice(0, 8).map((post) => {
            const snippet = post.title?.trim() || post.content.slice(0, 80) || t('common.untitled')
            return (
              <div key={post.id} className="dashboard-scheduled-card" role="listitem">
                <button type="button" className="dashboard-scheduled-card__body" onClick={() => onOpenPreview(post.id)}>
                  <time className="dashboard-scheduled-card__time" dateTime={post.scheduledAt}>
                    {formatRelativeTime(post.scheduledAt, locale)}
                  </time>
                  <p className="dashboard-scheduled-card__title">{snippet}</p>
                  <div className="dashboard-scheduled-card__accounts" aria-hidden="true">
                    {sharedAccountLabels(post, accounts).map((a) => (
                      <DestinationAvatar key={a.id} account={a} compact />
                    ))}
                  </div>
                </button>
                {onEditPost ? (
                  <button
                    type="button"
                    className="dashboard-scheduled-card__edit"
                    aria-label={t('common.edit')}
                    onClick={() => onEditPost(post.id)}
                  >
                    <Icon name="edit" />
                  </button>
                ) : null}
              </div>
            )
          })}
        </div>
      )}
    </section>
  )

  return (
    <div className="dashboard-view">
      {performancePanel}
      {accountPanel}
      {scheduledPanel}
    </div>
  )
}
