import { useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, MessageSquareText, Sparkles } from 'lucide-react'

import { getApiClient, useTriggerAIJob } from '../../hooks/useAI'
import { useAIJobStream } from '../../hooks/useSSE'
import { openAIChatWithComposerContext } from '../ai/composerChatBridge'
import { effectiveBody } from './composerUtils'
import type { AccountRecord } from '../../types'
import type { EditorDraftState } from './types'

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

async function waitForAIJob(teamId: string, jobId: string) {
  const api = getApiClient()
  for (let attempt = 0; attempt < 90; attempt += 1) {
    const job = await api.getAIJob(teamId, jobId)
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
}: {
  teamId: string
  isAiEnabled: boolean
  draft: EditorDraftState
  setDraft: Dispatch<SetStateAction<EditorDraftState>>
  activeTab: string
  teamAccounts: AccountRecord[]
}) {
  const { t } = useTranslation()
  const triggerJob = useTriggerAIJob()
  useAIJobStream(teamId)

  const [instruction, setInstruction] = useState('')
  const [showInstruction, setShowInstruction] = useState(false)
  const [optimizing, setOptimizing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)

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
    setOptimizing(true)
    try {
      const response = await triggerJob.mutateAsync({
        teamId,
        type: 'voice_engine',
        params: {
          refine_content: true,
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
      const job = await waitForAIJob(teamId, response.jobId)
      const content = typeof job.result?.content === 'string' ? job.result.content : ''
      if (!content.trim()) {
        throw new Error('AI returned empty content')
      }
      const overrides =
        job.result?.account_content_override && typeof job.result.account_content_override === 'object'
          ? (job.result.account_content_override as Record<string, string>)
          : {}
      setDraft((current) => ({
        ...current,
        content,
        accountContentOverride: overrides,
      }))
      setStatusMessage(t('composer.aiOptimizeApplied'))
      setShowInstruction(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('composer.aiOptimizeFailed'))
    } finally {
      setOptimizing(false)
    }
  }

  return (
    <div className="stack stack--sm" data-testid="composer-ai-assist">
      <div className="flex-row--center gap-2 flex-wrap">
        <button
          type="button"
          className="btn btn--secondary btn--sm"
          data-testid="composer-ai-optimize"
          disabled={!canOptimize || busy}
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
      {statusMessage ? <span className="status-banner__success" style={{ fontSize: '0.8rem' }}>{statusMessage}</span> : null}
      {error ? <span className="status-banner__error" style={{ fontSize: '0.8rem' }}>{error}</span> : null}
    </div>
  )
}
