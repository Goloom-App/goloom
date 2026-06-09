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
