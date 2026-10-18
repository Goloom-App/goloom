import type { AccountRecord } from '../../types'
import { providerPostLength } from '../../postLength'
import { bodyForAccountLimit } from './composerUtils'
import type { EditorDraftState } from './types'

/**
 * A proposed change to one account's specific text. `remove` is an explicit
 * proposal to drop the override so the account inherits the shared text; an
 * account the AI omitted has no entry at all and stays unchanged.
 */
export type AccountProposalChange = { kind: 'set'; text: string } | { kind: 'remove' }

/** AI optimization result held for review, bound to the draft state it was made for. */
export type ComposerProposal = {
  teamId: string
  /** Composer draft generation (bumped by App whenever another draft or team loads). */
  generation: number
  /** The draft fields the proposal compares against and may change. */
  base: Pick<EditorDraftState, 'content' | 'accountContentOverride' | 'targetAccountIds'>
  sharedContent: string
  sharedChanged: boolean
  /** Only real changes for targeted accounts, keyed by account ID. */
  accountChanges: Record<string, AccountProposalChange>
}

export type ProposalSelection = { shared: boolean; accounts: string[] }

export type ProposalIdentity = { teamId: string; generation: number; draft: EditorDraftState }

export type ProposalLimitViolation = { account: AccountRecord; length: number }

function hasUserOverride(overrides: Record<string, string>, accountId: string): boolean {
  return (overrides[accountId] ?? '').trim() !== ''
}

/**
 * Builds a reviewable proposal from an AI job result, or null when the result
 * carries no usable shared text. In `account_content_override`, a non-empty
 * string proposes new account text, while `null` or an empty string proposes
 * removing an existing override.
 */
export function buildComposerProposal({
  teamId,
  generation,
  draft,
  result,
}: ProposalIdentity & { result: Record<string, unknown> | null | undefined }): ComposerProposal | null {
  const content = typeof result?.content === 'string' ? result.content : ''
  if (!content.trim()) {
    return null
  }
  const rawOverrides = result?.account_content_override
  const accountChanges: Record<string, AccountProposalChange> = {}
  if (rawOverrides && typeof rawOverrides === 'object' && !Array.isArray(rawOverrides)) {
    for (const [accountId, value] of Object.entries(rawOverrides as Record<string, unknown>)) {
      if (!draft.targetAccountIds.includes(accountId)) {
        continue
      }
      const existing = draft.accountContentOverride[accountId] ?? ''
      if (value === null || (typeof value === 'string' && value.trim() === '')) {
        if (hasUserOverride(draft.accountContentOverride, accountId)) {
          accountChanges[accountId] = { kind: 'remove' }
        }
      } else if (typeof value === 'string' && value !== existing) {
        accountChanges[accountId] = { kind: 'set', text: value }
      }
    }
  }
  return {
    teamId,
    generation,
    base: {
      content: draft.content,
      accountContentOverride: { ...draft.accountContentOverride },
      targetAccountIds: [...draft.targetAccountIds],
    },
    sharedContent: content,
    sharedChanged: content.trim() !== draft.content.trim(),
    accountChanges,
  }
}

export function proposalHasChanges(proposal: ComposerProposal): boolean {
  return proposal.sharedChanged || Object.keys(proposal.accountChanges).length > 0
}

/**
 * Preselects the shared change and AI variants for accounts without user-written
 * text, so user overrides survive unless the user explicitly opts in.
 */
export function defaultProposalSelection(proposal: ComposerProposal): ProposalSelection {
  return {
    shared: proposal.sharedChanged,
    accounts: Object.entries(proposal.accountChanges)
      .filter(([id, change]) => change.kind === 'set' && !hasUserOverride(proposal.base.accountContentOverride, id))
      .map(([id]) => id),
  }
}

export function fullProposalSelection(proposal: ComposerProposal): ProposalSelection {
  return { shared: proposal.sharedChanged, accounts: Object.keys(proposal.accountChanges) }
}

function sameRecord(a: Record<string, string>, b: Record<string, string>): boolean {
  const keys = Object.keys(a)
  return keys.length === Object.keys(b).length && keys.every((key) => Object.hasOwn(b, key) && a[key] === b[key])
}

function sameList(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((value, index) => value === b[index])
}

function sameBase(a: ComposerProposal['base'], b: EditorDraftState): boolean {
  return (
    a.content === b.content &&
    sameRecord(a.accountContentOverride, b.accountContentOverride) &&
    sameList(a.targetAccountIds, b.targetAccountIds)
  )
}

/** True once team, loaded draft, shared text, overrides or targets moved on since `start`. */
export function isDraftIdentityStale(start: ProposalIdentity, current: ProposalIdentity): boolean {
  return start.teamId !== current.teamId || start.generation !== current.generation || !sameBase(start.draft, current.draft)
}

/** A proposal may only land on the team, draft and draft text it was generated for. */
export function isProposalStale(proposal: ComposerProposal, current: ProposalIdentity): boolean {
  return (
    proposal.teamId !== current.teamId ||
    proposal.generation !== current.generation ||
    !sameBase(proposal.base, current.draft)
  )
}

/** Applies only the selected parts; unselected text and overrides stay as they are. */
export function applyComposerProposal(
  draft: EditorDraftState,
  proposal: ComposerProposal,
  selection: ProposalSelection,
): EditorDraftState {
  const selectedAccounts = selection.accounts.filter((id) => Object.hasOwn(proposal.accountChanges, id))
  const applyShared = selection.shared && proposal.sharedChanged
  if (!applyShared && selectedAccounts.length === 0) {
    return draft
  }
  const overrides = { ...draft.accountContentOverride }
  for (const id of selectedAccounts) {
    const change = proposal.accountChanges[id]
    if (change.kind === 'set') {
      overrides[id] = change.text
    } else {
      delete overrides[id]
    }
  }
  return {
    ...draft,
    content: applyShared ? proposal.sharedContent : draft.content,
    accountContentOverride: overrides,
  }
}

/** Target accounts whose final effective text would exceed their limit with this selection. */
export function proposalLimitViolations(
  draft: EditorDraftState,
  proposal: ComposerProposal,
  selection: ProposalSelection,
  teamAccounts: AccountRecord[],
): ProposalLimitViolation[] {
  const next = applyComposerProposal(draft, proposal, selection)
  const out: ProposalLimitViolation[] = []
  for (const id of next.targetAccountIds) {
    const account = teamAccounts.find((a) => a.id === id)
    if (!account || account.maxChars <= 0) {
      continue
    }
    const length = providerPostLength(account.provider, bodyForAccountLimit(next, id))
    if (length > account.maxChars) {
      out.push({ account, length })
    }
  }
  return out
}
