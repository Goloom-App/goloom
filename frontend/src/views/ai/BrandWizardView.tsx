import { Fragment, useEffect, useMemo, useState } from 'react'
import {
  Bot,
  CalendarClock,
  Check,
  ChevronLeft,
  ChevronRight,
  Edit,
  FileText,
  Globe,
  Languages,
  Loader2,
  MessageCircle,
  Minus,
  Play,
  Plus,
  Save,
  Send,
  Sparkles,
  Target,
  Trash2,
  Wand2,
  X,
  Zap,
} from 'lucide-react'

import { ActionBar, OptionPill, SectionCard, Segmented, TagInput, ToggleSwitch } from '../../components/ui'

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

const STEPS: { id: WizardStep; title: string; caption: string }[] = [
  { id: 1, title: 'Setup', caption: 'Brand-Profil' },
  { id: 2, title: 'Aufgabe', caption: 'Anlass & Format' },
  { id: 3, title: 'Editor', caption: 'Feinschliff' },
]

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

function knowledgeIcon(type: 'text' | 'url' | 'file') {
  if (type === 'url') return <Globe size={16} />
  if (type === 'file') return <FileText size={16} />
  return <FileText size={16} />
}

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

  // Language DNA
  const [sentenceStyle, setSentenceStyle] = useState('')
  const [humorStyle, setHumorStyle] = useState('')
  const [preferredWords, setPreferredWords] = useState<string[]>([])
  const [signaturePhrases, setSignaturePhrases] = useState<string[]>([])
  const [bannedWords, setBannedWords] = useState<string[]>([])
  const [antiAiOverride, setAntiAiOverride] = useState(false)

  // Reach
  const [hookStyle, setHookStyle] = useState('')
  const [ctaFocus, setCtaFocus] = useState('')

  const [preferredLanguage, setPreferredLanguage] = useState('de')
  const [maxHashtags, setMaxHashtags] = useState(3)
  const [autoPublishEnabled, setAutoPublishEnabled] = useState(false)

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

  // Profile assistant
  const [assistantBrief, setAssistantBrief] = useState('')
  const [assistantJobId, setAssistantJobId] = useState<string | null>(null)
  const [assistantOpen, setAssistantOpen] = useState(false)

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
    return jobs?.find(
      (j) =>
        j.type === 'voice_engine' &&
        (j.status === 'completed' || j.status === 'processing' || j.status === 'pending'),
    )
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
        // Vibe preview is optional if the AI service isn't configured.
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
    if (occasionType === 'url') params.source_url = occasion.trim()
    if (occasionType === 'rss') params.rss_feed_url = occasion.trim()
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
      if (response.jobId) setActiveJobId(response.jobId)
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
    <div className="brand-wizard" data-testid="brand-wizard-view">
      {/* Stepper */}
      <nav className="brand-stepper" aria-label="Wizard-Schritte">
        {STEPS.map((s, idx) => {
          const state = step === s.id ? 'active' : step > s.id ? 'done' : 'todo'
          return (
            <Fragment key={s.id}>
              <button
                type="button"
                className={`brand-stepper__item${state === 'active' ? ' brand-stepper__item--active' : ''}${state === 'done' ? ' brand-stepper__item--done' : ''}`}
                onClick={() => setStep(s.id)}
                aria-current={state === 'active'}
              >
                <span className="brand-stepper__num">
                  {state === 'done' ? <Check size={14} /> : s.id}
                </span>
                <span className="brand-stepper__label">
                  <span className="brand-stepper__title">{s.title}</span>
                  <span className="brand-stepper__caption">{s.caption}</span>
                </span>
              </button>
              {idx < STEPS.length - 1 ? <span className="brand-stepper__divider" /> : null}
            </Fragment>
          )
        })}
      </nav>

      {(error || statusMessage) && (
        <div className="status-banner-panel" style={{ padding: '0.75rem 1rem' }}>
          {statusMessage && (
            <span className="status-banner__success" data-testid="brand-status-success">
              {statusMessage}
            </span>
          )}
          {error && (
            <span className="status-banner__error" data-testid="brand-status-error">
              {error}
            </span>
          )}
        </div>
      )}

      {/* ============================================================ */}
      {/* STEP 1 — SETUP                                                */}
      {/* ============================================================ */}
      {step === 1 && (
        <>
          {/* AI Assistant — hero card */}
          <SectionCard
            hero
            icon={<Wand2 size={18} />}
            title="KI-Assistent"
            subtitle="Beschreibe in 2–4 Sätzen, wer du bist und für wen du postest. Die KI füllt das Profil mit einem konkreten Vorschlag aus — alles bleibt editierbar."
            testId="brand-assistant-panel"
            headerExtra={
              <button
                type="button"
                className="btn btn--secondary btn--sm"
                data-testid="brand-assistant-toggle"
                onClick={() => setAssistantOpen((v) => !v)}
              >
                {assistantOpen ? (
                  <>
                    <Minus size={14} /> Schließen
                  </>
                ) : (
                  <>
                    <Sparkles size={14} /> Vorschlag generieren
                  </>
                )}
              </button>
            }
          >
            {assistantOpen && (
              <>
                <textarea
                  data-testid="brand-assistant-brief"
                  rows={4}
                  value={assistantBrief}
                  onChange={(e) => setAssistantBrief(e.target.value)}
                  placeholder='z. B. „Wir sind ein Selfhosting-Podcast für Anfänger, sprechen über Heimserver, Home-Assistant und Datenschutz. Zielgruppe: Tech-Nerds, leicht zynisch."'
                />
                <div>
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
                        <Sparkles size={14} /> Profil vorschlagen
                      </>
                    )}
                  </button>
                </div>
              </>
            )}
          </SectionCard>

          {/* Identity */}
          <SectionCard
            icon={<Bot size={18} />}
            title="Identität"
            subtitle="Wer steht hinter dem Account? Persona und Archetyp prägen den Vibe stärker als jede Tonalitätsangabe."
          >
            <div className="brand-card__grid">
              <label className="field">
                <span>Archetyp</span>
                <input
                  data-testid="brand-archetype"
                  value={archetype}
                  onChange={(e) => setArchetype(e.target.value)}
                  placeholder="z. B. Tech-Podcast, Zahnarztpraxis, Werbeagentur"
                />
                <p className="brand-field__hint">Kurzes Label, das deinen Account einordnet.</p>
              </label>
              <label className="field">
                <span>Branche / Kontext</span>
                <input
                  data-testid="brand-industry"
                  value={industry}
                  onChange={(e) => setIndustry(e.target.value)}
                  placeholder="z. B. Open-Source Hosting, Zahnmedizin, B2B-Marketing"
                />
              </label>
            </div>
            <label className="field">
              <span>Voice-Persona</span>
              <textarea
                data-testid="brand-persona"
                rows={2}
                value={persona}
                onChange={(e) => setPersona(e.target.value)}
                placeholder='z. B. „Maximilian, 38, IT-Nerd mit Selfhosting-Spleen, redet wie mit Kollegen am Stehtisch."'
              />
              <p className="brand-field__hint">Wer schreibt? Die Person hinter dem Account.</p>
            </label>
            <div className="brand-card__grid">
              <label className="field">
                <span>Haupt-Mehrwert</span>
                <input
                  data-testid="brand-main-value"
                  value={mainValue}
                  onChange={(e) => setMainValue(e.target.value)}
                  placeholder="Was bietest du einzigartig? Konkret, kein Marketing."
                />
              </label>
              <label className="field">
                <span>Zielgruppe</span>
                <input
                  data-testid="brand-audience"
                  value={targetAudience}
                  onChange={(e) => setTargetAudience(e.target.value)}
                  placeholder='z. B. „Patienten mit Zahnarzt-Angst" oder „Hobby-Sysadmins über 30"'
                />
              </label>
            </div>
          </SectionCard>

          {/* Language DNA */}
          <SectionCard
            icon={<MessageCircle size={18} />}
            title="Sprach-DNA"
            subtitle="Wie redest du? Freitext — keine starren Kategorien. Beschreibe so präzise wie möglich."
          >
            <div className="brand-card__grid">
              <label className="field">
                <span>Satzbau</span>
                <textarea
                  data-testid="brand-sentence-style"
                  rows={2}
                  value={sentenceStyle}
                  onChange={(e) => setSentenceStyle(e.target.value)}
                  placeholder='z. B. „Kurze Sätze, gerne Halbsätze. Kein Verb-am-Ende-Drama."'
                />
              </label>
              <label className="field">
                <span>Humor / Ton</span>
                <textarea
                  data-testid="brand-humor"
                  rows={2}
                  value={humorStyle}
                  onChange={(e) => setHumorStyle(e.target.value)}
                  placeholder='z. B. „Trocken mit IT-Insider-Witzen" oder „Warm und beruhigend"'
                />
              </label>
            </div>

            <div className="field">
              <span>Bevorzugte Wörter / Fachbegriffe</span>
              <TagInput
                values={preferredWords}
                onChange={setPreferredWords}
                placeholder="z. B. Heimserver, Pi, Docker"
                testId="brand-preferred-word"
              />
            </div>

            <div className="field">
              <span>Signature-Phrasen</span>
              <TagInput
                values={signaturePhrases}
                onChange={setSignaturePhrases}
                placeholder='z. B. „läuft auf meinem Pi"'
                testId="brand-signature-phrase"
              />
              <p className="brand-field__hint">
                Wiederkehrende Wendungen, die deinen Account erkennbar machen. Werden nur eingesetzt, wenn sie passen.
              </p>
            </div>

            <div className="field">
              <span>Zusätzlich verbotene Wörter</span>
              <TagInput
                values={bannedWords}
                onChange={setBannedWords}
                placeholder="z. B. branchenspezifische Hype-Wörter"
                testId="brand-banned-word"
              />
              <p className="brand-field__hint">
                Goloom blockt bereits typische KI-Phrasen („tauche ein", „spannend", „game-changer" …). Hier ergänzt du eigene.
              </p>
            </div>

            <ToggleSwitch
              checked={antiAiOverride}
              onChange={setAntiAiOverride}
              testId="brand-anti-ai-override"
              title="Standard-KI-Phrasen-Block deaktivieren"
              description="Nicht empfohlen. Die KI-Sprach-Defaults verhindern typische LLM-Klischees. Nur deaktivieren, wenn du sehr genau weißt, was du tust."
            />
          </SectionCard>

          {/* Reach strategy */}
          <SectionCard
            icon={<Target size={18} />}
            title="Reach-Strategie"
            subtitle="Wie öffnest du Posts und wozu rufst du auf? Frei beschreiben."
          >
            <div className="brand-card__grid">
              <label className="field">
                <span>Hook-Stil</span>
                <textarea
                  data-testid="brand-hook"
                  rows={2}
                  value={hookStyle}
                  onChange={(e) => setHookStyle(e.target.value)}
                  placeholder='z. B. „Mit einer konkreten Beobachtung einsteigen, nie Floskeln."'
                />
              </label>
              <label className="field">
                <span>CTA-Fokus</span>
                <textarea
                  data-testid="brand-cta"
                  rows={2}
                  value={ctaFocus}
                  onChange={(e) => setCtaFocus(e.target.value)}
                  placeholder='z. B. „Zum Kommentar einladen, kein Verkaufslink."'
                />
              </label>
            </div>
            <div className="brand-card__grid">
              <label className="field">
                <span>Sprache</span>
                <select value={preferredLanguage} onChange={(e) => setPreferredLanguage(e.target.value)}>
                  <option value="de">Deutsch</option>
                  <option value="en">English</option>
                </select>
              </label>
              <label className="field">
                <span>Max. Hashtags</span>
                <input
                  type="number"
                  min={0}
                  max={30}
                  value={maxHashtags}
                  onChange={(e) => setMaxHashtags(parseInt(e.target.value, 10) || 0)}
                />
              </label>
            </div>
            <ToggleSwitch
              checked={autoPublishEnabled}
              onChange={setAutoPublishEnabled}
              title="Auto-Publish"
              description="Generierte Posts werden direkt veröffentlicht statt im Review-Queue zu landen."
            />
          </SectionCard>

          {/* Knowledge base */}
          <SectionCard
            icon={<FileText size={18} />}
            title="Knowledge-Base"
            subtitle="Exklusive Faktenquelle. Die KI darf nichts erfinden, was hier nicht steht."
            testId="brand-knowledge-section"
          >
            {(knowledgeSources ?? []).length > 0 ? (
              <div className="brand-knowledge-list">
                {(knowledgeSources ?? []).map((source) => (
                  <div key={source.id} className="brand-knowledge-item">
                    <span className="brand-knowledge-item__icon">{knowledgeIcon(source.type)}</span>
                    <div className="brand-knowledge-item__body">
                      <span className="brand-knowledge-item__name">{source.name}</span>
                      <span className="brand-knowledge-item__meta">
                        {source.type} · {source.content.slice(0, 100)}
                        {source.content.length > 100 ? '…' : ''}
                      </span>
                    </div>
                    <button
                      type="button"
                      className="btn btn--ghost btn--icon-sm"
                      aria-label={`${source.name} entfernen`}
                      onClick={() => void deleteKnowledge.mutateAsync({ teamId: team.id, sourceId: source.id })}
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                ))}
              </div>
            ) : (
              <div className="brand-knowledge-empty">Noch keine Quellen — füge Texte oder URLs hinzu.</div>
            )}

            <div className="field">
              <span>Typ</span>
              <Segmented
                value={kbType}
                options={[
                  { id: 'text', label: 'Text / Transkript' },
                  { id: 'url', label: 'Website-URL' },
                ]}
                onChange={(v) => setKbType(v)}
                testIdPrefix="brand-kb-type"
              />
            </div>
            <label className="field">
              <span>Name</span>
              <input
                value={kbName}
                onChange={(e) => setKbName(e.target.value)}
                placeholder="z. B. Produkt-FAQ"
              />
            </label>
            {kbType === 'url' ? (
              <label className="field">
                <span>URL</span>
                <input value={kbUrl} onChange={(e) => setKbUrl(e.target.value)} placeholder="https://…" />
              </label>
            ) : (
              <label className="field">
                <span>Inhalt</span>
                <textarea
                  rows={4}
                  value={kbContent}
                  onChange={(e) => setKbContent(e.target.value)}
                  placeholder="Fakten, Zitate, Produktinfos…"
                />
              </label>
            )}
            <div>
              <button
                type="button"
                className="btn btn--secondary"
                onClick={() => void handleAddKnowledge()}
                disabled={createKnowledge.isPending || !kbName.trim()}
              >
                <Plus size={14} /> Wissensquelle hinzufügen
              </button>
            </div>
          </SectionCard>

          {/* Vibe preview */}
          {vibeSummary && (
            <div className="brand-vibe" data-testid="brand-vibe-preview">
              <Sparkles size={20} className="brand-vibe__icon" />
              <div className="brand-vibe__text">
                {vibeSummary}
                {vibeSuggestion ? <div className="brand-vibe__suggestion">{vibeSuggestion}</div> : null}
              </div>
            </div>
          )}

          <ActionBar
            right={
              <>
                <button
                  type="button"
                  className="btn btn--secondary"
                  data-testid="brand-save-setup"
                  onClick={() => void handleSaveSetup()}
                  disabled={upsertProfile.isPending}
                >
                  {upsertProfile.isPending ? (
                    <>
                      <Loader2 size={14} className="spin" /> Speichern…
                    </>
                  ) : (
                    <>
                      <Save size={14} /> Profil speichern
                    </>
                  )}
                </button>
                <button type="button" className="btn btn--primary" onClick={() => setStep(2)}>
                  Weiter <ChevronRight size={14} />
                </button>
              </>
            }
          />
        </>
      )}

      {/* ============================================================ */}
      {/* STEP 2 — TASK                                                 */}
      {/* ============================================================ */}
      {step === 2 && (
        <>
          <SectionCard
            icon={<Zap size={18} />}
            title="Was ist der Anlass?"
            subtitle="Quelle, Format und Ziel-Konten festlegen — das Brand-Profil aus Schritt 1 liefert den Vibe."
          >
            <div className="field">
              <span>Quelle</span>
              <Segmented
                value={occasionType}
                options={OCCASION_TYPES}
                onChange={(v) => setOccasionType(v)}
                testIdPrefix="brand-occasion-type"
              />
            </div>
            <label className="field">
              <span>{occasionType === 'text' ? 'Beschreibung des Anlasses' : occasionType === 'url' ? 'URL' : 'RSS-Feed-Link'}</span>
              <textarea
                data-testid="brand-occasion"
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
                testIdPrefix="brand-output-format"
              />
            </div>
            <div className="field">
              <span>Ziel-Konten</span>
              <DestinationPicker
                accounts={accounts}
                selectedIds={selectedAccounts}
                onToggle={toggleAccount}
                testIdPrefix="brand-dest"
              />
            </div>
          </SectionCard>

          <ActionBar
            left={
              <button type="button" className="btn btn--secondary" onClick={() => setStep(1)}>
                <ChevronLeft size={14} /> Zurück
              </button>
            }
            right={
              <button
                type="button"
                className="btn btn--primary"
                data-testid="brand-generate"
                onClick={() => void handleGenerate()}
                disabled={triggerJob.isPending}
              >
                {triggerJob.isPending ? (
                  <>
                    <Loader2 size={14} className="spin" /> Generiere…
                  </>
                ) : (
                  <>
                    <Play size={14} /> Generieren
                  </>
                )}
              </button>
            }
          />
        </>
      )}

      {/* ============================================================ */}
      {/* STEP 3 — EDITOR                                               */}
      {/* ============================================================ */}
      {step === 3 && (
        <>
          {activeJobs.length > 0 && (
            <SectionCard icon={<Loader2 size={18} className="spin" />} title="Generierung läuft …" subtitle="Du kannst den Job jederzeit abbrechen.">
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
              testId="brand-editor"
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
                      testId={`brand-mood-${option.id}`}
                    >
                      {option.label}
                    </OptionPill>
                  ))}
                </div>
                <p className="brand-field__hint">Wähle aus, was angepasst werden soll, und klicke „Neu generieren".</p>
              </div>

              {showPromptPreview && promptText && (
                <details open className="brand-prompt-preview" data-testid="brand-prompt-preview-panel">
                  <summary>
                    <FileText size={14} /> Haupt-Prompt
                  </summary>
                  <pre>{promptText}</pre>
                </details>
              )}

              <ActionBar
                left={
                  <>
                    <button type="button" className="btn btn--ghost" onClick={() => setStep(2)}>
                      <CalendarClock size={14} /> Neue Aufgabe
                    </button>
                    <button
                      type="button"
                      className="btn btn--ghost"
                      data-testid="brand-prompt-preview"
                      onClick={() => void handleLoadPromptPreview()}
                      disabled={promptPreview.isPending}
                    >
                      {promptPreview.isPending ? (
                        <>
                          <Loader2 size={14} className="spin" /> Lade…
                        </>
                      ) : (
                        <>
                          <FileText size={14} /> Prompt-Vorschau
                        </>
                      )}
                    </button>
                  </>
                }
                right={
                  <>
                    <button
                      type="button"
                      className="btn btn--secondary"
                      onClick={() => void handleRefine()}
                      disabled={triggerJob.isPending || moodAdjustments.length === 0}
                    >
                      <Sparkles size={14} /> Neu generieren
                    </button>
                    {onEditInComposer && displayJob.status === 'completed' && (
                      <button
                        type="button"
                        className="btn btn--secondary"
                        onClick={() =>
                          onEditInComposer({
                            title:
                              typeof displayJob.result?.title === 'string' ? displayJob.result.title : undefined,
                            content: String(displayJob.result?.content ?? ''),
                            targetAccountIds: selectedAccounts,
                          })
                        }
                      >
                        <Edit size={14} /> Im Composer öffnen
                      </button>
                    )}
                    {displayJob.status === 'completed' && !profile?.autoPublishEnabled && (
                      <button
                        type="button"
                        className="btn btn--primary"
                        onClick={() => void saveGeneratedPost(displayJob)}
                        disabled={savingDraftId === displayJob.id}
                      >
                        {savingDraftId === displayJob.id ? (
                          <>
                            <Loader2 size={14} className="spin" /> Speichern…
                          </>
                        ) : (
                          <>
                            <Send size={14} /> Als Entwurf speichern
                          </>
                        )}
                      </button>
                    )}
                  </>
                }
              />
            </SectionCard>
          )}

          {!displayJob && activeJobs.length === 0 && (
            <SectionCard
              icon={<Languages size={18} />}
              title="Noch kein Post generiert"
              subtitle="Starte in Schritt 2 eine Generierung — das Ergebnis erscheint hier."
            >
              <div>
                <button type="button" className="btn btn--primary" onClick={() => setStep(2)}>
                  <ChevronLeft size={14} /> Zur Aufgabe
                </button>
              </div>
            </SectionCard>
          )}
        </>
      )}
    </div>
  )
}
