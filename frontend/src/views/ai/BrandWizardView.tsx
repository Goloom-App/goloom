import { useEffect, useMemo, useState } from 'react'
import { CalendarClock, ChevronRight, Loader2, Play, Save, Sparkles, Trash2, X } from 'lucide-react'

import {
  useTeamProfile,
  useUpsertTeamProfile,
  useKnowledgeSources,
  useCreateKnowledgeSource,
  useDeleteKnowledgeSource,
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

type WizardStep = 1 | 2 | 3

interface BrandWizardViewProps {
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

const STEPS: { id: WizardStep; label: string }[] = [
  { id: 1, label: 'Setup' },
  { id: 2, label: 'Aufgabe' },
  { id: 3, label: 'Editor' },
]

export function BrandWizardView({ team, accounts, onEditInComposer }: BrandWizardViewProps) {
  const { data: profile, isLoading } = useTeamProfile(team.id)
  const { data: knowledgeSources } = useKnowledgeSources(team.id)
  const { data: jobs } = useAIJobs(team.id)
  const upsertProfile = useUpsertTeamProfile()
  const createKnowledge = useCreateKnowledgeSource()
  const deleteKnowledge = useDeleteKnowledgeSource()
  const triggerJob = useTriggerAIJob()
  const cancelJob = useCancelAIJob()
  const promptPreview = useAIPromptPreview()
  useAIJobStream(team.id)

  const [step, setStep] = useState<WizardStep>(1)
  const [error, setError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)

  // Identity
  const [archetype, setArchetype] = useState('')
  const [persona, setPersona] = useState('')
  const [industry, setIndustry] = useState('')
  const [mainValue, setMainValue] = useState('')
  const [targetAudience, setTargetAudience] = useState('')

  // Language DNA — all free text
  const [sentenceStyle, setSentenceStyle] = useState('')
  const [humorStyle, setHumorStyle] = useState('')
  const [preferredWords, setPreferredWords] = useState<string[]>([])
  const [newPreferredWord, setNewPreferredWord] = useState('')
  const [signaturePhrases, setSignaturePhrases] = useState<string[]>([])
  const [newSignaturePhrase, setNewSignaturePhrase] = useState('')
  const [bannedWords, setBannedWords] = useState<string[]>([])
  const [newBannedWord, setNewBannedWord] = useState('')
  const [antiAiOverride, setAntiAiOverride] = useState(false)

  // Reach
  const [hookStyle, setHookStyle] = useState('')
  const [ctaFocus, setCtaFocus] = useState('')

  const [preferredLanguage, setPreferredLanguage] = useState('de')
  const [maxHashtags, setMaxHashtags] = useState(3)
  const [autoPublishEnabled, setAutoPublishEnabled] = useState(false)

  // Profile assistant
  const [assistantBrief, setAssistantBrief] = useState('')
  const [assistantJobId, setAssistantJobId] = useState<string | null>(null)
  const [assistantOpen, setAssistantOpen] = useState(false)

  // Knowledge base form
  const [kbName, setKbName] = useState('')
  const [kbType, setKbType] = useState<'text' | 'url'>('text')
  const [kbContent, setKbContent] = useState('')
  const [kbUrl, setKbUrl] = useState('')

  // Vibe preview
  const [vibeSummary, setVibeSummary] = useState<string | null>(null)
  const [vibeSuggestion, setVibeSuggestion] = useState<string | null>(null)

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

  useEffect(() => {
    if (!profile) return
    const meta = profile.styleMetadata
    setArchetype(meta.identity?.archetype ?? '')
    setPersona(meta.identity?.persona ?? '')
    setIndustry(meta.identity?.industry ?? '')
    setMainValue(meta.identity?.mainValue ?? '')
    setTargetAudience(meta.identity?.targetAudience ?? '')
    setSentenceStyle(meta.languageDna?.sentenceStyle ?? '')
    setHumorStyle(meta.languageDna?.humorStyle ?? '')
    setPreferredWords(meta.languageDna?.preferredWords ?? [])
    setSignaturePhrases(meta.languageDna?.signaturePhrases ?? [])
    setAntiAiOverride(Boolean(meta.languageDna?.antiAiOverride))
    setBannedWords(meta.bannedWords ?? [])
    setHookStyle(meta.reachStrategy?.hookStyle ?? '')
    setCtaFocus(meta.reachStrategy?.ctaFocus ?? '')
    setPreferredLanguage(meta.preferredLanguage || 'de')
    setMaxHashtags(meta.maxHashtags ?? 3)
    setAutoPublishEnabled(profile.autoPublishEnabled)
  }, [profile])

  const activeJobs = jobs?.filter((j) => j.status === 'pending' || j.status === 'processing') ?? []
  const voiceJob = useMemo(() => {
    if (activeJobId) {
      return jobs?.find((j) => j.id === activeJobId)
    }
    return jobs?.find((j) => j.type === 'voice_engine' && (j.status === 'completed' || j.status === 'processing' || j.status === 'pending'))
  }, [jobs, activeJobId])

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

  useEffect(() => {
    if (!activeJobId || !jobs) return
    const job = jobs.find((j) => j.id === activeJobId)
    if (job?.status === 'completed' && job.type === 'voice_engine') {
      setStep(3)
      setActiveJobId(null)
    }
    if (job?.type === 'vibe_preview' && job.status === 'completed' && typeof job.result?.summary === 'string') {
      setVibeSummary(job.result.summary)
      setVibeSuggestion(typeof job.result.suggestion === 'string' ? job.result.suggestion : null)
    }
  }, [jobs, activeJobId])

  // Apply profile_assistant proposal to the form when its job completes.
  useEffect(() => {
    if (!assistantJobId || !jobs) return
    const job = jobs.find((j) => j.id === assistantJobId)
    if (!job || job.status === 'pending' || job.status === 'processing') return
    if (job.status === 'failed') {
      setError(job.errorMessage || 'KI-Assistent ist fehlgeschlagen')
      setAssistantJobId(null)
      return
    }
    const proposed = job.result?.proposed_profile as Record<string, unknown> | undefined
    if (proposed && typeof proposed === 'object') {
      const identity = (proposed.identity as Record<string, string> | undefined) ?? {}
      const dna = (proposed.language_dna as Record<string, unknown> | undefined) ?? {}
      const reach = (proposed.reach_strategy as Record<string, string> | undefined) ?? {}
      if (identity.archetype) setArchetype(String(identity.archetype))
      if (identity.persona) setPersona(String(identity.persona))
      if (identity.industry) setIndustry(String(identity.industry))
      if (identity.main_value) setMainValue(String(identity.main_value))
      if (identity.target_audience) setTargetAudience(String(identity.target_audience))
      if (dna.sentence_style) setSentenceStyle(String(dna.sentence_style))
      if (dna.humor_style) setHumorStyle(String(dna.humor_style))
      if (Array.isArray(dna.preferred_words)) setPreferredWords(dna.preferred_words.map(String))
      if (Array.isArray(dna.signature_phrases)) setSignaturePhrases(dna.signature_phrases.map(String))
      if (Array.isArray(proposed.banned_words)) setBannedWords((proposed.banned_words as unknown[]).map(String))
      if (reach.hook_style) setHookStyle(String(reach.hook_style))
      if (reach.cta_focus) setCtaFocus(String(reach.cta_focus))
      if (typeof proposed.max_hashtags === 'number') setMaxHashtags(proposed.max_hashtags)
      if (typeof proposed.preferred_language === 'string') setPreferredLanguage(proposed.preferred_language)
      setStatusMessage('Profilvorschlag eingefügt — bitte prüfen und speichern')
      setAssistantOpen(false)
    }
    setAssistantJobId(null)
  }, [jobs, assistantJobId])

  if (!team.isAiEnabled) {
    return (
      <div className="empty-state">
        <p className="hint">AI features are not enabled for this team.</p>
      </div>
    )
  }

  if (isLoading) {
    return <p className="hint">Loading brand profile...</p>
  }

  const buildStyleMetadata = () => ({
    formatting_rules: profile?.styleMetadata.formattingRules ?? [],
    banned_words: bannedWords,
    max_hashtags: maxHashtags,
    preferred_language: preferredLanguage,
    identity: {
      archetype,
      persona,
      industry,
      main_value: mainValue,
      target_audience: targetAudience,
    },
    language_dna: {
      sentence_style: sentenceStyle,
      preferred_words: preferredWords,
      signature_phrases: signaturePhrases,
      humor_style: humorStyle,
      anti_ai_override: antiAiOverride,
    },
    reach_strategy: {
      hook_style: hookStyle,
      cta_focus: ctaFocus,
    },
  })

  const handleAssistantSubmit = async () => {
    if (!assistantBrief.trim()) {
      setError('Bitte beschreibe dich oder dein Projekt')
      return
    }
    setError(null)
    setStatusMessage('KI-Assistent denkt nach …')
    try {
      const response = await triggerJob.mutateAsync({
        teamId: team.id,
        type: 'profile_assistant',
        params: {
          brief: assistantBrief.trim(),
          language: preferredLanguage,
        },
      })
      if (response.jobId) {
        setAssistantJobId(response.jobId)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'KI-Assistent fehlgeschlagen')
    }
  }

  const handleSaveSetup = async () => {
    setError(null)
    setStatusMessage(null)
    try {
      await upsertProfile.mutateAsync({
        teamId: team.id,
        data: {
          style_metadata: buildStyleMetadata(),
          auto_publish_enabled: autoPublishEnabled,
        },
      })
      setStatusMessage('Profil gespeichert')
      try {
        const response = await triggerJob.mutateAsync({
          teamId: team.id,
          type: 'vibe_preview',
          params: {},
        })
        if (response.jobId) {
          setActiveJobId(response.jobId)
        }
        setStatusMessage('Profil gespeichert — Vibe-Vorschau wird erstellt…')
      } catch {
        // Vibe preview is optional when the AI service is not configured.
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Speichern fehlgeschlagen')
    }
  }

  const handleAddKnowledge = async () => {
    if (!kbName.trim()) return
    setError(null)
    try {
      await createKnowledge.mutateAsync({
        teamId: team.id,
        data:
          kbType === 'url'
            ? { type: 'url', name: kbName.trim(), source_url: kbUrl.trim() }
            : { type: 'text', name: kbName.trim(), content: kbContent.trim() },
      })
      setKbName('')
      setKbContent('')
      setKbUrl('')
      setStatusMessage('Wissensquelle hinzugefügt')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Wissensquelle konnte nicht hinzugefügt werden')
    }
  }

  const buildGenerationParams = (refine = false) => {
    const params: Record<string, unknown> = {
      occasion: occasion.trim(),
      output_format: outputFormat,
      target_account_ids: selectedAccounts,
      mood_adjustments: moodAdjustments,
      platform: accounts.find((a) => selectedAccounts.includes(a.id))?.provider ?? 'mastodon',
    }
    if (occasionType === 'url') {
      params.source_url = occasion.trim()
    }
    if (occasionType === 'rss') {
      params.rss_feed_url = occasion.trim()
    }
    if (refine && latestVoiceResult?.result?.content) {
      params.refine_content = true
      params.source_content = String(latestVoiceResult.result.content)
    }
    for (const mood of moodAdjustments) {
      params[mood] = true
    }
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
      if (response.jobId) {
        setActiveJobId(response.jobId)
      }
      setStep(3)
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
      if (response.jobId) {
        setActiveJobId(response.jobId)
      }
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
    <div className="stack" data-testid="brand-wizard-view">
      <div className="flex-row--center gap-2 flex-wrap" style={{ marginBottom: '1rem' }}>
        {STEPS.map((s, idx) => (
          <div key={s.id} className="flex-row--center gap-2">
            <button
              type="button"
              className={`btn btn--sm ${step === s.id ? 'btn--primary' : 'btn--secondary'}`}
              onClick={() => setStep(s.id)}
            >
              {s.id}. {s.label}
            </button>
            {idx < STEPS.length - 1 ? <ChevronRight size={14} className="hint" /> : null}
          </div>
        ))}
      </div>

      {(error || statusMessage) && (
        <div className="status-banner-panel" style={{ padding: '1rem' }}>
          {statusMessage && <span className="status-banner__success" data-testid="brand-status-success">{statusMessage}</span>}
          {error && <span className="status-banner__error" data-testid="brand-status-error">{error}</span>}
        </div>
      )}

      {step === 1 && (
        <div className="stack stack--lg">
          <div className="glass-panel stack" data-testid="brand-assistant-panel">
            <div className="flex-row--between" style={{ alignItems: 'center' }}>
              <h2 className="section-card__title" style={{ margin: 0 }}>
                <Sparkles size={16} style={{ display: 'inline', verticalAlign: 'middle', marginRight: '0.4rem' }} />
                KI-Assistent
              </h2>
              <button
                type="button"
                className="btn btn--secondary btn--sm"
                data-testid="brand-assistant-toggle"
                onClick={() => setAssistantOpen((v) => !v)}
              >
                {assistantOpen ? 'Schließen' : 'Profil von KI erstellen lassen'}
              </button>
            </div>
            <p className="hint" style={{ margin: 0 }}>
              Beschreibe in 2–4 Sätzen, wer du bist und für wen du postest. Die KI füllt das Profil mit
              einem konkreten Vorschlag aus — du kannst alles nachträglich anpassen.
            </p>
            {assistantOpen && (
              <>
                <textarea
                  data-testid="brand-assistant-brief"
                  rows={4}
                  value={assistantBrief}
                  onChange={(e) => setAssistantBrief(e.target.value)}
                  placeholder={'z. B. „Wir sind ein Selfhosting-Podcast für Anfänger, sprechen über Heimserver, Home-Assistant und Datenschutz. Zielgruppe: Tech-Nerds, leicht zynisch.“'}
                />
                <div className="flex-row--center gap-2">
                  <button
                    type="button"
                    className="btn btn--primary"
                    data-testid="brand-assistant-submit"
                    onClick={() => void handleAssistantSubmit()}
                    disabled={triggerJob.isPending || Boolean(assistantJobId)}
                  >
                    {assistantJobId ? (
                      <>
                        <Loader2 size={14} className="spin" /> KI denkt nach…
                      </>
                    ) : (
                      <>
                        <Sparkles size={14} /> Vorschlag generieren
                      </>
                    )}
                  </button>
                </div>
              </>
            )}
          </div>

          <div className="glass-panel stack">
            <h2 className="section-card__title">Identität — Wer bist du?</h2>
            <label className="field">
              <span>Archetyp</span>
              <input
                data-testid="brand-archetype"
                value={archetype}
                onChange={(e) => setArchetype(e.target.value)}
                placeholder="z. B. Tech-Podcast, Zahnarztpraxis, Solo-Privat, Werbeagentur"
              />
              <p className="hint" style={{ fontSize: '0.8rem' }}>Kurzes Label, das deinen Account einordnet.</p>
            </label>
            <label className="field">
              <span>Voice-Persona</span>
              <textarea
                data-testid="brand-persona"
                rows={2}
                value={persona}
                onChange={(e) => setPersona(e.target.value)}
                placeholder={'z. B. „Maximilian, 38, IT-Nerd mit Selfhosting-Spleen, redet wie mit Kollegen am Stehtisch.“'}
              />
              <p className="hint" style={{ fontSize: '0.8rem' }}>
                Wer schreibt? Beschreibe die Person hinter dem Account — das prägt den Vibe stärker als jede Tonalitätsangabe.
              </p>
            </label>
            <label className="field">
              <span>Branche / Kontext</span>
              <input data-testid="brand-industry" value={industry} onChange={(e) => setIndustry(e.target.value)} placeholder="z. B. Open-Source Hosting, Zahnmedizin, B2B-Marketing" />
            </label>
            <label className="field">
              <span>Haupt-Mehrwert</span>
              <input data-testid="brand-main-value" value={mainValue} onChange={(e) => setMainValue(e.target.value)} placeholder="Was bietest du einzigartig? Konkret, kein Marketing." />
            </label>
            <label className="field">
              <span>Zielgruppe</span>
              <input data-testid="brand-audience" value={targetAudience} onChange={(e) => setTargetAudience(e.target.value)} placeholder='z. B. „Patienten mit Zahnarzt-Angst“ oder „Hobby-Sysadmins über 30“' />
            </label>
          </div>

          <div className="glass-panel stack">
            <h2 className="section-card__title">Sprach-DNA — Wie redest du?</h2>
            <label className="field">
              <span>Satzbau</span>
              <textarea
                data-testid="brand-sentence-style"
                rows={2}
                value={sentenceStyle}
                onChange={(e) => setSentenceStyle(e.target.value)}
                placeholder={'z. B. „Kurze Sätze, gerne Halbsätze. Kein Verb-am-Ende-Drama.“'}
              />
            </label>
            <label className="field">
              <span>Humor / Ton</span>
              <textarea
                data-testid="brand-humor"
                rows={2}
                value={humorStyle}
                onChange={(e) => setHumorStyle(e.target.value)}
                placeholder={'z. B. „Trocken mit IT-Insider-Witzen“ oder „Warm und beruhigend für Angstpatienten“'}
              />
            </label>
            <div className="field">
              <span>Bevorzugte Wörter / Fachbegriffe</span>
              <div className="flex-row--wrap gap-2 mb-2">
                {preferredWords.map((word, idx) => (
                  <div key={idx} className="badge flex-row--center gap-1">
                    <span>{word}</span>
                    <button type="button" className="btn btn--ghost btn--xs" onClick={() => setPreferredWords(preferredWords.filter((_, i) => i !== idx))}>
                      <X size={12} />
                    </button>
                  </div>
                ))}
              </div>
              <div className="flex-row--center gap-2">
                <input value={newPreferredWord} onChange={(e) => setNewPreferredWord(e.target.value)} placeholder="Wort hinzufügen" onKeyDown={(e) => {
                  if (e.key === 'Enter' && newPreferredWord.trim()) {
                    e.preventDefault()
                    setPreferredWords([...preferredWords, newPreferredWord.trim()])
                    setNewPreferredWord('')
                  }
                }} />
                <button type="button" className="btn btn--secondary" onClick={() => {
                  if (newPreferredWord.trim()) {
                    setPreferredWords([...preferredWords, newPreferredWord.trim()])
                    setNewPreferredWord('')
                  }
                }}>Add</button>
              </div>
            </div>
            <div className="field">
              <span>Signature-Phrasen</span>
              <div className="flex-row--wrap gap-2 mb-2">
                {signaturePhrases.map((phrase, idx) => (
                  <div key={idx} className="badge flex-row--center gap-1">
                    <span>{phrase}</span>
                    <button type="button" className="btn btn--ghost btn--xs" onClick={() => setSignaturePhrases(signaturePhrases.filter((_, i) => i !== idx))}>
                      <X size={12} />
                    </button>
                  </div>
                ))}
              </div>
              <div className="flex-row--center gap-2">
                <input
                  data-testid="brand-signature-phrase"
                  value={newSignaturePhrase}
                  onChange={(e) => setNewSignaturePhrase(e.target.value)}
                  placeholder={'z. B. „läuft auf meinem Pi“'}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && newSignaturePhrase.trim()) {
                      e.preventDefault()
                      setSignaturePhrases([...signaturePhrases, newSignaturePhrase.trim()])
                      setNewSignaturePhrase('')
                    }
                  }}
                />
                <button type="button" className="btn btn--secondary" onClick={() => {
                  if (newSignaturePhrase.trim()) {
                    setSignaturePhrases([...signaturePhrases, newSignaturePhrase.trim()])
                    setNewSignaturePhrase('')
                  }
                }}>Add</button>
              </div>
              <p className="hint" style={{ fontSize: '0.8rem' }}>Wiederkehrende Wendungen, die deinen Account erkennbar machen. Werden nur eingesetzt, wenn sie wirklich passen.</p>
            </div>
            <div className="field">
              <span>Verbotene Wörter (zusätzlich zu den Standard-KI-Phrasen)</span>
              <div className="flex-row--wrap gap-2 mb-2">
                {bannedWords.map((word, idx) => (
                  <div key={idx} className="badge flex-row--center gap-1">
                    <span>{word}</span>
                    <button type="button" className="btn btn--ghost btn--xs" onClick={() => setBannedWords(bannedWords.filter((_, i) => i !== idx))}>
                      <X size={12} />
                    </button>
                  </div>
                ))}
              </div>
              <div className="flex-row--center gap-2">
                <input
                  data-testid="brand-banned-word"
                  value={newBannedWord}
                  onChange={(e) => setNewBannedWord(e.target.value)}
                  placeholder="zusätzliche Wörter, die nie auftauchen sollen"
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && newBannedWord.trim()) {
                      e.preventDefault()
                      setBannedWords([...bannedWords, newBannedWord.trim()])
                      setNewBannedWord('')
                    }
                  }}
                />
                <button type="button" className="btn btn--secondary" onClick={() => {
                  if (newBannedWord.trim()) {
                    setBannedWords([...bannedWords, newBannedWord.trim()])
                    setNewBannedWord('')
                  }
                }}>Add</button>
              </div>
              <p className="hint" style={{ fontSize: '0.8rem' }}>
                Goloom blockt automatisch typische KI-Phrasen („tauche ein“, „spannend“, „game-changer“ …). Hier kannst du eigene ergänzen.
              </p>
              <label className="flex-row--center gap-2" style={{ flexDirection: 'row', alignItems: 'center', marginTop: '0.5rem' }}>
                <input
                  type="checkbox"
                  data-testid="brand-anti-ai-override"
                  checked={antiAiOverride}
                  onChange={(e) => setAntiAiOverride(e.target.checked)}
                />
                <span style={{ fontSize: '0.85rem' }}>Standard-KI-Phrasen-Block deaktivieren (nur eigene Wörter verwenden)</span>
              </label>
            </div>
            <label className="field">
              <span>Sprache</span>
              <select value={preferredLanguage} onChange={(e) => setPreferredLanguage(e.target.value)}>
                <option value="de">Deutsch</option>
                <option value="en">English</option>
              </select>
            </label>
          </div>

          <div className="glass-panel stack">
            <h2 className="section-card__title">Reach-Strategie</h2>
            <label className="field">
              <span>Hook-Stil</span>
              <textarea
                data-testid="brand-hook"
                rows={2}
                value={hookStyle}
                onChange={(e) => setHookStyle(e.target.value)}
                placeholder={'z. B. „Mit einer konkreten Beobachtung einsteigen, nie mit Floskeln.“'}
              />
            </label>
            <label className="field">
              <span>CTA-Fokus</span>
              <textarea
                data-testid="brand-cta"
                rows={2}
                value={ctaFocus}
                onChange={(e) => setCtaFocus(e.target.value)}
                placeholder={'z. B. „Zum Kommentar einladen, kein Verkaufslink.“ oder „Direkt zur Terminbuchung.“'}
              />
            </label>
            <label className="field">
              <span>Max. Hashtags</span>
              <input type="number" min={0} max={30} value={maxHashtags} onChange={(e) => setMaxHashtags(parseInt(e.target.value, 10) || 0)} />
            </label>
          </div>

          <div className="glass-panel stack" data-testid="brand-knowledge-section">
            <h2 className="section-card__title">Knowledge-Base — Das Gold</h2>
            <p className="hint">Nur Fakten aus diesen Quellen. Die KI darf nichts erfinden, was hier nicht steht.</p>
            <div className="stack stack--sm">
              {(knowledgeSources ?? []).map((source) => (
                <div key={source.id} className="glass-panel glass-panel--compact flex-row--between" style={{ alignItems: 'center' }}>
                  <div>
                    <strong>{source.name}</strong>
                    <p className="hint" style={{ margin: 0, fontSize: '0.8rem' }}>{source.type} · {source.content.slice(0, 80)}{source.content.length > 80 ? '…' : ''}</p>
                  </div>
                  <button type="button" className="btn btn--ghost btn--xs" onClick={() => void deleteKnowledge.mutateAsync({ teamId: team.id, sourceId: source.id })}>
                    <Trash2 size={14} />
                  </button>
                </div>
              ))}
            </div>
            <label className="field">
              <span>Typ</span>
              <select value={kbType} onChange={(e) => setKbType(e.target.value as 'text' | 'url')}>
                <option value="text">Text / Transkript</option>
                <option value="url">Website-URL</option>
              </select>
            </label>
            <label className="field">
              <span>Name</span>
              <input value={kbName} onChange={(e) => setKbName(e.target.value)} placeholder="z. B. Produkt-FAQ" />
            </label>
            {kbType === 'url' ? (
              <label className="field">
                <span>URL</span>
                <input value={kbUrl} onChange={(e) => setKbUrl(e.target.value)} placeholder="https://…" />
              </label>
            ) : (
              <label className="field">
                <span>Inhalt</span>
                <textarea rows={4} value={kbContent} onChange={(e) => setKbContent(e.target.value)} placeholder="Fakten, Zitate, Produktinfos…" />
              </label>
            )}
            <button type="button" className="btn btn--secondary" onClick={() => void handleAddKnowledge()} disabled={createKnowledge.isPending}>
              Wissensquelle hinzufügen
            </button>
          </div>

          <div className="flex-row--center gap-2 flex-wrap">
            <button type="button" className="btn btn--primary" data-testid="brand-save-setup" onClick={() => void handleSaveSetup()} disabled={upsertProfile.isPending}>
              {upsertProfile.isPending ? 'Speichern…' : 'Profil speichern & Vibe-Vorschau'}
            </button>
            <button type="button" className="btn btn--secondary" onClick={() => setStep(2)}>
              Weiter zur Aufgabe <ChevronRight size={14} />
            </button>
          </div>

          {vibeSummary && (
            <div className="glass-panel stack" data-testid="brand-vibe-preview">
              <h3 className="subsection-title"><Sparkles size={16} style={{ display: 'inline', verticalAlign: 'middle' }} /> Vibe-Vorschau</h3>
              <p style={{ margin: 0 }}>{vibeSummary}</p>
              {vibeSuggestion ? <p className="hint" style={{ margin: 0 }}>{vibeSuggestion}</p> : null}
            </div>
          )}
        </div>
      )}

      {step === 2 && (
        <div className="glass-panel stack">
          <h2 className="section-card__title">Was ist der Anlass?</h2>
          <label className="field">
            <span>Quelle</span>
            <select value={occasionType} onChange={(e) => setOccasionType(e.target.value as typeof occasionType)}>
              <option value="text">Freitext / Notiz</option>
              <option value="url">URL</option>
              <option value="rss">RSS-Feed</option>
            </select>
          </label>
          <label className="field">
            <span>Anlass</span>
            <textarea data-testid="brand-occasion" rows={5} value={occasion} onChange={(e) => setOccasion(e.target.value)} placeholder="URL, Feed-Link oder Beschreibung des Anlasses…" />
          </label>
          <label className="field">
            <span>Format</span>
            <select data-testid="brand-output-format" value={outputFormat} onChange={(e) => setOutputFormat(e.target.value as AIOutputFormat)}>
              <option value="post">Post</option>
              <option value="teaser">Teaser</option>
              <option value="poll">Umfrage</option>
              <option value="thread">Thread</option>
            </select>
          </label>
          <div className="field">
            <span>Ziel-Konten</span>
            <DestinationPicker accounts={accounts} selectedIds={selectedAccounts} onToggle={toggleAccount} testIdPrefix="brand-dest" />
          </div>
          <div className="flex-row--center gap-2">
            <button type="button" className="btn btn--secondary" onClick={() => setStep(1)}>Zurück</button>
            <button type="button" className="btn btn--primary" data-testid="brand-generate" onClick={() => void handleGenerate()} disabled={triggerJob.isPending}>
              {triggerJob.isPending ? <><Loader2 size={16} className="spin" /> Generiere…</> : <><Play size={16} /> Generieren</>}
            </button>
          </div>
        </div>
      )}

      {step === 3 && (
        <div className="stack">
          {activeJobs.length > 0 && (
            <div className="glass-panel flex-row--center gap-2">
              <Loader2 size={16} className="spin" />
              <span>Generierung läuft…</span>
              {activeJobs.map((job) => (
                <button key={job.id} type="button" className="btn btn--secondary btn--sm" onClick={() => void cancelJob.mutateAsync({ teamId: team.id, jobId: job.id })}>
                  Abbrechen
                </button>
              ))}
            </div>
          )}

          {displayJob && typeof displayJob.result?.content === 'string' && (
            <div className="glass-panel stack" data-testid="brand-editor">
              <h2 className="section-card__title">Generierter Post</h2>
              <div style={{ background: 'var(--bg-secondary)', padding: '0.75rem', borderRadius: '4px', whiteSpace: 'pre-wrap' }}>
                {String(displayJob.result.content)}
              </div>

              <div className="stack stack--sm">
                <p className="hint" style={{ margin: 0 }}>Stimmungs-Regler</p>
                <label className="field flex-row--center gap-2" style={{ flexDirection: 'row' }}>
                  <input type="checkbox" checked={moodAdjustments.includes('more_expertise')} onChange={() => toggleMood('more_expertise')} />
                  <span>Mehr Fokus auf Fachwissen</span>
                </label>
                <label className="field flex-row--center gap-2" style={{ flexDirection: 'row' }}>
                  <input type="checkbox" checked={moodAdjustments.includes('shorter_punchier')} onChange={() => toggleMood('shorter_punchier')} />
                  <span>Kürzer / knackiger</span>
                </label>
                <label className="field flex-row--center gap-2" style={{ flexDirection: 'row' }}>
                  <input type="checkbox" checked={moodAdjustments.includes('remove_marketing_speak')} onChange={() => toggleMood('remove_marketing_speak')} />
                  <span>Marketing-Sprache entfernen</span>
                </label>
              </div>

              <div className="flex-row--center gap-2 flex-wrap">
                <button type="button" className="btn btn--secondary" onClick={() => void handleRefine()} disabled={triggerJob.isPending || moodAdjustments.length === 0}>
                  Anwenden & neu generieren
                </button>
                <button type="button" className="btn btn--ghost" data-testid="brand-prompt-preview" onClick={() => void handleLoadPromptPreview()} disabled={promptPreview.isPending}>
                  {promptPreview.isPending ? 'Lade Prompt…' : 'Prompt-Vorschau'}
                </button>
                {displayJob.status === 'completed' && !profile?.autoPublishEnabled && (
                  <button type="button" className="btn btn--primary btn--sm" onClick={() => void saveGeneratedPost(displayJob)} disabled={savingDraftId === displayJob.id}>
                    {savingDraftId === displayJob.id ? <Loader2 size={14} className="spin" /> : <Save size={14} />}
                    Entwurf speichern
                  </button>
                )}
                {onEditInComposer && displayJob.status === 'completed' && (
                  <button
                    type="button"
                    className="btn btn--secondary btn--sm"
                    onClick={() =>
                      onEditInComposer({
                        title: typeof displayJob.result?.title === 'string' ? displayJob.result.title : undefined,
                        content: String(displayJob.result?.content ?? ''),
                        targetAccountIds: selectedAccounts,
                      })
                    }
                  >
                    Im Composer bearbeiten
                  </button>
                )}
                <button type="button" className="btn btn--secondary" onClick={() => setStep(2)}>
                  <CalendarClock size={14} /> Neue Aufgabe
                </button>
              </div>

              {showPromptPreview && promptText && (
                <details open className="glass-panel glass-panel--compact">
                  <summary className="subsection-title" style={{ cursor: 'pointer' }}>Haupt-Prompt</summary>
                  <pre style={{ whiteSpace: 'pre-wrap', fontSize: '0.75rem', maxHeight: '24rem', overflow: 'auto' }}>{promptText}</pre>
                </details>
              )}
            </div>
          )}

          {!displayJob && activeJobs.length === 0 && (
            <div className="glass-panel">
              <p className="hint">Noch kein generierter Post. Starte in Schritt 2 eine Generierung.</p>
              <button type="button" className="btn btn--secondary" onClick={() => setStep(2)}>Zur Aufgabe</button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
