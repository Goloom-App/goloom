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
  }
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
