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

  return (
    <div className="dashboard-spark">
      <div className="dashboard-spark__head">
        <span className="dashboard-spark__title">{title}</span>
        <span className="dashboard-spark__subtitle">Last 7 days</span>
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
  onOpenPost,
  onOpenSchedule,
  onOpenAccounts,
}: {
  teamName: string
  upcomingPosts: PostRecord[]
  accounts: AccountRecord[]
  fetchSeries: (metric: string) => Promise<{ series: BackendMetricHistoryPoint[] }>
  onOpenPost: (postId: string) => void
  onOpenSchedule: () => void
  onOpenAccounts: () => void
}) {
  const [seriesByMetric, setSeriesByMetric] = useState<Record<string, BackendMetricHistoryPoint[]>>({})
  const [loadingCharts, setLoadingCharts] = useState(false)

  const loadCharts = useCallback(async () => {
    setLoadingCharts(true)
    try {
      const results = await Promise.all(
        ENGAGEMENT_METRICS.map(async (m) => {
          const res = await fetchSeries(m)
          return [m, res.series ?? []] as const
        }),
      )
      const next: Record<string, BackendMetricHistoryPoint[]> = {}
      for (const [k, v] of results) {
        next[k] = v
      }
      setSeriesByMetric(next)
    } catch {
      setSeriesByMetric({})
    } finally {
      setLoadingCharts(false)
    }
  }, [fetchSeries])

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

  return (
    <div className="dashboard-view">
      <section className="glass-panel dashboard-panel">
        <div className="dashboard-panel__header">
          <div>
            <p className="eyebrow">Upcoming</p>
            <h2 className="dashboard-panel__title">Scheduled posts</h2>
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
          <ul className="dashboard-upcoming">
            {upcomingPosts.map((post) => (
              <li key={post.id}>
                <button type="button" className="dashboard-upcoming__row" onClick={() => onOpenPost(post.id)}>
                  <div className="dashboard-upcoming__time">
                    <span>{format(parseISO(post.scheduledAt), 'MMM d')}</span>
                    <strong>{format(parseISO(post.scheduledAt), 'HH:mm')}</strong>
                  </div>
                  <div className="dashboard-upcoming__main">
                    <span className="dashboard-upcoming__title">{post.title?.trim() || post.content.slice(0, 56) || 'Untitled'}</span>
                    <span className="dashboard-upcoming__accounts">
                      {sharedAccountLabels(post, accounts).map((a) => (
                        <DestinationAvatar key={a.id} account={a} compact />
                      ))}
                    </span>
                  </div>
                  {post.status === 'draft' ? <span className="dashboard-upcoming__draft">Draft</span> : null}
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="glass-panel dashboard-panel">
        <div className="dashboard-panel__header">
          <div>
            <p className="eyebrow">Performance</p>
            <h2 className="dashboard-panel__title">Recent engagement</h2>
            <p className="hint">Team totals from synced analytics</p>
          </div>
          <button type="button" className="button button--secondary" onClick={() => void loadCharts()} disabled={loadingCharts}>
            Refresh
          </button>
        </div>
        <div className="dashboard-spark-grid">
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

      <section className="glass-panel dashboard-panel">
        <div className="dashboard-panel__header">
          <div>
            <p className="eyebrow">Connections</p>
            <h2 className="dashboard-panel__title">Account status</h2>
            <p className="hint">OAuth expiry determines reconnect prompts</p>
          </div>
          <button type="button" className="button button--secondary" onClick={onOpenAccounts}>
            <Icon name="channels" className="inline-icon" />
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
    </div>
  )
}
