import { useEffect } from 'react'

import type { BackendTeam } from '../../api'

export function AdminAITab({
  teams,
  loading,
  onLoad,
}: {
  teams: BackendTeam[]
  loading: boolean
  onLoad: () => void | Promise<void>
}) {
  useEffect(() => {
    void onLoad()
  }, [onLoad])

  return (
    <div className="admin-tab-panel stack stack--lg">
      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <div>
            <h2 className="admin-section__title">AI Agents</h2>
            <p className="hint admin-section__hint">
              Teams with AI features enabled on this instance.
            </p>
          </div>
          {loading ? <span className="admin-section__badge hint">Loading...</span> : null}
        </header>

        {teams.length === 0 && !loading ? (
          <p className="hint">No teams have AI features enabled yet.</p>
        ) : (
          <table className="data-table">
            <thead>
              <tr>
                <th>Team</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {teams.map((team) => (
                <tr key={team.id}>
                  <td>
                    <strong>{team.name}</strong>
                    {team.description ? (
                      <p className="eyebrow m-0">{team.description}</p>
                    ) : null}
                  </td>
                  <td>
                    <span className="admin-pill admin-pill--ok">
                      <span className="status-dot status-dot--active" aria-hidden />
                      AI enabled
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  )
}
