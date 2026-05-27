import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

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
  ordinal: number
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
  ordinal: 1,
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
    obj.ordinal = state.ordinal
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
      ordinal: obj.ordinal ?? 1,
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
          <label className="recurrence-form__label">{t('recurring.dayOfMonth')}</label>
          <input
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
        <div className="recurrence-form__row">
          <div className="recurrence-form__field">
            <label className="recurrence-form__label">{t('recurring.ordinal')}</label>
            <select className="recurrence-form__input" value={state.ordinal} onChange={(e) => set({ ordinal: Number(e.target.value) })}>
              <option value={1}>{t('recurring.ordinal1')}</option>
              <option value={2}>{t('recurring.ordinal2')}</option>
              <option value={3}>{t('recurring.ordinal3')}</option>
              <option value={4}>{t('recurring.ordinal4')}</option>
              <option value={5}>{t('recurring.ordinal5')}</option>
              <option value={-1}>{t('recurring.ordinalLast')}</option>
            </select>
          </div>
          <div className="recurrence-form__field">
            <label className="recurrence-form__label">{t('recurring.weekday')}</label>
            <select className="recurrence-form__input" value={state.ordinalWeekday} onChange={(e) => set({ ordinalWeekday: Number(e.target.value) })}>
              {WEEKDAY_KEYS.map((key, i) => {
                const goWd = UI_TO_GO_WEEKDAY[i]
                return (
                  <option key={key} value={goWd}>{t(`weekdays.${key}` as 'weekdays.mon')}</option>
                )
              })}
            </select>
          </div>
        </div>
      )}

      <div className="recurrence-form__row">
        <div className="recurrence-form__field">
          <label className="recurrence-form__label">{t('recurring.hour')}</label>
          <input
            type="number"
            className="recurrence-form__input recurrence-form__input--narrow"
            min={0}
            max={23}
            value={state.hour}
            onChange={(e) => set({ hour: Math.min(23, Math.max(0, Number(e.target.value) || 0)) })}
          />
        </div>
        <div className="recurrence-form__field">
          <label className="recurrence-form__label">{t('recurring.minute')}</label>
          <input
            type="number"
            className="recurrence-form__input recurrence-form__input--narrow"
            min={0}
            max={59}
            value={state.minute}
            onChange={(e) => set({ minute: Math.min(59, Math.max(0, Number(e.target.value) || 0)) })}
          />
        </div>
        <div className="recurrence-form__field recurrence-form__field--grow">
          <label className="recurrence-form__label">{t('recurring.timezone')}</label>
          <input
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
