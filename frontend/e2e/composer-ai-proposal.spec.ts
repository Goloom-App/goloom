import { test, expect } from '@playwright/test'
import {
  applyComposerProposal,
  buildComposerProposal,
  defaultProposalSelection,
  fullProposalSelection,
  isProposalStale,
  proposalHasChanges,
  proposalLimitViolations,
} from '../src/components/Composer/composerProposal'
import type { EditorDraftState } from '../src/components/Composer/types'
import type { AccountRecord } from '../src/types'

// Pure-logic coverage for the Composer AI proposal (issue #164): building the
// comparison, selective application, explicit override removals, staleness and
// per-account limit checks of the final effective text.

function draft(overrides: Partial<EditorDraftState> = {}): EditorDraftState {
  return {
    title: 'Title',
    content: 'Original shared text.',
    scheduledAt: '',
    targetAccountIds: ['masto', 'bsky'],
    status: 'draft',
    accountContentOverride: {},
    mediaIds: [],
    mediaExcludeByAccount: {},
    ...overrides,
  }
}

function account(id: string, provider: 'mastodon' | 'bluesky', maxChars: number): AccountRecord {
  return {
    id,
    teamId: 'team-1',
    name: id,
    provider,
    instance: 'https://example.com',
    username: id,
    color: '#000',
    maxChars,
  }
}

const ACCOUNTS = [account('masto', 'mastodon', 500), account('bsky', 'bluesky', 30)]

function build(current: EditorDraftState, result: Record<string, unknown>) {
  return buildComposerProposal({ teamId: 'team-1', generation: 3, draft: current, result })
}

test.describe('composer AI proposal', () => {
  test('returns null for an empty proposed text', () => {
    expect(build(draft(), { content: '   ' })).toBeNull()
    expect(build(draft(), {})).toBeNull()
  })

  test('captures the shared change and only real account changes', () => {
    const current = draft({ accountContentOverride: { masto: 'User masto text.' } })
    const proposal = build(current, {
      content: 'Proposed shared text.',
      account_content_override: {
        masto: 'User masto text.', // identical → no change
        bsky: 'Short bsky.',
        foreign: 'Not a target', // not targeted → ignored
      },
    })!
    expect(proposal.sharedContent).toBe('Proposed shared text.')
    expect(proposal.sharedChanged).toBe(true)
    expect(proposal.accountChanges).toEqual({ bsky: { kind: 'set', text: 'Short bsky.' } })
    expect(proposalHasChanges(proposal)).toBe(true)
  })

  test('distinguishes an explicit override removal from an omitted override', () => {
    const current = draft({ accountContentOverride: { masto: 'User masto text.', bsky: 'User bsky text.' } })
    const proposal = build(current, {
      content: 'Original shared text.',
      account_content_override: { masto: null },
    })!
    expect(proposal.sharedChanged).toBe(false)
    expect(proposal.accountChanges).toEqual({ masto: { kind: 'remove' } })
    // bsky was omitted: it is unchanged, not a removal.
    expect(proposal.accountChanges.bsky).toBeUndefined()
  })

  test('an empty-string override counts as removal only when an override exists', () => {
    const proposal = build(draft({ accountContentOverride: { masto: 'User masto text.' } }), {
      content: 'Original shared text.',
      account_content_override: { masto: '', bsky: '  ' },
    })!
    expect(proposal.accountChanges).toEqual({ masto: { kind: 'remove' } })
  })

  test('a proposal without any change reports no changes', () => {
    const proposal = build(draft(), { content: 'Original shared text.' })!
    expect(proposalHasChanges(proposal)).toBe(false)
  })

  test('default selection keeps user-written overrides and full selection takes everything', () => {
    const current = draft({ accountContentOverride: { masto: 'User masto text.' } })
    const proposal = build(current, {
      content: 'Proposed shared text.',
      account_content_override: { masto: 'AI masto.', bsky: 'AI bsky.' },
    })!
    expect(defaultProposalSelection(proposal)).toEqual({ shared: true, accounts: ['bsky'] })
    expect(fullProposalSelection(proposal)).toEqual({ shared: true, accounts: ['masto', 'bsky'] })
  })

  test('applies only the selected parts and leaves everything else untouched', () => {
    const current = draft({ accountContentOverride: { masto: 'User masto text.' } })
    const proposal = build(current, {
      content: 'Proposed shared text.',
      account_content_override: { masto: 'AI masto.', bsky: 'AI bsky.' },
    })!

    const accountsOnly = applyComposerProposal(current, proposal, { shared: false, accounts: ['bsky'] })
    expect(accountsOnly.content).toBe('Original shared text.')
    expect(accountsOnly.accountContentOverride).toEqual({ masto: 'User masto text.', bsky: 'AI bsky.' })
    expect(accountsOnly.title).toBe('Title')

    const sharedOnly = applyComposerProposal(current, proposal, { shared: true, accounts: [] })
    expect(sharedOnly.content).toBe('Proposed shared text.')
    expect(sharedOnly.accountContentOverride).toEqual({ masto: 'User masto text.' })

    const nothing = applyComposerProposal(current, proposal, { shared: false, accounts: [] })
    expect(nothing).toEqual(current)
  })

  test('an accepted removal drops the override so the account inherits shared text', () => {
    const current = draft({ accountContentOverride: { masto: 'User masto text.' } })
    const proposal = build(current, {
      content: 'Proposed shared text.',
      account_content_override: { masto: null },
    })!
    const next = applyComposerProposal(current, proposal, fullProposalSelection(proposal))
    expect(Object.hasOwn(next.accountContentOverride, 'masto')).toBe(false)
    expect(next.content).toBe('Proposed shared text.')
  })

  test('is stale once team, draft generation, text, overrides or targets change', () => {
    const current = draft({ accountContentOverride: { masto: 'User masto text.' } })
    const proposal = build(current, { content: 'Proposed shared text.' })!
    const same = { teamId: 'team-1', generation: 3, draft: { ...current, title: 'Retitled' } }
    expect(isProposalStale(proposal, same)).toBe(false)
    expect(isProposalStale(proposal, { ...same, teamId: 'team-2' })).toBe(true)
    expect(isProposalStale(proposal, { ...same, generation: 4 })).toBe(true)
    expect(isProposalStale(proposal, { ...same, draft: { ...current, content: 'Edited.' } })).toBe(true)
    expect(
      isProposalStale(proposal, { ...same, draft: { ...current, accountContentOverride: { masto: 'Edited.' } } }),
    ).toBe(true)
    expect(isProposalStale(proposal, { ...same, draft: { ...current, targetAccountIds: ['masto'] } })).toBe(true)
  })

  test('checks the final effective text of each target against its limit', () => {
    const current = draft({ content: 'Short.' })
    const proposal = build(current, {
      content: 'A much longer shared text that exceeds Bluesky.',
      account_content_override: { bsky: 'Fits bsky.' },
    })!
    // Shared accepted without the bsky variant: bsky would inherit the long text.
    const invalid = proposalLimitViolations(current, proposal, { shared: true, accounts: [] }, ACCOUNTS)
    expect(invalid.map((v) => v.account.id)).toEqual(['bsky'])
    expect(invalid[0].length).toBeGreaterThan(30)
    // Taking the bsky variant too makes the combination valid.
    expect(proposalLimitViolations(current, proposal, fullProposalSelection(proposal), ACCOUNTS)).toEqual([])
  })
})
