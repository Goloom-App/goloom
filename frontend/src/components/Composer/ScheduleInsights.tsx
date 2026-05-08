import { useEffect, useMemo, useState } from 'react'
import { createApiClient } from '../../api'
import type { TeamSchedulingPreferences } from '../../types'

type Api = ReturnType<typeof createApiClient>

function labelHour(h: number, use24h: boolean): string {
  if (use24h) {
    return `${String(h).padStart(2, '0')}`
  }
  const mod = h % 12
  const hr = mod === 0 ? 12 : mod
  const sfx = h < 12 ? 'a' : 'p'
  return `${hr}${sfx}`
}

/** Apply HH:mm to existing datetime-local value (preserves date part). */
export function mergeTimeIntoDateTimeLocal(scheduledAt: string, timeHHMM: string): string {
  const [hhS, mmS] = timeHHMM.split(':')
  const hh = Number(hhS)
  const mm = Number(mmS)
  if (Number.isNaN(hh) || Number.isNaN(mm)) {
    return scheduledAt
  }
  const datePart =
    scheduledAt && scheduledAt.length >= 10 ? scheduledAt.slice(0, 10) : new Date().toISOString().slice(0, 10)
  return `${datePart}T${String(hh).padStart(2, '0')}:${String(mm).padStart(2, '0')}`
}

export function ScheduleInsights({
  teamId,
  api,
  scheduledAt,
  setScheduledAt,
  schedulingPreferences,
}: {
  teamId?: string | null
  api?: Api | null
  scheduledAt: string
  setScheduledAt: (v: string) => void
  schedulingPreferences?: TeamSchedulingPreferences | null
}) {
  const [hours, setHours] = useState<Array<{ hour: number; score: number }>>([])
  const [use24h, setUse24h] = useState(true)

  useEffect(() => {
    if (!api || !teamId) {
      setHours([])
      return
    }
    let cancelled = false
    void api
      .getTeamEngagementHours(teamId, { days: 90 })
      .then((res) => {
        if (!cancelled) {
          setHours(res.hours ?? [])
        }
      })
      .catch(() => {
        if (!cancelled) {
          setHours([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [api, teamId])

  const byHour = useMemo(() => {
    const m = new Map<number, number>()
    for (const row of hours) {
      m.set(row.hour, row.score)
    }
    return m
  }, [hours])

  const maxScore = useMemo(() => Math.max(1, ...Array.from(byHour.values()), 0), [byHour])

  const slots = schedulingPreferences?.default_timeslots ?? []

  return (
    <div className="schedule-insights">
      <div className="schedule-insights__toolbar">
        <span className="eyebrow">Engagement by hour (UTC)</span>
        <label className="schedule-insights__toggle">
          <input type="checkbox" checked={use24h} onChange={(e) => setUse24h(e.target.checked)} />
          <span>24h labels</span>
        </label>
      </div>
      <div className="schedule-insights__heatmap" role="img" aria-label="Engagement heatmap by hour">
        {Array.from({ length: 24 }, (_, h) => {
          const score = byHour.get(h) ?? 0
          const intensity = score / maxScore
          return (
            <button
              key={h}
              type="button"
              className="schedule-insights__cell"
              style={{ opacity: 0.25 + intensity * 0.75 }}
              title={`${h}:00 UTC — score ${score}`}
              onClick={() => {
                const cur = scheduledAt && scheduledAt.includes('T') ? scheduledAt : `${new Date().toISOString().slice(0, 10)}T12:00`
                const datePart = cur.slice(0, 10)
                setScheduledAt(`${datePart}T${String(h).padStart(2, '0')}:${cur.slice(14, 16) || '00'}`)
              }}
            >
              <span className="schedule-insights__cell-label">{labelHour(h, use24h)}</span>
            </button>
          )
        })}
      </div>
      {slots.length > 0 ? (
        <div className="schedule-insights__slots">
          <span className="hint">Default timeslots</span>
          <div className="inline-cluster" style={{ flexWrap: 'wrap', gap: '0.35rem' }}>
            {slots.map((slot) => (
              <button
                key={slot}
                type="button"
                className="button button--secondary button--small"
                onClick={() => setScheduledAt(mergeTimeIntoDateTimeLocal(scheduledAt, slot))}
              >
                {slot}
              </button>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  )
}
