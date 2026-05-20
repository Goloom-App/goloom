import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { format, parseISO } from 'date-fns'
import { translateApiError } from '../../i18n/translateApiError'
import { Bar, BarChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { DestinationAvatar } from '../../components/post/DestinationAvatar'
import { Icon } from '../../icons'
import type {
  BackendAccountGrowthPoint,
  BackendMetricHistoryPoint,
  BackendPostAnalyticsListRow,
  BackendPostMetric,
  BackendTeamAnalyticsReport,
} from '../../api'
import type { AccountRecord } from '../../types'
import { accountConnectionStatus } from '../../mappers'
import { aggregatePostMetrics, engagementForAccount } from '../../postMetrics'

function metricLabel(metric: string, t: (key: string) => string): string {
  if (metric === 'likes') return t('analytics.metricLikes')
  if (metric === 'reposts') return t('analytics.metricShares')
  if (metric === 'replies') return t('analytics.metricReplies')
  return metric
}

function PostMetricsDetailPanel({
  loading,
  metrics,
  totals,
  accounts,
}: {
  loading: boolean
  metrics: BackendPostMetric[]
  totals: Record<string, number>
  accounts: AccountRecord[]
}) {
  const { t } = useTranslation()
  return (
    <section className="glass-panel glass-panel--compact analytics-post-detail">
      <h4 className="subsection-title">{t('analytics.postMetrics')}</h4>
      {loading ? (
        <p className="hint">{t('common.loadingPostMetrics')}</p>
      ) : metrics.length === 0 ? (
        <p className="hint">{t('analytics.noPostMetrics')}</p>
      ) : (
        <>
          <ul className="analytics-delta-grid">
            {Object.entries(totals).map(([metric, total]) => (
              <li key={metric} className="analytics-delta-card">
                <span className="analytics-delta-card__metric">{metricLabel(metric, t)}</span>
                <span className="analytics-delta-card__total">{total.toLocaleString()}</span>
              </li>
            ))}
          </ul>
          <div className="analytics-post-detail__by-account">
            {accounts.map((acc) => {
              const engagement = engagementForAccount(metrics, acc.id)
              if (engagement.likes === 0 && engagement.reposts === 0 && engagement.replies === 0) {
                return null
              }
              return (
                <div key={acc.id} className="analytics-post-detail__account">
                  <DestinationAvatar account={acc} />
                  <span className="hint">
                    {t('analytics.engagementBreakdown', {
                      likes: engagement.likes,
                      reposts: engagement.reposts,
                      replies: engagement.replies,
                    })}
                  </span>
                </div>
              )
            })}
          </div>
        </>
      )}
    </section>
  )
}

function formatDeltaPct(n: number | undefined): string {
  if (n == null || Number.isNaN(n)) {
    return ''
  }
  const rounded = Math.round(n * 10) / 10
  const sign = rounded > 0 ? '+' : ''
  return `${sign}${rounded}%`
}

export function AnalyticsView({
  teamId,
  accounts,
  fetchSummary,
  fetchPosts,
  fetchChart,
  fetchAccountGrowth,
  fetchPostMetrics,
}: {
  teamId: string
  accounts: AccountRecord[]
  fetchSummary: (opts?: { top_posts?: number }) => Promise<BackendTeamAnalyticsReport>
  fetchPosts: (opts?: { sort?: string; limit?: number; offset?: number }) => Promise<{ items: BackendPostAnalyticsListRow[] }>
  fetchChart: (opts: { metric: string; days?: number }) => Promise<{ metric: string; days: number; series: BackendMetricHistoryPoint[] }>
  fetchAccountGrowth: (accountId: string, opts?: { days?: number }) => Promise<{ days: number; account: string; series: BackendAccountGrowthPoint[] }>
  fetchPostMetrics?: (postId: string) => Promise<{ items: BackendPostMetric[] }>
}) {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<'overview' | 'accounts' | 'posts'>('overview')
  const [summary, setSummary] = useState<BackendTeamAnalyticsReport | null>(null)
  const [posts, setPosts] = useState<BackendPostAnalyticsListRow[]>([])
  const [series, setSeries] = useState<BackendMetricHistoryPoint[]>([])
  const [chartMetric, setChartMetric] = useState<string>('')
  const [growthAccountId, setGrowthAccountId] = useState<string>('all')
  const [accountGrowthSeries, setAccountGrowthSeries] = useState<BackendAccountGrowthPoint[]>([])
  const [accountLatestGrowth, setAccountLatestGrowth] = useState<Record<string, BackendAccountGrowthPoint>>({})
  const [selectedPostId, setSelectedPostId] = useState<string | null>(null)
  const [selectedPostMetrics, setSelectedPostMetrics] = useState<BackendPostMetric[]>([])
  const [postMetricsLoading, setPostMetricsLoading] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const viewGapStyle = { display: 'flex', flexDirection: 'column' as const, gap: 'var(--space-8)' }
  const metrics = summary?.metrics ?? []

  const load = useCallback(async () => {
    if (!teamId) {
      return
    }
    setLoading(true)
    setError(null)
    try {
      const [sum, postRows] = await Promise.all([
        fetchSummary({ top_posts: 12 }),
        fetchPosts({ sort: 'score', limit: 50, offset: 0 }),
      ])
      setSummary(sum)
      setPosts(postRows.items ?? [])
      const firstMetric = sum.metrics?.[0]?.metric ?? ''
      setChartMetric((prev) => {
        if (prev && sum.metrics?.some((m) => m.metric === prev)) {
          return prev
        }
        return firstMetric
      })
    } catch (e) {
      setSummary(null)
      setPosts([])
      setSeries([])
      const raw = e instanceof Error ? e.message : t('common.failedLoadAnalytics')
      setError(translateApiError(raw, t))
    } finally {
      setLoading(false)
    }
  }, [fetchPosts, fetchSummary, teamId, t])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (!teamId || !chartMetric.trim()) {
      setSeries([])
      return
    }
    let cancelled = false
    void fetchChart({ metric: chartMetric, days: 30 })
      .then((res) => {
        if (!cancelled) {
          setSeries(res.series ?? [])
        }
      })
      .catch(() => {
        if (!cancelled) {
          setSeries([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [chartMetric, fetchChart, teamId])

  useEffect(() => {
    if (!teamId) {
      setAccountGrowthSeries([])
      return
    }
    let cancelled = false
    void fetchAccountGrowth(growthAccountId || 'all', { days: 30 })
      .then((res) => {
        if (!cancelled) {
          setAccountGrowthSeries(res.series ?? [])
        }
      })
      .catch(() => {
        if (!cancelled) {
          setAccountGrowthSeries([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [fetchAccountGrowth, growthAccountId, teamId])

  useEffect(() => {
    if (!teamId || activeTab !== 'accounts' || accounts.length === 0) {
      setAccountLatestGrowth({})
      return
    }
    let cancelled = false
    void Promise.all(
      accounts.map(async (acc) => {
        try {
          const res = await fetchAccountGrowth(acc.id, { days: 7 })
          const last = res.series?.[res.series.length - 1]
          return { id: acc.id, last }
        } catch {
          return { id: acc.id, last: undefined }
        }
      }),
    ).then((rows) => {
      if (cancelled) {
        return
      }
      const next: Record<string, BackendAccountGrowthPoint> = {}
      for (const row of rows) {
        if (row.last) {
          next[row.id] = row.last
        }
      }
      setAccountLatestGrowth(next)
    })
    return () => {
      cancelled = true
    }
  }, [accounts, activeTab, fetchAccountGrowth, teamId])

  useEffect(() => {
    if (!fetchPostMetrics || !selectedPostId) {
      setSelectedPostMetrics([])
      return
    }
    let cancelled = false
    setPostMetricsLoading(true)
    void fetchPostMetrics(selectedPostId)
      .then((res) => {
        if (!cancelled) {
          setSelectedPostMetrics(res.items ?? [])
        }
      })
      .catch(() => {
        if (!cancelled) {
          setSelectedPostMetrics([])
        }
      })
      .finally(() => {
        if (!cancelled) {
          setPostMetricsLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [fetchPostMetrics, selectedPostId])

  const barData = useMemo(() => {
    if (!metrics.length) {
      return []
    }
    return [...metrics]
      .map((m) => ({ name: m.metric, value: m.total }))
      .sort((a, b) => b.value - a.value)
  }, [metrics])

  const totalEngagement = useMemo(() => {
    if (!metrics.length) {
      return 0
    }
    return metrics.reduce((a, m) => a + m.total, 0)
  }, [metrics])

  const newFollowers7d = useMemo(() => {
    if (accountGrowthSeries.length < 2) {
      return 0
    }
    const latest = accountGrowthSeries[accountGrowthSeries.length - 1].followers
    // Find point roughly 7 days ago or the oldest available if less than 7
    const index = Math.max(0, accountGrowthSeries.length - 8)
    const sevenDaysAgo = accountGrowthSeries[index].followers
    return latest - sevenDaysAgo
  }, [accountGrowthSeries])

  const lineData = useMemo(() => series.map((p) => ({ date: p.date, value: p.value })), [series])
  const growthData = useMemo(
    () =>
      accountGrowthSeries.map((point) => ({
        date: point.date,
        followers: point.followers,
        following: point.following,
        posts: point.posts,
        networkSize: point.followers + point.following,
      })),
    [accountGrowthSeries],
  )

  const selectedPostTotals = useMemo(() => aggregatePostMetrics(selectedPostMetrics), [selectedPostMetrics])

  if (!teamId) {
    return <p className="hint">{t('common.selectTeamAnalytics')}</p>
  }

  return (
    <div className="analytics-view" style={viewGapStyle}>
      <div className="page-header" style={{ marginBottom: 0 }}>
        <div>
          <p className="eyebrow">{t('eyebrow.analytics')}</p>
          <h1>{t('analytics.teamPerformance')}</h1>
        </div>
        <div className="page-header__actions">
          <div className="view-toggle view-toggle--scrollable">
            <button
              type="button"
              className={`view-toggle__btn ${activeTab === 'overview' ? 'view-toggle__btn--active' : ''}`}
              onClick={() => setActiveTab('overview')}
            >
              {t('analytics.tabOverview')}
            </button>
            <button
              type="button"
              className={`view-toggle__btn ${activeTab === 'accounts' ? 'view-toggle__btn--active' : ''}`}
              onClick={() => setActiveTab('accounts')}
            >
              {t('analytics.tabAccounts')}
            </button>
            <button
              type="button"
              className={`view-toggle__btn ${activeTab === 'posts' ? 'view-toggle__btn--active' : ''}`}
              onClick={() => setActiveTab('posts')}
            >
              {t('analytics.tabPosts')}
            </button>
          </div>
          <button type="button" className="button button--secondary" onClick={() => void load()} disabled={loading}>
            <Icon name="refresh" className="inline-icon" />
            <span className="hidden-mobile">{t('common.refresh')}</span>
          </button>
        </div>
      </div>

      {error ? <p className="status-banner__error">{error}</p> : null}
      {loading && !summary ? <p className="hint">{t('common.loadingAnalytics')}</p> : null}

      {summary && activeTab === 'overview' && (
        <div style={viewGapStyle}>
          <div className="analytics-cards">
            <section className="glass-panel analytics-card">
              <p className="eyebrow">{t('analytics.totalEngagement')}</p>
              <p className="analytics-card__value">{totalEngagement.toLocaleString()}</p>
              <p className="hint">{t('analytics.totalEngagementHint')}</p>
            </section>
            <section className="glass-panel analytics-card">
              <p className="eyebrow">{t('analytics.followerTrend7d')}</p>
              <p className="analytics-card__value" style={{ color: newFollowers7d < 0 ? '#ef4444' : (newFollowers7d > 0 ? '#22c55e' : 'inherit') }}>
                {(newFollowers7d >= 0 ? '+' : '') + newFollowers7d.toLocaleString()}
              </p>
              <p className="hint">{t('analytics.followerTrend7dHint')}</p>
            </section>
          </div>

          <div className="analytics-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(400px, 1fr))', gap: 'var(--space-8)' }}>
            {metrics.length > 0 && (
              <section className="glass-panel">
                <h3 className="subsection-title">{t('analytics.platformEngagement')}</h3>
                <div className="analytics-chart-wrap">
                  <ResponsiveContainer width="100%" height={300}>
                    <BarChart data={barData} layout="vertical" margin={{ top: 5, right: 30, left: 40, bottom: 5 }}>
                      <CartesianGrid strokeDasharray="3 3" horizontal={true} vertical={false} stroke="var(--border)" />
                      <XAxis type="number" hide />
                      <YAxis dataKey="name" type="category" tick={{ fill: 'var(--text-soft)', fontSize: 12 }} width={80} />
                      <Tooltip
                        cursor={{ fill: 'var(--surface-hover)' }}
                        contentStyle={{ background: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 8 }}
                      />
                      <Bar dataKey="value" fill="var(--accent)" radius={[0, 4, 4, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
                <ul className="analytics-delta-grid mt-4">
                  {metrics.map((m) => (
                    <li key={m.metric} className="analytics-delta-card">
                      <span className="analytics-delta-card__metric">{m.metric}</span>
                      <span className="analytics-delta-card__total">{m.total.toLocaleString()}</span>
                      <span
                        className={`analytics-delta-card__delta ${
                          m.delta_vs_prev_day > 0 ? 'analytics-delta-card__delta--up' : m.delta_vs_prev_day < 0 ? 'analytics-delta-card__delta--down' : ''
                        }`}
                      >
                        {m.delta_vs_prev_day > 0 ? '+' : ''}
                        {m.delta_vs_prev_day.toLocaleString()}
                        {m.delta_percent != null ? ` · ${formatDeltaPct(m.delta_percent)}` : ''}
                      </span>
                    </li>
                  ))}
                </ul>
              </section>
            )}

            <section className="glass-panel analytics-card-chart">
              <div className="analytics-chart-panel__head">
                <h3 className="subsection-title">{t('analytics.metricTrend30d')}</h3>
                <select className="select-sm" value={chartMetric} onChange={(e) => setChartMetric(e.target.value)}>
                  {metrics.map((m) => (
                    <option key={m.metric} value={m.metric}>{m.metric}</option>
                  ))}
                </select>
              </div>
              <div className="analytics-chart-wrap">
                <ResponsiveContainer width="100%" height={300}>
                  <LineChart data={lineData}>
                    <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--border)" />
                    <XAxis dataKey="date" tick={{ fill: 'var(--text-soft)', fontSize: 10 }} minTickGap={20} />
                    <YAxis tick={{ fill: 'var(--text-soft)', fontSize: 12 }} width={35} />
                    <Tooltip contentStyle={{ background: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 8 }} />
                    <Line type="monotone" dataKey="value" stroke="var(--accent)" strokeWidth={2} dot={false} />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </section>
          </div>
        </div>
      )}

      {summary && activeTab === 'accounts' && (
        <div className="analytics-accounts-view" style={viewGapStyle}>
          <section className="glass-panel analytics-card-chart">
            <div className="analytics-chart-panel__head">
              <h3 className="subsection-title">{t('analytics.accountGrowth')}</h3>
              <select className="select-sm" value={growthAccountId} onChange={(e) => setGrowthAccountId(e.target.value)}>
                <option value="all">{t('common.allAccounts')}</option>
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>{a.name} ({a.username})</option>
                ))}
              </select>
            </div>
            <div className="analytics-chart-wrap">
              <ResponsiveContainer width="100%" height={350}>
                <LineChart data={growthData}>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="var(--border)" />
                  <XAxis dataKey="date" tick={{ fill: 'var(--text-soft)', fontSize: 10 }} minTickGap={20} />
                  <YAxis tick={{ fill: 'var(--text-soft)', fontSize: 12 }} width={35} />
                  <Tooltip contentStyle={{ background: 'var(--surface-raised)', border: '1px solid var(--border)', borderRadius: 8 }} />
                  <Line name={t('analytics.followers')} type="monotone" dataKey="followers" stroke="#22c55e" strokeWidth={2} dot={false} />
                  <Line name={t('analytics.following')} type="monotone" dataKey="following" stroke="#8b5cf6" strokeWidth={1} strokeDasharray="4 4" dot={false} />
                  <Line name={t('analytics.networkSize')} type="monotone" dataKey="networkSize" stroke="var(--accent)" strokeWidth={1} dot={false} />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </section>

          <div className="analytics-accounts-grid" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: 'var(--space-6)' }}>
            {accounts.map(acc => {
              const status = accountConnectionStatus(acc)
              const latestGrowth = accountLatestGrowth[acc.id]
              
              return (
                <section key={acc.id} className="glass-panel account-stat-card">
                  <div className="account-stat-card__header">
                    <DestinationAvatar account={acc} />
                    <div className="account-stat-card__identity">
                      <strong className="account-stat-card__name">{acc.name}</strong>
                      <span className="hint">@{acc.username}</span>
                    </div>
                    <span className={`status-pill status-pill--${status}`}>
                      {status === 'active' ? t('common.active') : t('common.reauth')}
                    </span>
                  </div>
                  
                  <div className="account-stat-card__metrics">
                    <div className="account-stat-card__metric">
                      <span className="account-stat-card__metric-value">
                        {latestGrowth?.followers?.toLocaleString() ?? '—'}
                      </span>
                      <span className="account-stat-card__metric-label">{t('analytics.followers')}</span>
                    </div>
                    <div className="account-stat-card__metric">
                      <span className="account-stat-card__metric-value">
                        {latestGrowth?.posts?.toLocaleString() ?? '—'}
                      </span>
                      <span className="account-stat-card__metric-label">{t('analytics.posts')}</span>
                    </div>
                    <div className="account-stat-card__metric">
                      <span className="account-stat-card__metric-value">
                        {latestGrowth?.following?.toLocaleString() ?? '—'}
                      </span>
                      <span className="account-stat-card__metric-label">{t('analytics.following')}</span>
                    </div>
                  </div>

                  <div className="account-stat-card__footer">
                    <span className="hint mono text-xs">{acc.provider} · {acc.instance.replace('https://', '')}</span>
                  </div>
                </section>
              )
            })}
          </div>
        </div>
      )}

      {summary && activeTab === 'posts' && (
        <section className="glass-panel" style={viewGapStyle}>
          <h3 className="subsection-title">{t('analytics.postPerformance')}</h3>
          {posts.length === 0 ? (
            <p className="hint">{t('analytics.noPublishedPosts')}</p>
          ) : (
          <div className="analytics-posts-table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>{t('common.post')}</th>
                  <th>{t('common.publishedAt')}</th>
                  <th className="text-right">{t('common.score')}</th>
                </tr>
              </thead>
              <tbody>
                {posts.map((row) => {
                  const isSelected = selectedPostId === row.post_id
                  return (
                    <Fragment key={row.post_id}>
                      <tr
                      className={fetchPostMetrics ? 'analytics-post-row--clickable' : undefined}
                      data-selected={isSelected || undefined}
                      onClick={
                        fetchPostMetrics
                          ? () => setSelectedPostId(isSelected ? null : row.post_id)
                          : undefined
                      }
                    >
                      <td>
                        <div className="post-table-title">{row.title || t('common.untitled')}</div>
                        <div className="hint mono text-xs">{row.post_id}</div>
                      </td>
                      <td className="text-soft">
                        {(() => {
                          try {
                            const d = parseISO(row.scheduled_at)
                            return format(d, 'MMM d, HH:mm')
                          } catch {
                            return row.scheduled_at
                          }
                        })()}
                      </td>
                      <td className="text-right font-bold">{row.score.toLocaleString()}</td>
                      </tr>
                      {fetchPostMetrics && isSelected ? (
                        <tr className="analytics-post-detail-row">
                          <td colSpan={3}>
                            <PostMetricsDetailPanel
                              loading={postMetricsLoading}
                              metrics={selectedPostMetrics}
                              totals={selectedPostTotals}
                              accounts={accounts}
                            />
                          </td>
                        </tr>
                      ) : null}
                    </Fragment>
                  )
                })}
              </tbody>
            </table>
          </div>
          )}
        </section>
      )}
    </div>
  )
}

