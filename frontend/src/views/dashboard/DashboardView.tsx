import { useCallback, useEffect, useMemo, useState } from 'react'
import { format, parseISO } from 'date-fns'
import type { BackendMetricHistoryPoint } from '../../api'
import { DestinationAvatar } from '../../components/post/DestinationAvatar'
import { Icon } from '../../icons'
import { accountConnectionStatus } from '../../mappers'
import type { AccountRecord, PostRecord } from '../../types'
import { sharedAccountLabels } from '../../schedule'
import { Area, AreaChart, ResponsiveContainer, Tooltip } from 'recharts'

const ENGAGEMENT_METRICS = ['likes', 'reposts', 'replies'] as const

function MetricSparkline({
  title,
  color,
  points,
  loading,
}: {
  title: string
  color: string
  points: { date: string; value: number }[]
  loading: boolean
}) {
  const data = useMemo(() => points.map((p) => ({ ...p, label: p.date })), [points])
  const lastValue = useMemo(() => {
    if (data.length === 0) return null
    return data[data.length - 1].value
  }, [data])

  return (
    <div className="dashboard-spark">
      <div className="dashboard-spark__head">
        <div className="dashboard-spark__info">
          <span className="dashboard-spark__title">{title}</span>
          <span className="dashboard-spark__subtitle">Last 7 days</span>
        </div>
        {lastValue !== null && !loading && (
          <div className="dashboard-spark__value" style={{ color: lastValue < 0 ? '#ef4444' : (lastValue > 0 ? '#22c55e' : 'inherit') }}>
            {lastValue > 0 ? `+${lastValue}` : lastValue}
          </div>
        )}
      </div>
      <div className="dashboard-spark__chart">
        {loading ? (
          <p className="hint dashboard-spark__placeholder">Loading…</p>
        ) : data.length === 0 ? (
          <p className="hint dashboard-spark__placeholder">No data yet</p>
        ) : (
          <ResponsiveContainer width="100%" height={88}>
            <AreaChart data={data} margin={{ top: 4, right: 4, left: 0, bottom: 0 }}>
              <Tooltip contentStyle={{ fontSize: '12px', borderRadius: '8px' }} />
              <Area type="monotone" dataKey="value" stroke={color} fill={color} fillOpacity={0.12} strokeWidth={1.75} isAnimationActive={false} />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  )
}

export function DashboardView({
  teamName,
  upcomingPosts,
  accounts,
  fetchSeries,
  fetchGrowth,
  onOpenPost,
  onOpenSchedule,
  onOpenAccounts,
}: {
  teamName: string
  upcomingPosts: PostRecord[]
  accounts: AccountRecord[]
  fetchSeries: (metric: string) => Promise<{ series: BackendMetricHistoryPoint[] }>
  fetchGrowth: (accountId: string, opts?: { days?: number }) => Promise<{ days: number; account: string; series: import('../../api').BackendAccountGrowthPoint[] }>
  onOpenPost: (postId: string) => void
  onOpenSchedule: () => void
  onOpenAccounts: () => void
}) {
  const [seriesByMetric, setSeriesByMetric] = useState<Record<string, BackendMetricHistoryPoint[]>>({})
  const [reachSeries, setReachSeries] = useState<{ date: string; value: number }[]>([])
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

      const next: Record<string, BackendMetricHistoryPoint[]> = {}
      for (const [k, v] of engagementResults) {
        next[k] = v
      }
      setSeriesByMetric(next)

      const reach = (growthResult.series ?? []).map((p, i, arr) => {
        const prev = arr[i - 1]
        // Täglicher Zuwachs: Heutige Follower minus gestrige Follower
        const delta = prev ? p.followers - prev.followers : 0
        return {
          date: p.date,
          value: delta,
        }
      })
      setReachSeries(reach)
    } catch {
      setSeriesByMetric({})
      setReachSeries([])
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
    likes: 'Likes',
    reposts: 'Shares',
    replies: 'Replies',
  }

  const performancePanel = (
    <section className="glass-panel dashboard-panel">
      <div className="dashboard-panel__header">
        <div>
          <p className="eyebrow">Performance</p>
          <h2 className="dashboard-panel__title">Recent engagement</h2>
          <p className="hint">Team totals from synced analytics</p>
        </div>
        <button type="button" className="button button--secondary" onClick={() => void loadCharts()} disabled={loadingCharts}>
          {loadingCharts ? <Icon name="loader" className="reload-spin" /> : <Icon name="refresh" />}
          <span>Refresh</span>
        </button>
      </div>
      <div className="dashboard-spark-grid">
        <MetricSparkline
          title="Follower Trend"
          color="#38bdf8"
          points={reachSeries}
          loading={loadingCharts}
        />
        {ENGAGEMENT_METRICS.map((m) => (
          <MetricSparkline
            key={m}
            title={chartTitles[m]}
            color={chartColors[m] ?? 'var(--accent)'}
            points={(seriesByMetric[m] ?? []).map((p) => ({ date: p.date, value: p.value }))}
            loading={loadingCharts}
          />
        ))}
      </div>
    </section>
  )

  const accountPanel = (
    <section className="glass-panel dashboard-panel">
      <div className="dashboard-panel__header">
        <div>
          <p className="eyebrow">Account health</p>
          <h2 className="dashboard-panel__title">Connection status</h2>
          <p className="hint">OAuth expiry determines reconnect prompts</p>
        </div>
        <button type="button" className="button button--secondary" onClick={onOpenAccounts}>
          <Icon name="share" className="dashboard-manage-accounts__icon" />
          <span>Manage accounts</span>
        </button>
      </div>
      {accounts.length === 0 ? (
        <p className="hint dashboard-panel__empty">No accounts linked to this workspace yet.</p>
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
                  {status === 'active' ? 'Active' : 'Needs re-auth'}
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
          <p className="eyebrow">Schedule</p>
          <h2 className="dashboard-panel__title">Upcoming posts</h2>
          <p className="hint">{teamName}</p>
        </div>
        <button type="button" className="button button--secondary" onClick={onOpenSchedule}>
          <Icon name="calendar" className="inline-icon" />
          <span>Open schedule</span>
        </button>
      </div>
      {upcomingPosts.length === 0 ? (
        <p className="hint dashboard-panel__empty">No upcoming scheduled or draft posts. Create a post to fill your calendar.</p>
      ) : (
        <ul className="dashboard-scheduled-cards" role="list" aria-label="Upcoming scheduled posts">
          {upcomingPosts.map((post) => {
            const when = parseISO(post.scheduledAt)
            const snippet = post.title?.trim() || post.content.slice(0, 80) || 'Untitled'
            return (
              <li key={post.id} className="dashboard-scheduled-cards__item">
                <button type="button" className="dashboard-scheduled-card" onClick={() => onOpenPost(post.id)}>
                  <div className="dashboard-scheduled-card__top">
                    <time className="dashboard-scheduled-card__time" dateTime={post.scheduledAt}>
                      <span className="dashboard-scheduled-card__date">{format(when, 'MMM d')}</span>
                      <span className="dashboard-scheduled-card__clock">{format(when, 'HH:mm')}</span>
                    </time>
                    {post.status === 'draft' ? <span className="dashboard-scheduled-card__draft">Draft</span> : null}
                  </div>
                  <p className="dashboard-scheduled-card__title">{snippet}</p>
                  <div className="dashboard-scheduled-card__accounts" aria-hidden="true">
                    {sharedAccountLabels(post, accounts).map((a) => (
                      <DestinationAvatar key={a.id} account={a} compact />
                    ))}
                  </div>
                </button>
              </li>
            )
          })}
        </ul>
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
