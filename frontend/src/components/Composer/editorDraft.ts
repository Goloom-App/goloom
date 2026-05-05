import { addMinutes, format, set } from 'date-fns'
import type { AccountRecord } from '../../types'
import { SLOT_MINUTES } from '../../schedule'
import type { EditorDraftState } from './types'

export function roundToNextSlot(date: Date) {
  const minutes = date.getMinutes()
  const remainder = minutes % SLOT_MINUTES
  if (remainder === 0) {
    return set(date, { seconds: 0, milliseconds: 0 })
  }
  return set(addMinutes(date, SLOT_MINUTES - remainder), { seconds: 0, milliseconds: 0 })
}

export function toInputDateTime(date: Date) {
  return format(date, "yyyy-MM-dd'T'HH:mm")
}

export function defaultEditorDraft(date: Date, teamAccounts: AccountRecord[]): EditorDraftState {
  const roundedDate = roundToNextSlot(date)
  return {
    title: '',
    content: '',
    scheduledAt: toInputDateTime(roundedDate),
    targetAccountIds: teamAccounts[0] ? [teamAccounts[0].id] : [],
    status: 'scheduled',
    accountContentOverride: {},
    mediaIds: [],
    mediaExcludeByAccount: {},
  }
}

/** After removing `mediaId` from the post, drop it from all per-target exclusion lists */
export function buildMediaExcludePayload(
  ex: Record<string, string[]>,
  targetAccountIds: string[],
  mediaIds: string[],
): Record<string, string[]> | undefined {
  const mset = new Set(mediaIds)
  const out: Record<string, string[]> = {}
  for (const acc of targetAccountIds) {
    const xs = (ex[acc] ?? []).filter((id) => mset.has(id))
    if (xs.length > 0) {
      out[acc] = xs
    }
  }
  return Object.keys(out).length > 0 ? out : undefined
}

export function pruneMediaExcludeAfterRemove(ex: Record<string, string[]> | undefined, mediaId: string): Record<string, string[]> {
  if (!ex || Object.keys(ex).length === 0) {
    return {}
  }
  const next: Record<string, string[]> = {}
  for (const [k, xs] of Object.entries(ex)) {
    const kept = xs.filter((id) => id !== mediaId)
    if (kept.length > 0) {
      next[k] = kept
    }
  }
  return next
}

export function charCounterClass(length: number, maxChars: number) {
  if (maxChars === 0) {
    return 'char-counter--idle'
  }
  const usage = length / maxChars
  if (usage >= 1) {
    return 'char-counter--danger'
  }
  if (usage >= 0.85) {
    return 'char-counter--warning'
  }
  return 'char-counter--good'
}
