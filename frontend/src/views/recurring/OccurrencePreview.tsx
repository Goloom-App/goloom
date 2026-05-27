import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { RecurrenceState } from './RecurrenceForm'

export function computeOccurrences(state: RecurrenceState, count: number): Date[] {
  const results: Date[] = []
  const now = new Date()
  const cursor = new Date(now.getFullYear(), now.getMonth(), now.getDate(), state.hour, state.minute, 0, 0)

  let iterations = 0
  const maxIterations = count * 365

  while (results.length < count && iterations < maxIterations) {
    iterations++

    let matches = false
    const wd = cursor.getDay()

    if (state.kind === 'weekly') {
      if (state.weekdays.includes(wd)) {
        cursor.setHours(state.hour, state.minute, 0, 0)
        if (cursor.getTime() > now.getTime()) {
          matches = true
        }
      }
    } else if (state.kind === 'monthly_dom') {
      if (cursor.getDate() === state.dayOfMonth) {
        cursor.setHours(state.hour, state.minute, 0, 0)
        if (cursor.getTime() > now.getTime()) {
          matches = true
        }
      }
    } else if (state.kind === 'monthly_anchor_offset') {
      const monthEnd = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 0)
      const anchorDay = Math.min(state.anchorDay, monthEnd.getDate())
      const anchorDate = new Date(cursor.getFullYear(), cursor.getMonth(), anchorDay, state.hour, state.minute, 0, 0)
      anchorDate.setDate(anchorDate.getDate() + state.offsetDays)

      if (
        anchorDate.getMonth() === cursor.getMonth() &&
        anchorDate.getFullYear() === cursor.getFullYear() &&
        anchorDate.getDate() === cursor.getDate()
      ) {
        anchorDate.setHours(state.hour, state.minute, 0, 0)
        if (anchorDate.getTime() > now.getTime()) {
          results.push(new Date(anchorDate))
        }
      }
    } else if (state.kind === 'monthly_ordinal_weekday') {
      const maxd = new Date(cursor.getFullYear(), cursor.getMonth() + 1, 0).getDate()
      let day: number
      if (state.ordinal === -1) {
        day = maxd
        while (day > 0) {
          const t = new Date(cursor.getFullYear(), cursor.getMonth(), day)
          if (t.getDay() === state.ordinalWeekday) { break }
          day--
        }
      } else {
        const first = new Date(cursor.getFullYear(), cursor.getMonth(), 1).getDay()
        const offset = (state.ordinalWeekday - first + 7) % 7
        day = 1 + offset + (state.ordinal - 1) * 7
      }
      if (day >= 1 && day <= maxd && cursor.getDate() === day) {
        cursor.setHours(state.hour, state.minute, 0, 0)
        if (cursor.getTime() > now.getTime()) {
          matches = true
        }
      }
    }

    if (matches) {
      results.push(new Date(cursor))
    }

    cursor.setDate(cursor.getDate() + 1)
  }

  return results.slice(0, count)
}

export function OccurrencePreview({ state }: { state: RecurrenceState }) {
  const { t } = useTranslation()
  const occurrences = useMemo(() => computeOccurrences(state, 5), [state])

  return (
    <div className="occurrence-preview">
      <h3 className="occurrence-preview__heading">{t('recurring.preview')}</h3>
      {occurrences.length === 0 ? (
        <p className="occurrence-preview__empty">{t('recurring.noOccurrences')}</p>
      ) : (
        <ol className="occurrence-preview__list">
          {occurrences.map((d, i) => (
            <li key={i} className="occurrence-preview__item">
              {d.toLocaleDateString([], {
                weekday: 'long',
                day: 'numeric',
                month: 'long',
                year: 'numeric',
              })}
              {' — '}
              {d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
            </li>
          ))}
        </ol>
      )}
    </div>
  )
}
