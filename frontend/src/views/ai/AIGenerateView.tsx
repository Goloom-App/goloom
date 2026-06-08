import { useState } from 'react'
import { Check, Loader2, AlertCircle, Play, Save, X } from 'lucide-react'

import {
  useTriggerAIJob,
  useAIJobs,
  useCampaignFormats,
  useTeamProfile,
  useCancelAIJob,
} from '../../hooks/useAI'
import { useAIJobStream } from '../../hooks/useSSE'
import type { TeamRecord, AccountRecord, AIJobType, AIJob } from '../../types'
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

  const [jobType, setJobType] = useState<AIJobType>('voice_engine')
  
  const [platform, setPlatform] = useState('mastodon')
  const [promptHint, setPromptHint] = useState('')
  const [selectedAccounts, setSelectedAccounts] = useState<string[]>([])
  
  const [campaignFormatId, setCampaignFormatId] = useState('')
  const [targetDate, setTargetDate] = useState('')

  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [savingDraftId, setSavingDraftId] = useState<string | null>(null)

  if (!team.isAiEnabled) {
    return (
      <div className="empty-state">
        <p className="hint">AI features are not enabled for this team.</p>
      </div>
    )
  }

  const handleTrigger = async () => {
    setError(null)
    setStatusMessage(null)
    
    try {
      if (jobType === 'voice_engine') {
        if (!promptHint.trim()) {
          setError('Prompt hint is required')
          return
        }
        if (selectedAccounts.length === 0) {
          setError('Select at least one target account')
          return
        }
        
        await triggerJob.mutateAsync({
          teamId: team.id,
          type: 'voice_engine',
          params: {
            platform,
            prompt_hint: promptHint,
            target_account_ids: selectedAccounts,
          }
        })
      } else if (jobType === 'campaign_autopilot') {
        if (!campaignFormatId) {
          setError('Select a campaign format')
          return
        }
        if (!targetDate) {
          setError('Target date is required')
          return
        }
        
        await triggerJob.mutateAsync({
          teamId: team.id,
          type: 'campaign_autopilot',
          params: {
            campaign_format_id: campaignFormatId,
            target_date: targetDate,
          }
        })
      }
      
      setStatusMessage('AI Job triggered successfully')
      setPromptHint('')
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

  const handleSaveDraft = async (job: AIJob) => {
    if (!job.result || !job.result.content) return
    
    setSavingDraftId(job.id)
    setError(null)
    
    try {
      const raw = window.localStorage.getItem('goloom-ui-settings')
      let token = ''
      let baseUrl = window.location.origin
      if (raw) {
        const parsed = JSON.parse(raw)
        token = parsed.general?.bearerToken || ''
        baseUrl = parsed.general?.apiBaseUrl || baseUrl
      }
      
      const api = createApiClient({ baseUrl, token })
      
      await api.createAIDraft(team.id, {
        content: job.result.content,
        account_ids: job.payload.target_account_ids || [],
        ai_job_id: job.id
      })
      
      setStatusMessage('Draft saved successfully')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save draft')
    } finally {
      setSavingDraftId(null)
    }
  }

  const toggleAccount = (accountId: string) => {
    setSelectedAccounts(prev => 
      prev.includes(accountId) 
        ? prev.filter(id => id !== accountId)
        : [...prev, accountId]
    )
  }

  const activeJobs = jobs?.filter(j => j.status === 'pending' || j.status === 'processing') || []
  const recentJobs = jobs?.filter(j => j.status === 'completed' || j.status === 'failed').slice(0, 10) || []

  return (
    <div className="two-column-detail" data-testid="ai-generate-view">
      <div className="glass-panel stack">
        <h2 className="section-card__title">Generate Post</h2>
        <p className="hint">Use AI to generate content for your social media accounts.</p>

        {(error || statusMessage) && (
          <div className="status-banner-panel" style={{ padding: '1rem', marginBottom: '1rem' }}>
            {statusMessage && <span className="status-banner__success" data-testid="gen-status-success">{statusMessage}</span>}
            {error && <span className="status-banner__error" data-testid="gen-status-error">{error}</span>}
          </div>
        )}

        <div className="field">
          <span>Generation Type</span>
          <div className="flex-row--center gap-2">
            <button 
              data-testid="gen-type-voice"
              className={`btn ${jobType === 'voice_engine' ? 'btn--primary' : 'btn--secondary'}`}
              onClick={() => setJobType('voice_engine')}
            >
              Voice Engine
            </button>
            <button 
              data-testid="gen-type-campaign"
              className={`btn ${jobType === 'campaign_autopilot' ? 'btn--primary' : 'btn--secondary'}`}
              onClick={() => setJobType('campaign_autopilot')}
            >
              Campaign Auto-Pilot
            </button>
          </div>
        </div>

        {jobType === 'voice_engine' && (
          <div className="stack stack--sm mt-4">
            <label className="field">
              <span>Platform Context</span>
              <select data-testid="gen-platform" value={platform} onChange={(e) => setPlatform(e.target.value)}>
                <option value="mastodon">Mastodon</option>
                <option value="bluesky">Bluesky</option>
                <option value="friendica">Friendica</option>
                <option value="general">General</option>
              </select>
            </label>

            <label className="field">
              <span>Prompt Hint</span>
              <textarea
                data-testid="gen-prompt"
                rows={4}
                value={promptHint}
                onChange={(e) => setPromptHint(e.target.value)}
                placeholder="What should the post be about?"
              />
            </label>

            <div className="field">
              <span>Target Accounts</span>
              <div className="flex-row--wrap gap-2">
                {accounts.map(account => (
                  <button
                    key={account.id}
                    className={`badge ${selectedAccounts.includes(account.id) ? 'badge--primary' : 'badge--neutral'}`}
                    onClick={() => toggleAccount(account.id)}
                    style={{ cursor: 'pointer', border: 'none' }}
                  >
                    {account.name} ({account.provider})
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}

        {jobType === 'campaign_autopilot' && (
          <div className="stack stack--sm mt-4">
            <label className="field">
              <span>Campaign Format</span>
              <select data-testid="gen-campaign-format" value={campaignFormatId} onChange={(e) => setCampaignFormatId(e.target.value)}>
                <option value="">Select a format...</option>
                {formats?.filter(f => f.isActive).map(f => (
                  <option key={f.id} value={f.id}>{f.name}</option>
                ))}
              </select>
            </label>

            <label className="field">
              <span>Target Date</span>
              <input
                data-testid="gen-target-date"
                type="date"
                value={targetDate}
                onChange={(e) => setTargetDate(e.target.value)}
              />
            </label>
          </div>
        )}

        <div className="mt-4">
          <button
            data-testid="gen-submit"
            className="btn btn--primary"
            onClick={handleTrigger}
            disabled={triggerJob.isPending}
          >
            {triggerJob.isPending ? (
              <><Loader2 size={16} className="spin" /> Generating...</>
            ) : (
              <><Play size={16} /> Generate Post</>
            )}
          </button>
        </div>
      </div>

      <div className="stack">
        {activeJobs.length > 0 && (
          <div className="glass-panel stack" data-testid="gen-active-jobs">
            <h3 className="subsection-title">Active Jobs</h3>
            <div className="stack stack--sm">
              {activeJobs.map(job => (
                <div key={job.id} className="glass-panel glass-panel--compact flex-row--between" style={{ alignItems: 'center' }}>
                  <div>
                    <strong>{job.type === 'voice_engine' ? 'Voice Engine' : 'Campaign Auto-Pilot'}</strong>
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

        <div className="glass-panel stack" data-testid="gen-recent-jobs">
          <h3 className="subsection-title">Recent Jobs</h3>
          {recentJobs.length === 0 ? (
            <p className="hint">No recent AI jobs.</p>
          ) : (
            <div className="stack stack--sm">
              {recentJobs.map(job => (
                <div key={job.id} className="glass-panel glass-panel--compact stack stack--sm">
                  <div className="flex-row--between" style={{ alignItems: 'center' }}>
                    <div>
                      <strong>{job.type === 'voice_engine' ? 'Voice Engine' : 'Campaign Auto-Pilot'}</strong>
                      <p className="hint" style={{ fontSize: '0.8rem', margin: 0 }}>
                        {new Date(job.createdAt).toLocaleString()}
                      </p>
                    </div>
                    <div className="flex-row--center gap-2">
                      {job.status === 'completed' ? (
                        <Check size={16} style={{ color: 'var(--success)' }} />
                      ) : (
                        <AlertCircle size={16} style={{ color: 'var(--danger)' }} />
                      )}
                      <span className={`badge ${job.status === 'completed' ? 'badge--success' : 'badge--danger'}`} style={{ textTransform: 'capitalize' }}>
                        {job.status}
                      </span>
                    </div>
                  </div>
                  
                  {job.status === 'failed' && job.errorMessage && (
                    <div className="status-banner__error" style={{ fontSize: '0.8rem', padding: '0.5rem' }}>
                      {job.errorMessage}
                    </div>
                  )}
                  
                  {job.status === 'completed' && job.result && !!job.result.content && (
                    <div className="mt-2">
                      <div style={{ 
                        background: 'var(--bg-secondary)', 
                        padding: '0.75rem', 
                        borderRadius: '4px',
                        fontSize: '0.9rem',
                        whiteSpace: 'pre-wrap',
                        marginBottom: '0.75rem'
                      }}>
                        {String(job.result.content)}
                      </div>
                      
                      {profile?.autoPublishEnabled ? (
                        <p className="hint" style={{ fontSize: '0.8rem', margin: 0 }}>
                          <Check size={12} style={{ display: 'inline', verticalAlign: 'middle', marginRight: '4px' }} />
                          Draft auto-created
                        </p>
                      ) : (
                        <button
                          className="btn btn--secondary btn--sm"
                          onClick={() => handleSaveDraft(job)}
                          disabled={savingDraftId === job.id}
                        >
                          {savingDraftId === job.id ? (
                            <><Loader2 size={14} className="spin" /> Saving...</>
                          ) : (
                            <><Save size={14} /> Save as Draft</>
                          )}
                        </button>
                      )}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
