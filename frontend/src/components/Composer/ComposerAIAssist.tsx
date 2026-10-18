import { useEffect, useRef, useState } from 'react'
import type { Dispatch, RefObject, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, MessageSquareText, Sparkles } from 'lucide-react'

import { getApiClient, useCancelAIJob, useTriggerAIJob } from '../../hooks/useAI'
import { useAIJobStream } from '../../hooks/useSSE'
import { openAIChatWithComposerContext } from '../ai/composerChatBridge'
import { ComposerAIProposal } from './ComposerAIProposal'
import {
  applyComposerProposal,
  buildComposerProposal,
  isDraftIdentityStale,
  isProposalStale,
  type ComposerProposal,
  type ProposalSelection,
} from './composerProposal'
import { effectiveBody } from './composerUtils'
import type { AccountRecord } from '../../types'
import type { EditorDraftState } from './types'

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

class OptimizeAborted extends Error {}

async function waitForAIJob(teamId: string, jobId: string, isAborted: () => boolean) {
  const api = getApiClient()
  for (let attempt = 0; attempt < 90; attempt += 1) {
    if (isAborted()) {
      throw new OptimizeAborted()
    }
    const job = await api.getAIJob(teamId, jobId)
    if (isAborted()) {
      throw new OptimizeAborted()
    }
    if (job.status === 'completed') {
      return job
    }
    if (job.status === 'failed') {
      throw new Error(job.error_message || 'AI optimization failed')
    }
    await sleep(1000)
  }
  throw new Error('AI optimization timed out')
}

export function ComposerAIAssist({
  teamId,
  isAiEnabled,
  draft,
  setDraft,
  activeTab,
  teamAccounts,
  draftGenerationRef,
  onDiscardedResult,
}: {
  teamId: string
  isAiEnabled: boolean
  draft: EditorDraftState
  setDraft: Dispatch<SetStateAction<EditorDraftState>>
  activeTab: string
  teamAccounts: AccountRecord[]
  /** Bumped by App whenever a different draft or team is loaded into the composer. */
  draftGenerationRef: RefObject<number>
  /** Called when a finished AI job is dropped because the draft moved on. */
  onDiscardedResult?: () => void
}) {
  const { t } = useTranslation()
  const triggerJob = useTriggerAIJob()
  const cancelJob = useCancelAIJob()
  useAIJobStream(teamId)

  const [instruction, setInstruction] = useState('')
  const [showInstruction, setShowInstruction] = useState(false)
  const [optimizing, setOptimizing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [proposal, setProposal] = useState<ComposerProposal | null>(null)
  // Identifies the running request: cancel, unmount or a newer run bump it so
  // a late result of an abandoned request is ignored.
  const requestIdRef = useRef(0)
  const runningJobRef = useRef<{ teamId: string; jobId: string } | null>(null)

  // Async job results must be compared against the draft as it is now, not the
  // render that started the request.
  const latestDraftRef = useRef(draft)
  const latestTeamIdRef = useRef(teamId)
  useEffect(() => {
    latestDraftRef.current = draft
    latestTeamIdRef.current = teamId
  }, [draft, teamId])

  // Results arriving after the composer closed have nowhere to land.
  const mountedRef = useRef(true)
  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      requestIdRef.current += 1
    }
  }, [])

  // An open proposal belongs to the draft it was generated for: new edits,
  // another draft or a team change drop it instead of letting it overwrite work.
  useEffect(() => {
    if (proposal && isProposalStale(proposal, { teamId, generation: draftGenerationRef.current, draft })) {
      setProposal(null)
      onDiscardedResult?.()
    }
  }, [draft, teamId, proposal, draftGenerationRef, onDiscardedResult])

  if (!isAiEnabled || activeTab !== 'default') {
    return null
  }

  const canOptimize = draft.content.trim().length > 0 && draft.targetAccountIds.length > 0
  // Busy covers the whole round trip: trigger request plus waiting for the
  // job result — not just the initial POST.
  const busy = triggerJob.isPending || optimizing

  const handleOptimize = async () => {
    if (!canOptimize || busy) {
      return
    }
    setError(null)
    setStatusMessage(null)
    setProposal(null)
    setOptimizing(true)
    const requestId = ++requestIdRef.current
    const isAborted = () => requestIdRef.current !== requestId
    const startTeamId = teamId
    const startGeneration = draftGenerationRef.current
    const startDraft = draft
    try {
      const response = await triggerJob.mutateAsync({
        teamId,
        type: 'voice_engine',
        params: {
          refine_content: true,
          no_publish: true,
          source_content: draft.content.trim(),
          prompt_hint:
            instruction.trim() ||
            'Improve clarity, flow, and engagement while preserving the core message and team voice.',
          target_account_ids: draft.targetAccountIds,
        },
      })
      if (!response.jobId?.trim()) {
        throw new Error('AI job id missing from trigger response')
      }
      if (isAborted()) {
        throw new OptimizeAborted()
      }
      runningJobRef.current = { teamId, jobId: response.jobId }
      const job = await waitForAIJob(teamId, response.jobId, isAborted)
      runningJobRef.current = null
      const built = buildComposerProposal({
        teamId: startTeamId,
        generation: startGeneration,
        draft: startDraft,
        result: job.result,
      })
      // The user may have opened another draft, switched teams or edited the
      // text while the job ran — a result for the old draft must not land.
      if (
        isDraftIdentityStale(
          { teamId: startTeamId, generation: startGeneration, draft: startDraft },
          { teamId: latestTeamIdRef.current, generation: draftGenerationRef.current, draft: latestDraftRef.current },
        )
      ) {
        onDiscardedResult?.()
        return
      }
      if (!built) {
        throw new Error(t('composer.aiOptimizeEmpty'))
      }
      setProposal(built)
      setShowInstruction(false)
    } catch (err) {
      if (err instanceof OptimizeAborted || isAborted()) {
        return
      }
      setError(err instanceof Error ? err.message : t('composer.aiOptimizeFailed'))
    } finally {
      if (!isAborted()) {
        runningJobRef.current = null
        setOptimizing(false)
      }
    }
  }

  const handleCancel = () => {
    requestIdRef.current += 1
    const running = runningJobRef.current
    runningJobRef.current = null
    setOptimizing(false)
    setStatusMessage(t('composer.aiOptimizeCancelled'))
    if (running) {
      // Best effort: the result is ignored either way.
      cancelJob.mutate(running, { onError: () => undefined })
    }
  }

  const handleApply = (selection: ProposalSelection) => {
    if (!proposal) {
      return
    }
    if (isProposalStale(proposal, { teamId, generation: draftGenerationRef.current, draft: latestDraftRef.current })) {
      setProposal(null)
      onDiscardedResult?.()
      return
    }
    setDraft((current) => applyComposerProposal(current, proposal, selection))
    setProposal(null)
    setStatusMessage(t('composer.aiOptimizeApplied'))
  }

  return (
    <div className="stack stack--sm" data-testid="composer-ai-assist">
      <div className="flex-row--center gap-2 flex-wrap">
        <button
          type="button"
          className="btn btn--secondary btn--sm"
          data-testid="composer-ai-optimize"
          disabled={!canOptimize || busy || proposal !== null}
          onClick={() => {
            if (showInstruction) {
              void handleOptimize()
            } else {
              setShowInstruction(true)
            }
          }}
        >
          {busy ? (
            <>
              <Loader2 size={14} className="spin" /> {t('composer.aiOptimizing')}
            </>
          ) : (
            <>
              <Sparkles size={14} /> {t('composer.aiOptimize')}
            </>
          )}
        </button>
        {busy ? (
          <button type="button" className="btn btn--ghost btn--sm" data-testid="composer-ai-cancel" onClick={handleCancel}>
            {t('common.cancel')}
          </button>
        ) : null}
        {showInstruction && !busy ? (
          <button type="button" className="btn btn--ghost btn--sm" onClick={() => setShowInstruction(false)}>
            {t('common.cancel')}
          </button>
        ) : null}
        <button
          type="button"
          className="btn btn--ghost btn--sm"
          data-testid="composer-ai-chat"
          disabled={draft.content.trim().length === 0}
          onClick={() =>
            openAIChatWithComposerContext({
              content: draft.content,
              title: draft.title,
              targets: teamAccounts
                .filter((account) => draft.targetAccountIds.includes(account.id))
                .map((account) => ({
                  accountId: account.id,
                  name: account.name,
                  provider: account.provider,
                  maxChars: account.maxChars,
                  text: effectiveBody(draft, account.id),
                  hasOverride: Object.hasOwn(draft.accountContentOverride, account.id),
                })),
            })
          }
        >
          <MessageSquareText size={14} /> {t('composer.aiOpenChat')}
        </button>
      </div>
      {showInstruction ? (
        <label className="field">
          <span>{t('composer.aiOptimizeHint')}</span>
          <textarea
            rows={2}
            data-testid="composer-ai-instruction"
            value={instruction}
            onChange={(event) => setInstruction(event.target.value)}
            placeholder={t('composer.aiOptimizePlaceholder')}
          />
        </label>
      ) : (
        <p className="hint" style={{ margin: 0, fontSize: '0.8rem' }}>
          {t('composer.aiOptimizeDescription')}
        </p>
      )}
      {proposal ? (
        <ComposerAIProposal
          key={`${proposal.teamId}:${proposal.generation}:${proposal.sharedContent}`}
          proposal={proposal}
          draft={draft}
          teamAccounts={teamAccounts}
          onApply={handleApply}
          onDiscard={() => {
            setProposal(null)
            setStatusMessage(t('composer.aiProposalDiscarded'))
          }}
        />
      ) : null}
      {statusMessage ? <span className="status-banner__success" style={{ fontSize: '0.8rem' }}>{statusMessage}</span> : null}
      {error ? (
        <span className="status-banner__error" role="alert" data-testid="composer-ai-error" style={{ fontSize: '0.8rem' }}>
          {error}
        </span>
      ) : null}
    </div>
  )
}
