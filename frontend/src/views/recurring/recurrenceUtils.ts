import type { TFunction } from 'i18next'

const GO_WEEKDAY_KEYS = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'] as const
const ORDINAL_I18N: Record<number, string> = {
  1: 'recurring.ordinal1',
  2: 'recurring.ordinal2',
  3: 'recurring.ordinal3',
  4: 'recurring.ordinal4',
  5: 'recurring.ordinal5',
  [-1]: 'recurring.ordinalLast',
}

export function ordinalWeekdayDay(year: number, month: number, ordinal: number, weekday: number): number | null {
  const maxd = new Date(year, month, 0).getDate()
  if (ordinal === -1) {
    let day = maxd
    while (day > 0) {
      if (new Date(year, month - 1, day).getDay() === weekday) {
        return day
      }
      day--
    }
    return null
  }
  const first = new Date(year, month - 1, 1).getDay()
  const offset = (weekday - first + 7) % 7
  const day = 1 + offset + (ordinal - 1) * 7
  if (day < 1 || day > maxd) {
    return null
  }
  return day
}

export function normalizeOrdinals(raw: unknown, legacyOrdinal?: unknown): number[] {
  if (Array.isArray(raw)) {
    const out: number[] = []
    for (const value of raw) {
      if (typeof value !== 'number' || value < -1 || value === 0 || value > 5) {
        continue
      }
      if (!out.includes(value)) {
        out.push(value)
      }
    }
    out.sort((a, b) => {
      if (a === -1) return 1
      if (b === -1) return -1
      return a - b
    })
    return out
  }
  if (typeof legacyOrdinal === 'number' && legacyOrdinal !== 0) {
    return [legacyOrdinal]
  }
  return [1]
}

function formatTime(hour: number, minute: number): string {
  return `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
}

function weekdayLabel(t: TFunction, goWeekday: number): string {
  const key = GO_WEEKDAY_KEYS[goWeekday] ?? 'mon'
  return t(`weekdays.${key}`)
}

function ordinalLabels(t: TFunction, ordinals: number[]): string {
  const labels = ordinals.map((ordinal) => t(ORDINAL_I18N[ordinal] as 'recurring.ordinal1'))
  if (labels.length <= 1) {
    return labels[0] ?? ''
  }
  const last = labels.pop()!
  return `${labels.join(', ')} ${t('recurring.and')} ${last}`
}

export function formatRecurrenceSummary(raw: string, t: TFunction): string {
  try {
    const obj = JSON.parse(raw) as Record<string, unknown>
    const time = formatTime(typeof obj.hour === 'number' ? obj.hour : 9, typeof obj.minute === 'number' ? obj.minute : 0)
    const tz = typeof obj.timezone === 'string' && obj.timezone.trim() ? obj.timezone : 'UTC'
    const kind = typeof obj.kind === 'string' ? obj.kind : 'weekly'

    if (kind === 'weekly') {
      const weekdays = Array.isArray(obj.weekdays) ? obj.weekdays.filter((value): value is number => typeof value === 'number') : [1]
      const days = weekdays.map((wd) => weekdayLabel(t, wd)).join(', ')
      return t('recurring.summaryWeekly', { days, time, tz })
    }

    if (kind === 'monthly_dom') {
      const day = typeof obj.day_of_month === 'number' ? obj.day_of_month : 1
      return t('recurring.summaryMonthlyDom', { day, time, tz })
    }

    if (kind === 'monthly_anchor_offset') {
      const anchorDay = typeof obj.anchor_day === 'number' ? obj.anchor_day : 1
      const offsetDays = typeof obj.offset_days === 'number' ? obj.offset_days : 0
      return t('recurring.summaryMonthlyAnchor', { anchorDay, offsetDays, time, tz })
    }

    if (kind === 'monthly_ordinal_weekday') {
      const ordinals = normalizeOrdinals(obj.ordinals, obj.ordinal)
      const weekday = weekdayLabel(t, typeof obj.ordinal_weekday === 'number' ? obj.ordinal_weekday : 1)
      return t('recurring.summaryMonthlyOrdinal', {
        ordinals: ordinalLabels(t, ordinals),
        weekday,
        time,
        tz,
      })
    }

    return raw.trim()
  } catch {
    return raw.trim()
  }
}
