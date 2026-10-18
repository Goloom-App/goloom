import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { providerPostLength } from '../../postLength'
import type { AccountRecord } from '../../types'
import {
  applyComposerProposal,
  defaultProposalSelection,
  fullProposalSelection,
  proposalHasChanges,
  proposalLimitViolations,
  type ComposerProposal,
  type ProposalSelection,
} from './composerProposal'
import { bodyForAccountLimit } from './composerUtils'
import type { EditorDraftState } from './types'

function TextCompare({
  original,
  proposed,
  testId,
}: {
  original: string
  proposed: string | null
  testId?: string
}) {
  const { t } = useTranslation()
  return (
    <div className="composer-ai-proposal__compare" data-testid={testId}>
      <div className="composer-ai-proposal__text">
        <span className="hint">{t('composer.aiProposalOriginal')}</span>
        <p>{original}</p>
      </div>
      <div className="composer-ai-proposal__text composer-ai-proposal__text--proposed">
        <span className="hint">{t('composer.aiProposalProposed')}</span>
        {proposed === null ? (
          <p className="hint" data-testid="composer-ai-proposal-removal">
            {t('composer.aiProposalRemoval')}
          </p>
        ) : (
          <p>{proposed}</p>
        )}
      </div>
    </div>
  )
}

/** Side-by-side review of an AI optimization; nothing changes until the user applies. */
export function ComposerAIProposal({
  proposal,
  draft,
  teamAccounts,
  onApply,
  onDiscard,
}: {
  proposal: ComposerProposal
  draft: EditorDraftState
  teamAccounts: AccountRecord[]
  onApply: (selection: ProposalSelection) => void
  onDiscard: () => void
}) {
  const { t } = useTranslation()
  const [selection, setSelection] = useState<ProposalSelection>(() => defaultProposalSelection(proposal))

  const accountName = (id: string) => teamAccounts.find((a) => a.id === id)?.name ?? id
  const targets = proposal.base.targetAccountIds
  const changedIds = targets.filter((id) => Object.hasOwn(proposal.accountChanges, id))
  const unchangedIds = targets.filter((id) => !Object.hasOwn(proposal.accountChanges, id))

  if (!proposalHasChanges(proposal)) {
    return (
      <div className="composer-ai-proposal stack stack--sm" data-testid="composer-ai-proposal">
        <p className="hint" data-testid="composer-ai-proposal-empty" style={{ margin: 0 }}>
          {t('composer.aiProposalNoChanges')}
        </p>
        <div className="flex-row--center gap-2">
          <button type="button" className="btn btn--ghost btn--sm" data-testid="composer-ai-proposal-discard" onClick={onDiscard}>
            {t('composer.aiProposalDiscard')}
          </button>
        </div>
      </div>
    )
  }

  const violations = proposalLimitViolations(draft, proposal, selection, teamAccounts)
  const full = fullProposalSelection(proposal)
  const fullViolations = proposalLimitViolations(draft, proposal, full, teamAccounts)
  const nothingSelected = !(selection.shared && proposal.sharedChanged) && selection.accounts.length === 0
  const preview = applyComposerProposal(draft, proposal, selection)

  const toggleAccount = (id: string, checked: boolean) =>
    setSelection((current) => ({
      ...current,
      accounts: checked ? [...current.accounts.filter((x) => x !== id), id] : current.accounts.filter((x) => x !== id),
    }))

  return (
    <div className="composer-ai-proposal stack stack--sm" data-testid="composer-ai-proposal">
      <div>
        <strong>{t('composer.aiProposalTitle')}</strong>
        <p className="hint" style={{ margin: 0, fontSize: '0.8rem' }}>
          {t('composer.aiProposalDescription')}
        </p>
      </div>

      {proposal.sharedChanged ? (
        <div className="composer-ai-proposal__item">
          <label className="flex-row--center gap-2">
            <input
              type="checkbox"
              data-testid="composer-ai-proposal-shared"
              checked={selection.shared}
              onChange={(event) => setSelection((current) => ({ ...current, shared: event.target.checked }))}
            />
            <span>{t('composer.aiProposalShared')}</span>
          </label>
          <TextCompare
            original={proposal.base.content}
            proposed={proposal.sharedContent}
            testId="composer-ai-proposal-shared-diff"
          />
        </div>
      ) : null}

      {changedIds.map((id) => {
        const change = proposal.accountChanges[id]
        const existing = proposal.base.accountContentOverride[id]
        const hasOverride = (existing ?? '').trim() !== ''
        return (
          <div key={id} className="composer-ai-proposal__item" data-testid={`composer-ai-proposal-item-${id}`}>
            <label className="flex-row--center gap-2">
              <input
                type="checkbox"
                data-testid={`composer-ai-proposal-account-${id}`}
                checked={selection.accounts.includes(id)}
                onChange={(event) => toggleAccount(id, event.target.checked)}
              />
              <span>{t('composer.aiProposalAccount', { name: accountName(id) })}</span>
            </label>
            <TextCompare
              original={hasOverride ? existing : t('composer.aiProposalInheritsShared')}
              proposed={change.kind === 'set' ? change.text : null}
            />
          </div>
        )
      })}

      {unchangedIds.length > 0 ? (
        <p className="hint" data-testid="composer-ai-proposal-unchanged" style={{ margin: 0, fontSize: '0.8rem' }}>
          {t('composer.aiProposalUnchanged', { names: unchangedIds.map(accountName).join(', ') })}
        </p>
      ) : null}

      <ul className="composer-ai-proposal__counts">
        {preview.targetAccountIds.map((id) => {
          const account = teamAccounts.find((a) => a.id === id)
          if (!account) return null
          const length = providerPostLength(account.provider, bodyForAccountLimit(preview, id))
          const over = account.maxChars > 0 && length > account.maxChars
          return (
            <li key={id} className={over ? 'status-banner__error' : 'hint'}>
              {account.name}: {length} / {account.maxChars > 0 ? account.maxChars : t('common.emDash')}
            </li>
          )
        })}
      </ul>

      {violations.length > 0 ? (
        <div className="status-banner__error" role="alert" data-testid="composer-ai-proposal-limit-error">
          {t('composer.aiProposalLimitError')}
          <ul style={{ margin: 0 }}>
            {violations.map(({ account, length }) => (
              <li key={account.id}>
                {t('composer.aiProposalLimitAccount', { name: account.name, count: length, max: account.maxChars })}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="flex-row--center gap-2 flex-wrap">
        <button
          type="button"
          className="btn btn--primary btn--sm"
          data-testid="composer-ai-proposal-apply"
          disabled={nothingSelected || violations.length > 0}
          onClick={() => onApply(selection)}
        >
          {t('composer.aiProposalApplySelected')}
        </button>
        <button
          type="button"
          className="btn btn--secondary btn--sm"
          data-testid="composer-ai-proposal-apply-all"
          disabled={fullViolations.length > 0}
          onClick={() => onApply(full)}
        >
          {t('composer.aiProposalApplyAll')}
        </button>
        <button type="button" className="btn btn--ghost btn--sm" data-testid="composer-ai-proposal-discard" onClick={onDiscard}>
          {t('composer.aiProposalDiscard')}
        </button>
      </div>
    </div>
  )
}
