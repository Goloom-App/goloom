import type { AccountRecord } from '../../types'
import { providerRequiresMedia } from '../../mappers'
import type { EditorDraftState } from './types'

/** Media this destination publishes after per-account exclusions (mirrors backend FilterMediaIDsForAccount). */
export function effectiveMediaForAccount(draft: EditorDraftState, accountId: string): string[] {
  const excluded = new Set(draft.mediaExcludeByAccount[accountId] ?? [])
  return draft.mediaIds.filter((id) => !excluded.has(id))
}

/** Target accounts whose provider requires media but that have no attachment (e.g. Pixelfed). */
export function accountsMissingRequiredMedia(
  draft: EditorDraftState,
  teamAccounts: AccountRecord[],
): AccountRecord[] {
  const out: AccountRecord[] = []
  for (const id of draft.targetAccountIds) {
    const acc = teamAccounts.find((a) => a.id === id)
    if (acc && providerRequiresMedia(acc.provider) && effectiveMediaForAccount(draft, id).length === 0) {
      out.push(acc)
    }
  }
  return out
}

/** Matches backend domain.CreatePostInput.EffectiveContent. */
export function hasAccountContentOverride(draft: EditorDraftState, accountId: string): boolean {
  if (!Object.hasOwn(draft.accountContentOverride, accountId)) {
    return false
  }
  return draft.accountContentOverride[accountId].trim() !== ''
}

/** Text used for per-account character limit checks and save validation. */
export function bodyForAccountLimit(draft: EditorDraftState, accountId: string): string {
  if (hasAccountContentOverride(draft, accountId)) {
    return draft.accountContentOverride[accountId]
  }
  return draft.content
}

export function isAccountOverCharLimit(draft: EditorDraftState, account: AccountRecord): boolean {
  if (account.maxChars <= 0) {
    return false
  }
  return bodyForAccountLimit(draft, account.id).length > account.maxChars
}

export function isAnyTargetAccountOverCharLimit(
  draft: EditorDraftState,
  teamAccounts: AccountRecord[],
): boolean {
  if (draft.targetAccountIds.length === 0) {
    return true
  }
  for (const id of draft.targetAccountIds) {
    const acc = teamAccounts.find((a) => a.id === id)
    if (acc && isAccountOverCharLimit(draft, acc)) {
      return true
    }
  }
  return false
}

/** Drop empty / whitespace-only overrides before API calls (backend normalizes similarly). */
export function accountContentOverrideForSave(draft: EditorDraftState): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [accountId, content] of Object.entries(draft.accountContentOverride)) {
    const trimmed = content.trim()
    if (trimmed !== '' && draft.targetAccountIds.includes(accountId)) {
      out[accountId] = content
    }
  }
  return out
}

export function effectiveBody(draft: EditorDraftState, accountId: string | null) {
  if (!accountId || accountId === 'default') {
    return draft.content
  }
  if (Object.hasOwn(draft.accountContentOverride, accountId)) {
    return draft.accountContentOverride[accountId]
  }
  return draft.content
}

export interface ComposerPreviewsProps {
  draft: EditorDraftState
  teamAccounts: import('../../types').AccountRecord[]
  teamId?: string | null
  api?: { mediaPreviewUrl: (teamId: string, mediaId: string) => string } | null
  authHeader?: string
  theme: 'dark' | 'light'
  libraryItems: import('../../api').BackendMediaItem[]
}
