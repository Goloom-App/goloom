import { useCallback, useEffect, useMemo, useState } from 'react'
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import type { BackendTeamAnalytics } from '../../api'

export function AnalyticsView({
  teamId,
  fetchAnalytics,
}: {
  teamId: string
  fetchAnalytics: (opts?: { top_posts?: number }) => Promise<BackendTeamAnalytics>
}) {
  const [data, setData] = useState<BackendTeamAnalytics | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!teamId) {
      return
    }
    setLoading(true)
    setError(null)
    try {
      const res = await fetchAnalytics({ top_posts: 12 })
      setData(res)
    } catch (e) {
      setData(null)
      setError(e instanceof Error ? e.message : 'Failed to load analytics')
    } finally {
      setLoading(false)
    }
  }, [fetchAnalytics, teamId])

  useEffect(() => {
    void load()
  }, [load])

  const barData = useMemo(() => {
    if (!data?.metrics_total) {
      return []
    }
    return Object.entries(data.metrics_total)
      .map(([name, value]) => ({ name, value }))
      .sort((a, b) => b.value - a.value)
  }, [data])

  const totalEngagement = useMemo(() => {
    if (!data?.metrics_total) {
      return 0
    }
    return Object.values(data.metrics_total).reduce((a, b) => a + b, 0)
  }, [data])

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
      {loading && !data ? <p className="hint">Loading analytics…</p> : null}

      {data ? (
        <>
          <div className="analytics-cards">
            <section className="glass-panel analytics-card">
              <p className="eyebrow">Total engagement</p>
              <p className="analytics-card__value">{totalEngagement.toLocaleString()}</p>
              <p className="hint">Sum of stored metrics on published posts in this workspace.</p>
            </section>
            <section className="glass-panel analytics-card">
              <p className="eyebrow">Metric types</p>
              <p className="analytics-card__value">{Object.keys(data.metrics_total).length}</p>
              <p className="hint">Distinct metric names (likes, reposts, …).</p>
            </section>
            <section className="glass-panel analytics-card">
              <p className="eyebrow">Top posts tracked</p>
              <p className="analytics-card__value">{data.top_posts?.length ?? 0}</p>
              <p className="hint">Ranked by summed metrics after publish.</p>
            </section>
          </div>

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

          <section className="glass-panel analytics-top-posts">
            <h3 className="subsection-title">Top posts</h3>
            {data.top_posts.length === 0 ? (
              <p className="hint">No ranked posts yet.</p>
            ) : (
              <ul className="analytics-top-list">
                {data.top_posts.map((row) => (
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
