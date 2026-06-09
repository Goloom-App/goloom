import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { normalizeOrdinals } from './recurrenceUtils'

export type RecurrenceKind = 'weekly' | 'monthly_dom' | 'monthly_anchor_offset' | 'monthly_ordinal_weekday'

export interface RecurrenceState {
  kind: RecurrenceKind
  weekdays: number[]
  hour: number
  minute: number
  timezone: string
  dayOfMonth: number
  anchorDay: number
  offsetDays: number
  ordinals: number[]
  ordinalWeekday: number
}

export const DEFAULT_RECURRENCE: RecurrenceState = {
  kind: 'weekly',
  weekdays: [1],
  hour: 9,
  minute: 0,
  timezone: 'UTC',
  dayOfMonth: 15,
  anchorDay: 15,
  offsetDays: -3,
  ordinals: [1],
  ordinalWeekday: 1,
}

export function recurrenceStateToJSON(state: RecurrenceState): string {
  const obj: Record<string, unknown> = {
    kind: state.kind,
    hour: state.hour,
    minute: state.minute,
    timezone: state.timezone,
  }
  if (state.kind === 'weekly') {
    obj.weekdays = state.weekdays
  } else if (state.kind === 'monthly_dom') {
    obj.day_of_month = state.dayOfMonth
  } else if (state.kind === 'monthly_anchor_offset') {
    obj.anchor_day = state.anchorDay
    obj.offset_days = state.offsetDays
  } else if (state.kind === 'monthly_ordinal_weekday') {
    obj.ordinals = state.ordinals
    obj.ordinal_weekday = state.ordinalWeekday
  }
  return JSON.stringify(obj, null, 2)
}

export function parseRecurrenceJSON(raw: string): RecurrenceState {
  try {
    const obj = JSON.parse(raw)
    return {
      kind: obj.kind || 'weekly',
      weekdays: Array.isArray(obj.weekdays) ? obj.weekdays : [1],
      hour: typeof obj.hour === 'number' ? obj.hour : 9,
      minute: typeof obj.minute === 'number' ? obj.minute : 0,
      timezone: obj.timezone || 'UTC',
      dayOfMonth: obj.day_of_month || 15,
      anchorDay: obj.anchor_day || 15,
      offsetDays: obj.offset_days ?? -3,
      ordinals: normalizeOrdinals(obj.ordinals, obj.ordinal),
      ordinalWeekday: obj.ordinal_weekday ?? 1,
    }
  } catch {
    return { ...DEFAULT_RECURRENCE }
  }
}

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
// Go weekday: 0=Sun, 1=Mon...6=Sat
// UI order (Mon-Sun) index 0 → Go weekday 1
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

  const toggleOrdinal = useCallback(
    (ordinal: number) => {
      const exists = state.ordinals.includes(ordinal)
      const next = exists
        ? state.ordinals.filter((value) => value !== ordinal)
        : [...state.ordinals, ordinal].sort((a, b) => {
            if (a === -1) return 1
            if (b === -1) return -1
            return a - b
          })
      set({ ordinals: next.length > 0 ? next : [ordinal] })
    },
    [state.ordinals, set],
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
              {t(key as any)}
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
          <div className="recurrence-form__weekdays">
            <span className="recurrence-form__label">{t('recurring.ordinals')}</span>
            <div className="recurrence-form__weekday-row">
              {ORDINAL_OPTIONS.map((ordinal) => {
                const active = state.ordinals.includes(ordinal)
                return (
                  <button
                    key={ordinal}
                    type="button"
                    className={`recurrence-form__weekday-btn${active ? ' recurrence-form__weekday-btn--active' : ''}`}
                    onClick={() => toggleOrdinal(ordinal)}
                    aria-pressed={active}
                    data-testid={`recurring-ordinal-${ordinal}`}
                  >
                    {t(ORDINAL_I18N[ordinal] as 'recurring.ordinal1')}
                  </button>
                )
              })}
            </div>
            <p className="hint recurrence-form__ordinal-hint">{t('recurring.ordinalsHint')}</p>
          </div>
          <div className="recurrence-form__weekdays">
            <span className="recurrence-form__label">{t('recurring.weekday')}</span>
            <div className="recurrence-form__weekday-row">
              {WEEKDAY_KEYS.map((key, i) => {
                const goWd = UI_TO_GO_WEEKDAY[i]
                const active = state.ordinalWeekday === goWd
                return (
                  <button
                    key={key}
                    type="button"
                    className={`recurrence-form__weekday-btn${active ? ' recurrence-form__weekday-btn--active' : ''}`}
                    onClick={() => set({ ordinalWeekday: goWd })}
                    aria-pressed={active}
                    data-testid={`recurring-ordinal-weekday-${key}`}
                  >
                    {t(`weekdays.${key}` as 'weekdays.mon').slice(0, 2)}
                  </button>
                )
              })}
            </div>
          </div>
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
