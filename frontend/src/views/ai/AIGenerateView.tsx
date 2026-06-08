import { useMemo, useState } from 'react'
import { CalendarClock, Check, Loader2, Play, Save, X } from 'lucide-react'

import {
  useTriggerAIJob,
  useAIJobs,
  useCampaignFormats,
  useTeamProfile,
  useCancelAIJob,
} from '../../hooks/useAI'
import { useAIJobStream } from '../../hooks/useSSE'
import { DestinationPicker } from '../../components/ai/DestinationPicker'
import type { TeamRecord, AccountRecord, AIJob } from '../../types'
import { createApiClient } from '../../api'

interface AIGenerateViewProps {
  team: TeamRecord
  accounts: AccountRecord[]
}

export function AIGenerateView({ team, accounts }: AIGenerateViewProps) {
  const { data: profile } = useTeamProfile(team.id)
  const { data: formats } = useCampaignFormats(team.id)
  const { data: jobs } = useAIJobs(team.id)
  const triggerJob = useTriggerAIJob()
  const cancelJob = useCancelAIJob()

  useAIJobStream(team.id)

  const [prompt, setPrompt] = useState('')
  const [campaignFormatId, setCampaignFormatId] = useState('')
  const [selectedAccounts, setSelectedAccounts] = useState<string[]>([])

  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [savingDraftId, setSavingDraftId] = useState<string | null>(null)
  const [schedulingId, setSchedulingId] = useState<string | null>(null)

  const activeJobs = jobs?.filter((j) => j.status === 'pending' || j.status === 'processing') || []
  const recentJobs = jobs?.filter((j) => j.type === 'voice_engine' && (j.status === 'completed' || j.status === 'failed')).slice(0, 10) || []

  const latestCompleted = useMemo(
    () => recentJobs.find((job) => job.status === 'completed' && typeof job.result?.content === 'string'),
    [recentJobs],
  )

  if (!team.isAiEnabled) {
    return (
      <div className="empty-state">
        <p className="hint">AI features are not enabled for this team.</p>
      </div>
    )
  }

  const toggleAccount = (accountId: string) => {
    setSelectedAccounts((prev) =>
      prev.includes(accountId) ? prev.filter((id) => id !== accountId) : [...prev, accountId],
    )
  }

  const handleTrigger = async () => {
    setError(null)
    setStatusMessage(null)

    if (!prompt.trim()) {
      setError('Prompt is required')
      return
    }
    if (selectedAccounts.length === 0) {
      setError('Select at least one target account')
      return
    }

    try {
      await triggerJob.mutateAsync({
        teamId: team.id,
        type: 'voice_engine',
        params: {
          prompt_hint: prompt.trim(),
          target_account_ids: selectedAccounts,
          ...(campaignFormatId ? { campaign_format_id: campaignFormatId } : {}),
        },
      })
      setStatusMessage('Generation started')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to trigger AI job')
    }
  }

  const handleCancelJob = async (job: AIJob) => {
    setError(null)
    setStatusMessage(null)
    try {
      await cancelJob.mutateAsync({ teamId: team.id, jobId: job.id })
      setStatusMessage('AI job cancelled')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to cancel AI job')
    }
  }

  const getApi = () => {
    const raw = window.localStorage.getItem('goloom-ui-settings')
    let token = ''
    let baseUrl = window.location.origin
    if (raw) {
      const parsed = JSON.parse(raw)
      token = parsed.general?.bearerToken || ''
      baseUrl = parsed.general?.apiBaseUrl || baseUrl
    }
    return createApiClient({ baseUrl, token })
  }

  const saveGeneratedPost = async (job: AIJob, schedule: boolean) => {
    if (!job.result?.content) return

    if (schedule) {
      setSchedulingId(job.id)
    } else {
      setSavingDraftId(job.id)
    }
    setError(null)

    try {
      const api = getApi()
      const scheduledAt =
        typeof job.result.scheduled_at === 'string' && job.result.scheduled_at
          ? job.result.scheduled_at
          : undefined
      const overrides =
        job.result.account_content_override && typeof job.result.account_content_override === 'object'
          ? (job.result.account_content_override as Record<string, string>)
          : undefined

      await api.createAIDraft(team.id, {
        content: String(job.result.content),
        account_ids: Array.isArray(job.payload?.target_account_ids)
          ? (job.payload.target_account_ids as string[])
          : selectedAccounts,
        account_content_override: overrides,
        scheduled_at: scheduledAt,
        schedule,
        ai_job_id: job.id,
      })

      setStatusMessage(schedule ? 'Post scheduled' : 'Draft saved')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save post')
    } finally {
      setSavingDraftId(null)
      setSchedulingId(null)
    }
  }

  return (
    <div className="two-column-detail" data-testid="ai-generate-view">
      <div className="glass-panel stack">
        <h2 className="section-card__title">Generate Post</h2>
        <p className="hint">
          Describe what you want to publish. The AI writes one full-length post for the account with the highest
          character limit and only adds shorter overrides where a target account needs them.
        </p>

        {(error || statusMessage) && (
          <div className="status-banner-panel" style={{ padding: '1rem', marginBottom: '1rem' }}>
            {statusMessage && <span className="status-banner__success" data-testid="gen-status-success">{statusMessage}</span>}
            {error && <span className="status-banner__error" data-testid="gen-status-error">{error}</span>}
          </div>
        )}

        <label className="field">
          <span>Prompt</span>
          <textarea
            data-testid="gen-prompt"
            rows={6}
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="What should the post be about?"
          />
        </label>

        <label className="field">
          <span>Campaign (optional)</span>
          <select
            data-testid="gen-campaign"
            value={campaignFormatId}
            onChange={(e) => setCampaignFormatId(e.target.value)}
          >
            <option value="">No campaign</option>
            {(formats ?? []).filter((f) => f.isActive).map((format) => (
              <option key={format.id} value={format.id}>
                {format.name}
              </option>
            ))}
          </select>
        </label>

        <div className="field">
          <span>Target accounts</span>
          <DestinationPicker
            accounts={accounts}
            selectedIds={selectedAccounts}
            onToggle={toggleAccount}
            testIdPrefix="gen-dest"
          />
        </div>

        <div className="mt-4">
          <button
            data-testid="gen-submit"
            className="btn btn--primary"
            onClick={() => void handleTrigger()}
            disabled={triggerJob.isPending}
          >
            {triggerJob.isPending ? (
              <>
                <Loader2 size={16} className="spin" /> Generating...
              </>
            ) : (
              <>
                <Play size={16} /> Generate
              </>
            )}
          </button>
        </div>
      </div>

      <div className="stack">
        {activeJobs.length > 0 && (
          <div className="glass-panel stack" data-testid="gen-active-jobs">
            <h3 className="subsection-title">Active Jobs</h3>
            <div className="stack stack--sm">
              {activeJobs.map((job) => (
                <div key={job.id} className="glass-panel glass-panel--compact flex-row--between" style={{ alignItems: 'center' }}>
                  <div>
                    <strong>Voice Engine</strong>
                    <p className="hint" style={{ fontSize: '0.8rem', margin: 0 }}>
                      {new Date(job.createdAt).toLocaleString()}
                    </p>
                  </div>
                  <div className="flex-row--center gap-2">
                    <Loader2 size={16} className="spin" style={{ color: 'var(--primary)' }} />
                    <span className="badge badge--primary" style={{ textTransform: 'capitalize' }}>
                      {job.status}
                    </span>
                    <button
                      type="button"
                      className="btn btn--secondary btn--sm"
                      data-testid={`gen-cancel-job-${job.id}`}
                      onClick={() => void handleCancelJob(job)}
                      disabled={cancelJob.isPending}
                      title="Cancel job"
                    >
                      <X size={14} />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {(latestCompleted || recentJobs.length > 0) && (
          <div className="glass-panel stack" data-testid="gen-recent-jobs">
            <h3 className="subsection-title">Generated content</h3>
            {(latestCompleted ? [latestCompleted] : recentJobs).map((job) => (
              <div key={job.id} className="glass-panel glass-panel--compact stack stack--sm">
                <div className="flex-row--between" style={{ alignItems: 'center' }}>
                  <div>
                    <p className="hint" style={{ fontSize: '0.8rem', margin: 0 }}>
                      {new Date(job.createdAt).toLocaleString()}
                    </p>
                    {typeof job.result?.scheduled_at === 'string' && job.result.scheduled_at && (
                      <p className="hint" style={{ fontSize: '0.8rem', margin: 0 }}>
                        Suggested slot: {new Date(job.result.scheduled_at).toLocaleString()}
                      </p>
                    )}
                  </div>
                  <span className={`badge ${job.status === 'completed' ? 'badge--success' : 'badge--danger'}`}>
                    {job.status}
                  </span>
                </div>

                {job.status === 'failed' && job.errorMessage && (
                  <div className="status-banner__error" style={{ fontSize: '0.8rem', padding: '0.5rem' }}>
                    {job.errorMessage}
                  </div>
                )}

                {job.status === 'completed' && typeof job.result?.content === 'string' && job.result.content && (
                  <>
                    {(() => {
                      const primaryAccountId =
                        typeof job.result.primary_account_id === 'string'
                          ? job.result.primary_account_id
                          : null
                      const primaryAccount = primaryAccountId
                        ? accounts.find((item) => item.id === primaryAccountId)
                        : undefined
                      const primaryLabel = primaryAccount
                        ? `${primaryAccount.username || primaryAccount.name} · ${primaryAccount.provider}`
                        : 'Primary account'
                      const primaryLimit = primaryAccount?.maxChars
                      return (
                        <div className="stack stack--xs">
                          <p className="hint" style={{ margin: 0 }}>
                            Primary text
                            {primaryLimit ? ` (${String(job.result.content).length}/${primaryLimit} chars)` : ''}
                            {' · '}
                            {primaryLabel}
                          </p>
                          <div
                            style={{
                              background: 'var(--bg-secondary)',
                              padding: '0.75rem',
                              borderRadius: '4px',
                              fontSize: '0.9rem',
                              whiteSpace: 'pre-wrap',
                            }}
                          >
                            {String(job.result.content)}
                          </div>
                        </div>
                      )
                    })()}

                    {job.result.account_content_override &&
                      typeof job.result.account_content_override === 'object' &&
                      Object.keys(job.result.account_content_override as Record<string, string>).length > 0 && (
                        <div className="stack stack--xs">
                          <p className="hint" style={{ margin: 0 }}>Shortened overrides</p>
                          {Object.entries(job.result.account_content_override as Record<string, string>).map(
                            ([accountId, text]) => {
                              const account = accounts.find((item) => item.id === accountId)
                              return (
                                <div key={accountId} className="glass-panel glass-panel--compact">
                                  <strong style={{ fontSize: '0.8rem' }}>
                                    {account?.username || account?.name || accountId}
                                    {account?.maxChars ? ` · ${text.length}/${account.maxChars} chars` : ''}
                                  </strong>
                                  <p style={{ margin: '0.25rem 0 0', whiteSpace: 'pre-wrap', fontSize: '0.85rem' }}>
                                    {text}
                                  </p>
                                </div>
                              )
                            },
                          )}
                        </div>
                      )}

                    <div className="flex-row--center gap-2">
                      {profile?.autoPublishEnabled ? (
                        <p className="hint" style={{ fontSize: '0.8rem', margin: 0 }}>
                          <Check size={12} style={{ display: 'inline', verticalAlign: 'middle', marginRight: '4px' }} />
                          Auto-publish enabled
                        </p>
                      ) : (
                        <>
                          <button
                            className="btn btn--secondary btn--sm"
                            onClick={() => void saveGeneratedPost(job, false)}
                            disabled={savingDraftId === job.id || schedulingId === job.id}
                          >
                            {savingDraftId === job.id ? (
                              <>
                                <Loader2 size={14} className="spin" /> Saving...
                              </>
                            ) : (
                              <>
                                <Save size={14} /> Save draft
                              </>
                            )}
                          </button>
                          <button
                            className="btn btn--primary btn--sm"
                            onClick={() => void saveGeneratedPost(job, true)}
                            disabled={savingDraftId === job.id || schedulingId === job.id}
                          >
                            {schedulingId === job.id ? (
                              <>
                                <Loader2 size={14} className="spin" /> Scheduling...
                              </>
                            ) : (
                              <>
                                <CalendarClock size={14} /> Schedule
                              </>
                            )}
                          </button>
                        </>
                      )}
                    </div>
                  </>
                )}
              </div>
            ))}
          </div>
        )}

        {!latestCompleted && recentJobs.length === 0 && activeJobs.length === 0 && (
          <div className="glass-panel">
            <p className="hint">Generated posts will appear here.</p>
          </div>
        )}
      </div>
    </div>
  )
}
