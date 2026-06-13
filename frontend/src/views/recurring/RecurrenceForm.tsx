import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DEFAULT_ORDINAL_OCCURRENCE,
  type OrdinalOccurrence,
  type RecurrenceState,
} from './recurrenceUtils'

const COMMON_TZ = [
  'UTC',
  'Europe/Berlin',
  'Europe/Vienna',
  'Europe/Zurich',
  'Europe/London',
  'Europe/Paris',
  'Europe/Madrid',
  'Europe/Rome',
  'Europe/Amsterdam',
  'Europe/Stockholm',
  'Europe/Moscow',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'America/Mexico_City',
  'America/Toronto',
  'Asia/Tokyo',
  'Asia/Shanghai',
  'Asia/Kolkata',
  'Asia/Singapore',
  'Asia/Dubai',
  'Asia/Seoul',
  'Australia/Sydney',
  'Australia/Melbourne',
  'Pacific/Auckland',
  'Pacific/Honolulu',
  'Africa/Cairo',
  'Africa/Johannesburg',
  'Africa/Lagos',
]

const WEEKDAY_KEYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun']
const UI_TO_GO_WEEKDAY = [1, 2, 3, 4, 5, 6, 0]
const ORDINAL_OPTIONS = [1, 2, 3, 4, 5, -1] as const
const ORDINAL_I18N: Record<number, string> = {
  1: 'recurring.ordinal1',
  2: 'recurring.ordinal2',
  3: 'recurring.ordinal3',
  4: 'recurring.ordinal4',
  5: 'recurring.ordinal5',
  [-1]: 'recurring.ordinalLast',
}

export function RecurrenceForm({
  state,
  onChange,
}: {
  state: RecurrenceState
  onChange: (state: RecurrenceState) => void
}) {
  const { t } = useTranslation()

  const set = useCallback(
    (patch: Partial<RecurrenceState>) => {
      onChange({ ...state, ...patch })
    },
    [state, onChange],
  )

  const toggleWeekday = useCallback(
    (goWd: number) => {
      const exists = state.weekdays.includes(goWd)
      set({
        weekdays: exists
          ? state.weekdays.filter((w) => w !== goWd)
          : [...state.weekdays, goWd].sort(),
      })
    },
    [state.weekdays, set],
  )

  const updateOccurrence = useCallback(
    (index: number, patch: Partial<OrdinalOccurrence>) => {
      const next = state.ordinalOccurrences.map((occ, i) => (i === index ? { ...occ, ...patch } : occ))
      set({ ordinalOccurrences: next })
    },
    [state.ordinalOccurrences, set],
  )

  const addOccurrence = useCallback(() => {
    set({
      ordinalOccurrences: [...state.ordinalOccurrences, { ...DEFAULT_ORDINAL_OCCURRENCE }],
    })
  }, [state.ordinalOccurrences, set])

  const removeOccurrence = useCallback(
    (index: number) => {
      if (state.ordinalOccurrences.length <= 1) {
        return
      }
      set({
        ordinalOccurrences: state.ordinalOccurrences.filter((_, i) => i !== index),
      })
    },
    [state.ordinalOccurrences, set],
  )

  return (
    <div className="recurrence-form">
      <fieldset className="recurrence-form__kind-group">
        <legend className="recurrence-form__legend">{t('recurring.repeat')}</legend>
          {([
            { kind: 'weekly' as const, key: 'recurring.kindWeekly' },
            { kind: 'monthly_dom' as const, key: 'recurring.kindMonthlyDom' },
            { kind: 'monthly_anchor_offset' as const, key: 'recurring.kindMonthlyAnchor' },
            { kind: 'monthly_ordinal_weekday' as const, key: 'recurring.kindMonthlyOrdinal' },
          ]).map(({ kind, key }) => (
            <button
              key={kind}
              type="button"
              className={`recurrence-form__kind-pill${state.kind === kind ? ' recurrence-form__kind-pill--active' : ''}`}
              onClick={() => set({ kind })}
              role="radio"
              aria-checked={state.kind === kind}
            >
              {t(key as 'recurring.kindWeekly')}
            </button>
          ))}
      </fieldset>

      {state.kind === 'weekly' && (
        <div className="recurrence-form__weekdays">
          <span className="recurrence-form__label">{t('recurring.onDays')}</span>
          <div className="recurrence-form__weekday-row">
            {WEEKDAY_KEYS.map((key, i) => {
              const goWd = UI_TO_GO_WEEKDAY[i]
              const active = state.weekdays.includes(goWd)
              return (
                <button
                  key={key}
                  type="button"
                  className={`recurrence-form__weekday-btn${active ? ' recurrence-form__weekday-btn--active' : ''}`}
                  onClick={() => toggleWeekday(goWd)}
                >
                  {t(`weekdays.${key}` as 'weekdays.mon').slice(0, 2)}
                </button>
              )
            })}
          </div>
        </div>
      )}

      {state.kind === 'monthly_dom' && (
        <div className="recurrence-form__field">
          <label className="recurrence-form__label" htmlFor="recurrence-dayofmonth">{t('recurring.dayOfMonth')}</label>
          <input
            id="recurrence-dayofmonth"
            type="number"
            className="recurrence-form__input"
            min={1}
            max={31}
            value={state.dayOfMonth}
            onChange={(e) => set({ dayOfMonth: Math.min(31, Math.max(1, Number(e.target.value) || 1)) })}
          />
        </div>
      )}

      {state.kind === 'monthly_anchor_offset' && (
        <div className="recurrence-form__row">
          <div className="recurrence-form__field">
            <label className="recurrence-form__label">{t('recurring.anchorDay')}</label>
            <input
              type="number"
              className="recurrence-form__input"
              min={1}
              max={31}
              value={state.anchorDay}
              onChange={(e) => set({ anchorDay: Math.min(31, Math.max(1, Number(e.target.value) || 1)) })}
            />
          </div>
          <div className="recurrence-form__field">
            <label className="recurrence-form__label">{t('recurring.offsetDays')}</label>
            <input
              type="number"
              className="recurrence-form__input"
              min={-30}
              max={30}
              value={state.offsetDays}
              onChange={(e) => set({ offsetDays: Math.min(30, Math.max(-30, Number(e.target.value) || 0)) })}
            />
          </div>
        </div>
      )}

      {state.kind === 'monthly_ordinal_weekday' && (
        <div className="recurrence-form__ordinal-block">
          <span className="recurrence-form__label">{t('recurring.occurrences')}</span>
          <p className="hint recurrence-form__ordinal-hint">{t('recurring.occurrencesHint')}</p>
          <div className="recurrence-form__occurrence-list">
            {state.ordinalOccurrences.map((occ, index) => (
              <div key={index} className="recurrence-form__occurrence-row" data-testid={`recurring-occurrence-row-${index}`}>
                <div className="recurrence-form__occurrence-row-head">
                  <span className="recurrence-form__occurrence-title">
                    {t('recurring.occurrenceNumber', { number: index + 1 })}
                  </span>
                  {state.ordinalOccurrences.length > 1 ? (
                    <button
                      type="button"
                      className="btn btn--ghost btn--sm"
                      onClick={() => removeOccurrence(index)}
                      data-testid={`recurring-occurrence-remove-${index}`}
                    >
                      {t('recurring.removeOccurrence')}
                    </button>
                  ) : null}
                </div>
                <div className="recurrence-form__weekdays">
                  <span className="recurrence-form__label">{t('recurring.ordinal')}</span>
                  <div className="recurrence-form__weekday-row">
                    {ORDINAL_OPTIONS.map((ordinal) => {
                      const active = occ.ordinal === ordinal
                      return (
                        <button
                          key={ordinal}
                          type="button"
                          className={`recurrence-form__weekday-btn${active ? ' recurrence-form__weekday-btn--active' : ''}`}
                          onClick={() => updateOccurrence(index, { ordinal })}
                          aria-pressed={active}
                          data-testid={`recurring-ordinal-${index}-${ordinal}`}
                        >
                          {t(ORDINAL_I18N[ordinal] as 'recurring.ordinal1')}
                        </button>
                      )
                    })}
                  </div>
                </div>
                <div className="recurrence-form__weekdays">
                  <span className="recurrence-form__label">{t('recurring.weekday')}</span>
                  <div className="recurrence-form__weekday-row">
                    {WEEKDAY_KEYS.map((key, i) => {
                      const goWd = UI_TO_GO_WEEKDAY[i]
                      const active = occ.weekday === goWd
                      return (
                        <button
                          key={key}
                          type="button"
                          className={`recurrence-form__weekday-btn${active ? ' recurrence-form__weekday-btn--active' : ''}`}
                          onClick={() => updateOccurrence(index, { weekday: goWd })}
                          aria-pressed={active}
                          data-testid={`recurring-ordinal-weekday-${index}-${key}`}
                        >
                          {t(`weekdays.${key}` as 'weekdays.mon').slice(0, 2)}
                        </button>
                      )
                    })}
                  </div>
                </div>
              </div>
            ))}
          </div>
          <button
            type="button"
            className="btn btn--secondary btn--sm"
            onClick={addOccurrence}
            data-testid="recurring-add-occurrence"
          >
            {t('recurring.addOccurrence')}
          </button>
        </div>
      )}

      <div className="recurrence-form__row">
        <div className="recurrence-form__field">
          <label className="recurrence-form__label" htmlFor="recurrence-hour">{t('recurring.hour')}</label>
          <input
            id="recurrence-hour"
            type="number"
            className="recurrence-form__input recurrence-form__input--narrow"
            min={0}
            max={23}
            value={state.hour}
            onChange={(e) => set({ hour: Math.min(23, Math.max(0, Number(e.target.value) || 0)) })}
          />
        </div>
        <div className="recurrence-form__field">
          <label className="recurrence-form__label" htmlFor="recurrence-minute">{t('recurring.minute')}</label>
          <input
            id="recurrence-minute"
            type="number"
            className="recurrence-form__input recurrence-form__input--narrow"
            min={0}
            max={59}
            value={state.minute}
            onChange={(e) => set({ minute: Math.min(59, Math.max(0, Number(e.target.value) || 0)) })}
          />
        </div>
        <div className="recurrence-form__field recurrence-form__field--grow">
          <label className="recurrence-form__label" htmlFor="recurrence-timezone">{t('recurring.timezone')}</label>
          <input
            id="recurrence-timezone"
            type="text"
            className="recurrence-form__input"
            list="tz-list"
            value={state.timezone}
            onChange={(e) => set({ timezone: e.target.value })}
            placeholder="UTC"
          />
          <datalist id="tz-list">
            {COMMON_TZ.map((tz) => (
              <option key={tz} value={tz} />
            ))}
          </datalist>
        </div>
      </div>
    </div>
  )
}
