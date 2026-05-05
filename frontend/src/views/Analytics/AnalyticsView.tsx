import { useCallback, useEffect, useMemo, useState } from 'react'
import { format, parseISO } from 'date-fns'
import { Bar, BarChart, CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { BackendAccountGrowthPoint, BackendMetricHistoryPoint, BackendPostAnalyticsListRow, BackendTeamAnalyticsReport } from '../../api'
import type { AccountRecord } from '../../types'

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
}: {
  teamId: string
  accounts: AccountRecord[]
  fetchSummary: (opts?: { top_posts?: number }) => Promise<BackendTeamAnalyticsReport>
  fetchPosts: (opts?: { sort?: string; limit?: number; offset?: number }) => Promise<{ items: BackendPostAnalyticsListRow[] }>
  fetchChart: (opts: { metric: string; days?: number }) => Promise<{ metric: string; days: number; series: BackendMetricHistoryPoint[] }>
  fetchAccountGrowth: (accountId: string, opts?: { days?: number }) => Promise<{ days: number; account: string; series: BackendAccountGrowthPoint[] }>
}) {
  const [summary, setSummary] = useState<BackendTeamAnalyticsReport | null>(null)
  const [posts, setPosts] = useState<BackendPostAnalyticsListRow[]>([])
  const [series, setSeries] = useState<BackendMetricHistoryPoint[]>([])
  const [chartMetric, setChartMetric] = useState<string>('')
  const [growthAccountId, setGrowthAccountId] = useState<string>('all')
  const [accountGrowthSeries, setAccountGrowthSeries] = useState<BackendAccountGrowthPoint[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

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
      setError(e instanceof Error ? e.message : 'Failed to load analytics')
    } finally {
      setLoading(false)
    }
  }, [fetchPosts, fetchSummary, teamId])

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

  const barData = useMemo(() => {
    if (!summary?.metrics?.length) {
      return []
    }
    return [...summary.metrics]
      .map((m) => ({ name: m.metric, value: m.total }))
      .sort((a, b) => b.value - a.value)
  }, [summary])

  const totalEngagement = useMemo(() => {
    if (!summary?.metrics?.length) {
      return 0
    }
    return summary.metrics.reduce((a, m) => a + m.total, 0)
  }, [summary])

  const lineData = useMemo(() => series.map((p) => ({ date: p.date, value: p.value })), [series])
  const growthData = useMemo(
    () =>
      accountGrowthSeries.map((point) => ({
        date: point.date,
        followers: point.followers,
        following: point.following,
        posts: point.posts,
        reach: point.followers + point.following,
      })),
    [accountGrowthSeries],
  )

  if (!teamId) {
    return <p className="hint">Select a team to view analytics.</p>
  }

  return (
    <div className="analytics-view">
      <div className="analytics-view__toolbar">
        <button type="button" className="button button--secondary" onClick={() => void load()} disabled={loading}>
          Refresh
        </button>
      </div>
      {error ? <p className="status-banner__error">{error}</p> : null}
      {loading && !summary ? <p className="hint">Loading analytics…</p> : null}

      {summary ? (
        <>
          <div className="analytics-cards">
            <section className="glass-panel analytics-card">
              <p className="eyebrow">Total engagement</p>
              <p className="analytics-card__value">{totalEngagement.toLocaleString()}</p>
              <p className="hint">Sum of live metric totals on published posts in this workspace.</p>
            </section>
            <section className="glass-panel analytics-card">
              <p className="eyebrow">Metric types</p>
              <p className="analytics-card__value">{summary.metrics.length}</p>
              <p className="hint">Distinct metric names (likes, reposts, …).</p>
            </section>
            <section className="glass-panel analytics-card">
              <p className="eyebrow">Posts ranked</p>
              <p className="analytics-card__value">{posts.length}</p>
              <p className="hint">Published posts returned for the performance table.</p>
            </section>
          </div>

          {summary.metrics.length > 0 ? (
            <section className="glass-panel analytics-deltas-panel">
              <h3 className="subsection-title">Totals and day-over-day</h3>
              <p className="hint">Delta compares the latest and previous calendar day in metric history (requires two days of snapshots).</p>
              <ul className="analytics-delta-grid">
                {summary.metrics.map((m) => (
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
          ) : null}

          <section className="glass-panel analytics-chart-panel">
            <h3 className="subsection-title">Metrics breakdown</h3>
            {barData.length === 0 ? (
              <p className="hint">No metrics yet. Published posts need a successful metric sync from the server.</p>
            ) : (
              <div className="analytics-chart-wrap">
                <ResponsiveContainer width="100%" height={280}>
                  <BarChart data={barData} margin={{ top: 8, right: 16, left: 0, bottom: 8 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                    <XAxis dataKey="name" tick={{ fill: 'var(--text-soft)', fontSize: 12 }} />
                    <YAxis tick={{ fill: 'var(--text-soft)', fontSize: 12 }} allowDecimals={false} />
                    <Tooltip
                      contentStyle={{
                        background: 'var(--surface-raised)',
                        border: '1px solid var(--border)',
                        borderRadius: 8,
                        color: 'var(--text)',
                      }}
                    />
                    <Bar dataKey="value" fill="var(--accent)" radius={[4, 4, 0, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            )}
          </section>

          {summary.metrics.length > 0 ? (
            <section className="glass-panel analytics-chart-panel">
              <div className="analytics-chart-panel__head">
                <h3 className="subsection-title">Trend (30 days)</h3>
                <label className="analytics-metric-select">
                  <span className="hint">Metric</span>
                  <select value={chartMetric} onChange={(e) => setChartMetric(e.target.value)}>
                    {summary.metrics.map((m) => (
                      <option key={m.metric} value={m.metric}>
                        {m.metric}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              {lineData.length === 0 ? (
                <p className="hint">No history points for this metric in the selected window yet.</p>
              ) : (
                <div className="analytics-chart-wrap">
                  <ResponsiveContainer width="100%" height={260}>
                    <LineChart data={lineData} margin={{ top: 8, right: 16, left: 0, bottom: 8 }}>
                      <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                      <XAxis dataKey="date" tick={{ fill: 'var(--text-soft)', fontSize: 11 }} />
                      <YAxis tick={{ fill: 'var(--text-soft)', fontSize: 12 }} allowDecimals={false} />
                      <Tooltip
                        contentStyle={{
                          background: 'var(--surface-raised)',
                          border: '1px solid var(--border)',
                          borderRadius: 8,
                          color: 'var(--text)',
                        }}
                      />
                      <Line type="monotone" dataKey="value" stroke="var(--accent)" strokeWidth={2} dot={false} />
                    </LineChart>
                  </ResponsiveContainer>
                </div>
              )}
            </section>
          ) : null}

          <section className="glass-panel analytics-chart-panel">
            <div className="analytics-chart-panel__head">
              <h3 className="subsection-title">Follower growth (30 days)</h3>
              <label className="analytics-metric-select">
                <span className="hint">Account</span>
                <select value={growthAccountId} onChange={(e) => setGrowthAccountId(e.target.value)}>
                  <option value="all">All accounts</option>
                  {accounts.map((account) => (
                    <option key={account.id} value={account.id}>
                      {account.name}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            {growthData.length === 0 ? (
              <p className="hint">No account growth snapshots yet.</p>
            ) : (
              <div className="analytics-chart-wrap">
                <ResponsiveContainer width="100%" height={260}>
                  <LineChart data={growthData} margin={{ top: 8, right: 16, left: 0, bottom: 8 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                    <XAxis dataKey="date" tick={{ fill: 'var(--text-soft)', fontSize: 11 }} />
                    <YAxis tick={{ fill: 'var(--text-soft)', fontSize: 12 }} allowDecimals={false} />
                    <Tooltip
                      contentStyle={{
                        background: 'var(--surface-raised)',
                        border: '1px solid var(--border)',
                        borderRadius: 8,
                        color: 'var(--text)',
                      }}
                    />
                    <Line type="monotone" dataKey="followers" stroke="#22c55e" strokeWidth={2} dot={false} />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            )}
          </section>

          <section className="glass-panel analytics-chart-panel">
            <h3 className="subsection-title">Total reach (30 days)</h3>
            {growthData.length === 0 ? (
              <p className="hint">No account growth snapshots yet.</p>
            ) : (
              <div className="analytics-chart-wrap">
                <ResponsiveContainer width="100%" height={260}>
                  <LineChart data={growthData} margin={{ top: 8, right: 16, left: 0, bottom: 8 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                    <XAxis dataKey="date" tick={{ fill: 'var(--text-soft)', fontSize: 11 }} />
                    <YAxis tick={{ fill: 'var(--text-soft)', fontSize: 12 }} allowDecimals={false} />
                    <Tooltip
                      contentStyle={{
                        background: 'var(--surface-raised)',
                        border: '1px solid var(--border)',
                        borderRadius: 8,
                        color: 'var(--text)',
                      }}
                    />
                    <Line type="monotone" dataKey="reach" stroke="#38bdf8" strokeWidth={2} dot={false} />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            )}
          </section>

          <section className="glass-panel analytics-top-posts">
            <h3 className="subsection-title">Post performance</h3>
            {posts.length === 0 ? (
              <p className="hint">No published posts with metrics in this workspace yet.</p>
            ) : (
              <div className="analytics-posts-table-wrap">
                <table className="data-table analytics-posts-table">
                  <thead>
                    <tr>
                      <th>Title</th>
                      <th>Scheduled</th>
                      <th>Score</th>
                    </tr>
                  </thead>
                  <tbody>
                    {posts.map((row) => (
                      <tr key={row.post_id}>
                        <td>
                          <strong>{row.title || 'Untitled'}</strong>
                          <div className="hint mono">{row.post_id}</div>
                        </td>
                        <td>
                          {(() => {
                            try {
                              const d = parseISO(row.scheduled_at)
                              return format(d, 'PPp')
                            } catch {
                              return row.scheduled_at
                            }
                          })()}
                        </td>
                        <td className="analytics-posts-table__score">{row.score.toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          <section className="glass-panel analytics-top-posts">
            <h3 className="subsection-title">Top posts (summary)</h3>
            {summary.top_posts.length === 0 ? (
              <p className="hint">No ranked posts yet.</p>
            ) : (
              <ul className="analytics-top-list">
                {summary.top_posts.map((row) => (
                  <li key={row.post_id} className="analytics-top-list__row">
                    <div>
                      <strong>{row.title || 'Untitled'}</strong>
                      <div className="hint mono">{row.post_id}</div>
                    </div>
                    <span className="analytics-top-list__score">{row.score.toLocaleString()}</span>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </>
      ) : null}
    </div>
  )
}
