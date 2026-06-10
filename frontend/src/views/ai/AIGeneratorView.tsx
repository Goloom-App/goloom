import { useEffect, useMemo, useState } from 'react'
import { Edit, FileText, Loader2, Play, Send, Sparkles, X, Zap } from 'lucide-react'

import { ActionBar, OptionPill, SectionCard, Segmented } from '../../components/ui'
import {
  useTeamProfile,
  useTriggerAIJob,
  useAIJobs,
  useCancelAIJob,
  useAIPromptPreview,
} from '../../hooks/useAI'
import { useAIJobStream } from '../../hooks/useSSE'
import { DestinationPicker } from '../../components/ai/DestinationPicker'
import type {
  AccountRecord,
  AIJob,
  AIMoodAdjustment,
  AIOutputFormat,
  TeamRecord,
} from '../../types'
import { createApiClient } from '../../api'

interface AIGeneratorViewProps {
  team: TeamRecord
  accounts: AccountRecord[]
  onEditInComposer?: (payload: {
    title?: string
    content: string
    targetAccountIds: string[]
    accountContentOverride?: Record<string, string>
    scheduledAt?: string
  }) => void
}

const MOOD_OPTIONS: { id: AIMoodAdjustment; label: string }[] = [
  { id: 'more_expertise', label: 'Mehr Fachwissen' },
  { id: 'shorter_punchier', label: 'Kürzer & knackiger' },
  { id: 'remove_marketing_speak', label: 'Marketing-Sprache entfernen' },
]

const OUTPUT_FORMAT_OPTIONS: { id: AIOutputFormat; label: string }[] = [
  { id: 'post', label: 'Post' },
  { id: 'teaser', label: 'Teaser' },
  { id: 'poll', label: 'Umfrage' },
  { id: 'thread', label: 'Thread' },
]

const OCCASION_TYPES: { id: 'text' | 'url' | 'rss'; label: string }[] = [
  { id: 'text', label: 'Freitext' },
  { id: 'url', label: 'URL' },
  { id: 'rss', label: 'RSS-Feed' },
]

export function AIGeneratorView({ team, accounts, onEditInComposer }: AIGeneratorViewProps) {
  const { data: profile } = useTeamProfile(team.id)
  const { data: jobs } = useAIJobs(team.id)
  const triggerJob = useTriggerAIJob()
  const cancelJob = useCancelAIJob()
  const promptPreview = useAIPromptPreview()
  useAIJobStream(team.id)

  const [error, setError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)

  // Task input
  const [occasion, setOccasion] = useState('')
  const [occasionType, setOccasionType] = useState<'text' | 'url' | 'rss'>('text')
  const [outputFormat, setOutputFormat] = useState<AIOutputFormat>('post')
  const [selectedAccounts, setSelectedAccounts] = useState<string[]>([])
  const [activeJobId, setActiveJobId] = useState<string | null>(null)

  // Editor
  const [moodAdjustments, setMoodAdjustments] = useState<AIMoodAdjustment[]>([])
  const [showPromptPreview, setShowPromptPreview] = useState(false)
  const [promptText, setPromptText] = useState('')
  const [savingDraftId, setSavingDraftId] = useState<string | null>(null)

  const activeJobs = jobs?.filter((j) => j.status === 'pending' || j.status === 'processing') ?? []

  const latestVoiceResult = useMemo(
    () =>
      jobs?.find(
        (j) =>
          j.type === 'voice_engine' &&
          j.status === 'completed' &&
          typeof j.result?.content === 'string' &&
          j.result.content,
      ),
    [jobs],
  )

  const voiceJob = useMemo(() => {
    if (activeJobId) return jobs?.find((j) => j.id === activeJobId)
    return latestVoiceResult
  }, [jobs, activeJobId, latestVoiceResult])

  useEffect(() => {
    if (!activeJobId || !jobs) return
    const job = jobs.find((j) => j.id === activeJobId)
    if (job?.status === 'completed' && job.type === 'voice_engine') {
      setActiveJobId(null)
    }
  }, [jobs, activeJobId])

  if (!team.isAiEnabled) {
    return (
      <div className="empty-state">
        <p className="hint">AI features are not enabled for this team.</p>
      </div>
    )
  }

  const buildGenerationParams = (refine = false) => {
    const params: Record<string, unknown> = {
      occasion: occasion.trim(),
      output_format: outputFormat,
      target_account_ids: selectedAccounts,
      mood_adjustments: moodAdjustments,
      platform: accounts.find((a) => selectedAccounts.includes(a.id))?.provider ?? 'mastodon',
    }
    if (occasionType === 'url') params.source_url = occasion.trim()
    if (occasionType === 'rss') params.rss_feed_url = occasion.trim()
    if (refine && latestVoiceResult?.result?.content) {
      params.refine_content = true
      params.source_content = String(latestVoiceResult.result.content)
    }
    for (const mood of moodAdjustments) params[mood] = true
    return params
  }

  const handleGenerate = async () => {
    setError(null)
    if (!occasion.trim()) {
      setError('Bitte einen Anlass angeben')
      return
    }
    if (selectedAccounts.length === 0) {
      setError('Mindestens ein Konto auswählen')
      return
    }
    try {
      const response = await triggerJob.mutateAsync({
        teamId: team.id,
        type: 'voice_engine',
        params: buildGenerationParams(false),
      })
      if (response.jobId) setActiveJobId(response.jobId)
      setStatusMessage('Generierung gestartet')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Generierung fehlgeschlagen')
    }
  }

  const handleRefine = async () => {
    setError(null)
    try {
      const response = await triggerJob.mutateAsync({
        teamId: team.id,
        type: 'voice_engine',
        params: buildGenerationParams(true),
      })
      if (response.jobId) setActiveJobId(response.jobId)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Anpassung fehlgeschlagen')
    }
  }

  const handleLoadPromptPreview = async () => {
    setError(null)
    try {
      const preview = await promptPreview.mutateAsync({
        teamId: team.id,
        params: buildGenerationParams(Boolean(latestVoiceResult)),
      })
      setPromptText(preview.generation_prompt)
      setShowPromptPreview(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Prompt-Vorschau fehlgeschlagen')
    }
  }

  const toggleMood = (mood: AIMoodAdjustment) => {
    setMoodAdjustments((prev) => (prev.includes(mood) ? prev.filter((m) => m !== mood) : [...prev, mood]))
  }

  const toggleAccount = (accountId: string) => {
    setSelectedAccounts((prev) =>
      prev.includes(accountId) ? prev.filter((id) => id !== accountId) : [...prev, accountId],
    )
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

  const saveGeneratedPost = async (job: AIJob) => {
    if (!job.result?.content) return
    setSavingDraftId(job.id)
    setError(null)
    try {
      const api = getApi()
      await api.createAIDraft(team.id, {
        title: typeof job.result.title === 'string' ? job.result.title : '',
        content: String(job.result.content),
        account_ids: Array.isArray(job.payload?.target_account_ids)
          ? (job.payload.target_account_ids as string[])
          : selectedAccounts,
        account_content_override:
          job.result.account_content_override && typeof job.result.account_content_override === 'object'
            ? (job.result.account_content_override as Record<string, string>)
            : undefined,
        scheduled_at: typeof job.result.scheduled_at === 'string' ? job.result.scheduled_at : undefined,
        schedule: false,
        ai_job_id: job.id,
      })
      setStatusMessage('Entwurf gespeichert')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Speichern fehlgeschlagen')
    } finally {
      setSavingDraftId(null)
    }
  }

  const displayJob = voiceJob?.status === 'completed' ? voiceJob : latestVoiceResult

  return (
    <div className="brand-wizard" data-testid="ai-generator-view">
      {(error || statusMessage) && (
        <div className="status-banner-panel" style={{ padding: '0.75rem 1rem' }}>
          {statusMessage && <span className="status-banner__success" data-testid="gen-status-success">{statusMessage}</span>}
          {error && <span className="status-banner__error" data-testid="gen-status-error">{error}</span>}
        </div>
      )}

      <SectionCard
        icon={<Zap size={18} />}
        title="Was ist der Anlass?"
        subtitle="Quelle, Format und Ziel-Konten festlegen. Das Brand-Profil aus dem AI-Profil liefert den Vibe."
      >
        <div className="field">
          <span>Quelle</span>
          <Segmented
            value={occasionType}
            options={OCCASION_TYPES}
            onChange={(v) => setOccasionType(v)}
            testIdPrefix="gen-occasion-type"
          />
        </div>
        <label className="field">
          <span>{occasionType === 'text' ? 'Beschreibung des Anlasses' : occasionType === 'url' ? 'URL' : 'RSS-Feed-Link'}</span>
          <textarea
            data-testid="gen-occasion"
            rows={4}
            value={occasion}
            onChange={(e) => setOccasion(e.target.value)}
            placeholder={
              occasionType === 'text'
                ? 'Worum geht es? Was ist neu, was willst du teilen?'
                : occasionType === 'url'
                  ? 'https://…'
                  : 'https://example.com/feed.xml'
            }
          />
        </label>
        <div className="field">
          <span>Format</span>
          <Segmented
            value={outputFormat}
            options={OUTPUT_FORMAT_OPTIONS}
            onChange={(v) => setOutputFormat(v)}
            testIdPrefix="gen-output-format"
          />
        </div>
        <div className="field">
          <span>Ziel-Konten</span>
          <DestinationPicker
            accounts={accounts}
            selectedIds={selectedAccounts}
            onToggle={toggleAccount}
            testIdPrefix="gen-dest"
          />
        </div>
      </SectionCard>

      {activeJobs.length > 0 && (
        <SectionCard
          icon={<Loader2 size={18} className="spin" />}
          title="Generierung läuft …"
          subtitle="Du kannst den Job jederzeit abbrechen."
        >
          <div className="brand-actionbar__group">
            {activeJobs.map((job) => (
              <button
                key={job.id}
                type="button"
                className="btn btn--secondary btn--sm"
                onClick={() => void cancelJob.mutateAsync({ teamId: team.id, jobId: job.id })}
              >
                <X size={14} /> Abbrechen
              </button>
            ))}
          </div>
        </SectionCard>
      )}

      {displayJob && typeof displayJob.result?.content === 'string' && (
        <SectionCard
          icon={<Sparkles size={18} />}
          title="Generierter Post"
          subtitle="Feinschliff über die Stimmungs-Regler — keine neue Eingabe nötig."
          testId="gen-editor"
        >
          <div className="brand-generated-post">{String(displayJob.result.content)}</div>

          <div className="field">
            <span>Stimmungs-Regler</span>
            <div className="brand-option-grid">
              {MOOD_OPTIONS.map((option) => (
                <OptionPill
                  key={option.id}
                  active={moodAdjustments.includes(option.id)}
                  onClick={() => toggleMood(option.id)}
                  testId={`gen-mood-${option.id}`}
                >
                  {option.label}
                </OptionPill>
              ))}
            </div>
            <p className="brand-field__hint">Wähle aus, was angepasst werden soll, und klicke „Neu generieren".</p>
          </div>

          {showPromptPreview && promptText && (
            <details open className="brand-prompt-preview" data-testid="gen-prompt-preview-panel">
              <summary><FileText size={14} /> Haupt-Prompt</summary>
              <pre>{promptText}</pre>
            </details>
          )}
        </SectionCard>
      )}

      {!displayJob && activeJobs.length === 0 && (
        <SectionCard
          icon={<Sparkles size={18} />}
          title="Noch kein Post generiert"
          subtitle={'Fülle das Formular oben und klicke „Generieren".'}
        >
          <p className="brand-field__hint" style={{ margin: 0 }}>Das Ergebnis erscheint hier.</p>
        </SectionCard>
      )}

      <ActionBar
        left={
          displayJob ? (
            <button
              type="button"
              className="btn btn--ghost"
              data-testid="gen-prompt-preview"
              onClick={() => void handleLoadPromptPreview()}
              disabled={promptPreview.isPending}
            >
              {promptPreview.isPending ? <><Loader2 size={14} className="spin" /> Lade…</> : <><FileText size={14} /> Prompt-Vorschau</>}
            </button>
          ) : null
        }
        right={
          <>
            {displayJob && (
              <button
                type="button"
                className="btn btn--secondary"
                onClick={() => void handleRefine()}
                disabled={triggerJob.isPending || moodAdjustments.length === 0}
              >
                <Sparkles size={14} /> Neu generieren
              </button>
            )}
            {onEditInComposer && displayJob?.status === 'completed' && (
              <button
                type="button"
                className="btn btn--secondary"
                onClick={() =>
                  onEditInComposer({
                    title: typeof displayJob.result?.title === 'string' ? displayJob.result.title : undefined,
                    content: String(displayJob.result?.content ?? ''),
                    targetAccountIds: selectedAccounts,
                  })
                }
              >
                <Edit size={14} /> Im Composer öffnen
              </button>
            )}
            {displayJob?.status === 'completed' && !profile?.autoPublishEnabled && (
              <button
                type="button"
                className="btn btn--secondary"
                onClick={() => void saveGeneratedPost(displayJob)}
                disabled={savingDraftId === displayJob.id}
              >
                {savingDraftId === displayJob.id ? <><Loader2 size={14} className="spin" /> Speichern…</> : <><Send size={14} /> Als Entwurf speichern</>}
              </button>
            )}
            <button
              type="button"
              className="btn btn--primary"
              data-testid="gen-generate"
              onClick={() => void handleGenerate()}
              disabled={triggerJob.isPending}
            >
              {triggerJob.isPending ? <><Loader2 size={14} className="spin" /> Generiere…</> : <><Play size={14} /> Generieren</>}
            </button>
          </>
        }
      />
    </div>
  )
}
