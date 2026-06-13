import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { computeOccurrences, type RecurrenceState } from './recurrenceUtils'

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
