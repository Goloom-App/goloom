import { useEffect, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import {
  Bot,
  Eye,
  FileText,
  Globe,
  Loader2,
  MessageCircle,
  Minus,
  Plus,
  RefreshCw,
  Save,
  Sparkles,
  Target,
  Trash2,
  Wand2,
  X,
} from 'lucide-react'

import { ActionBar, SectionCard, Segmented, TagInput, ToggleSwitch } from '../../components/ui'
import {
  useTeamProfile,
  useUpsertTeamProfile,
  useKnowledgeSources,
  useCreateKnowledgeSource,
  useUpdateKnowledgeSource,
  useDeleteKnowledgeSource,
  useTriggerAIJob,
  useAIJobs,
  useAIPromptPreview,
  useStyleExamples,
  useCreateStyleExample,
  useDeleteStyleExample,
} from '../../hooks/useAI'
import { useAIJobStream } from '../../hooks/useSSE'
import type { KnowledgeSource, TeamRecord } from '../../types'

interface BrandProfileViewProps {
  team: TeamRecord
}

type ProfileTab = 'profile' | 'examples'

function knowledgeIcon(type: 'text' | 'url' | 'file') {
  if (type === 'url') return <Globe size={16} />
  return <FileText size={16} />
}

function knowledgePreview(source: KnowledgeSource): string {
  const snippet = source.content.trim().slice(0, 140)
  if (source.type === 'url') {
    const url = source.sourceUrl?.trim() || 'URL'
    if (!snippet) return url
    return `${url} · ${snippet}${source.content.length > 140 ? '…' : ''}`
  }
  return snippet + (source.content.length > 140 ? '…' : '')
}

export function BrandProfileView({ team }: BrandProfileViewProps) {
  const { data: profile, isLoading } = useTeamProfile(team.id)
  const { data: knowledgeSources } = useKnowledgeSources(team.id)
  const { data: styleExamples, isLoading: examplesLoading } = useStyleExamples(team.id)
  const { data: jobs } = useAIJobs(team.id)
  const upsertProfile = useUpsertTeamProfile()
  const createKnowledge = useCreateKnowledgeSource()
  const updateKnowledge = useUpdateKnowledgeSource()
  const deleteKnowledge = useDeleteKnowledgeSource()
  const createExample = useCreateStyleExample()
  const deleteExample = useDeleteStyleExample()
  const triggerJob = useTriggerAIJob()
  const promptPreview = useAIPromptPreview()
  useAIJobStream(team.id)

  const [activeTab, setActiveTab] = useState<ProfileTab>('profile')
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
  const [formattingRules, setFormattingRules] = useState<string[]>([])
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

  // Vibe + brand prompt preview
  const [vibeSummary, setVibeSummary] = useState<string | null>(null)
  const [vibeSuggestion, setVibeSuggestion] = useState<string | null>(null)
  const [vibeJobId, setVibeJobId] = useState<string | null>(null)
  const [brandPromptText, setBrandPromptText] = useState<string | null>(null)

  // Profile assistant
  const [assistantBrief, setAssistantBrief] = useState('')
  const [assistantJobId, setAssistantJobId] = useState<string | null>(null)
  const [assistantOpen, setAssistantOpen] = useState(false)

  // Style examples
  const [exampleDialogOpen, setExampleDialogOpen] = useState(false)
  const [examplePlatform, setExamplePlatform] = useState('general')
  const [exampleContent, setExampleContent] = useState('')
  const [exampleNotes, setExampleNotes] = useState('')

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
    setFormattingRules(meta.formattingRules ?? [])
    setAntiAiOverride(Boolean(meta.languageDna?.antiAiOverride))
    setBannedWords(meta.bannedWords ?? [])
    setHookStyle(meta.reachStrategy?.hookStyle ?? '')
    setCtaFocus(meta.reachStrategy?.ctaFocus ?? '')
    setPreferredLanguage(meta.preferredLanguage || 'de')
    setMaxHashtags(meta.maxHashtags ?? 3)
    setAutoPublishEnabled(profile.autoPublishEnabled)
  }, [profile])

  // Watch for vibe preview completion
  useEffect(() => {
    if (!vibeJobId || !jobs) return
    const job = jobs.find((j) => j.id === vibeJobId)
    if (!job) return
    if (job.status === 'completed' && typeof job.result?.summary === 'string') {
      setVibeSummary(job.result.summary)
      setVibeSuggestion(typeof job.result.suggestion === 'string' ? job.result.suggestion : null)
      setVibeJobId(null)
    } else if (job.status === 'failed') {
      setVibeJobId(null)
    }
  }, [jobs, vibeJobId])

  // Watch for assistant completion
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
      if (Array.isArray(proposed.formatting_rules)) setFormattingRules((proposed.formatting_rules as unknown[]).map(String))
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
    formatting_rules: formattingRules,
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
        params: { brief: assistantBrief.trim(), language: preferredLanguage },
      })
      if (response.jobId) setAssistantJobId(response.jobId)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'KI-Assistent fehlgeschlagen')
    }
  }

  const handleSave = async () => {
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
        if (response.jobId) setVibeJobId(response.jobId)
      } catch {
        // Vibe preview optional
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Speichern fehlgeschlagen')
    }
  }

  const handleRefetchKnowledge = async (source: KnowledgeSource) => {
    if (source.type !== 'url' || !source.sourceUrl?.trim()) return
    setError(null)
    try {
      await updateKnowledge.mutateAsync({
        teamId: team.id,
        sourceId: source.id,
        data: {
          type: 'url',
          name: source.name,
          source_url: source.sourceUrl,
          content: '',
        },
      })
      setStatusMessage(`„${source.name}" neu von URL geladen`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'URL-Inhalt konnte nicht neu geladen werden')
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

  const handleAddExample = async () => {
    if (!exampleContent.trim()) return
    setError(null)
    try {
      await createExample.mutateAsync({
        teamId: team.id,
        data: {
          platform: examplePlatform,
          content: exampleContent.trim(),
          notes: exampleNotes.trim(),
        },
      })
      setExampleContent('')
      setExampleNotes('')
      setExampleDialogOpen(false)
      setStatusMessage('Beispieltext hinzugefügt')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Beispieltext konnte nicht hinzugefügt werden')
    }
  }

  const handleDeleteExample = async (exampleId: string) => {
    setError(null)
    try {
      await deleteExample.mutateAsync({ teamId: team.id, exampleId })
      setStatusMessage('Beispieltext entfernt')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Beispieltext konnte nicht entfernt werden')
    }
  }

  const handleShowBrandPrompt = async () => {
    setError(null)
    try {
      const preview = await promptPreview.mutateAsync({
        teamId: team.id,
        params: {
          occasion: 'Beispiel-Post für die Brand-Prompt-Vorschau.',
          output_format: 'post',
          platform: 'mastodon',
        },
      })
      // The generation prompt already includes the full brand context — show that.
      setBrandPromptText(preview.system_prompt || preview.generation_prompt)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Prompt-Vorschau fehlgeschlagen')
    }
  }

  return (
    <div className="brand-wizard" data-testid="brand-profile-view">
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

      <div className="brand-profile-tabs">
        <Segmented<ProfileTab>
          value={activeTab}
          options={[
            { id: 'profile', label: 'Profil' },
            { id: 'examples', label: 'Beispieltexte' },
          ]}
          onChange={setActiveTab}
          testIdPrefix="brand-profile-tab"
        />
      </div>

      {activeTab === 'profile' && (
        <>
      {/* AI Assistant */}
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
            {assistantOpen ? <><Minus size={14} /> Schließen</> : <><Sparkles size={14} /> Vorschlag generieren</>}
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
                {assistantJobId ? <><Loader2 size={14} className="spin" /> KI denkt nach…</> : <><Sparkles size={14} /> Profil vorschlagen</>}
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
        subtitle="Wie redest du? Freitext — keine starren Kategorien. So präzise wie möglich."
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
          <TagInput values={preferredWords} onChange={setPreferredWords} placeholder="z. B. Heimserver, Pi, Docker" testId="brand-preferred-word" />
        </div>

        <div className="field">
          <span>Signature-Phrasen</span>
          <TagInput values={signaturePhrases} onChange={setSignaturePhrases} placeholder='z. B. „läuft auf meinem Pi"' testId="brand-signature-phrase" />
          <p className="brand-field__hint">Wiederkehrende Wendungen, die deinen Account erkennbar machen.</p>
        </div>

        <div className="field">
          <span>Formatting-Regeln</span>
          <TagInput
            values={formattingRules}
            onChange={setFormattingRules}
            placeholder="z. B. Max. 2 Emojis pro Post, kein Emoji pro Zeile"
            testId="brand-formatting-rule"
          />
          <p className="brand-field__hint">
            Konkrete Schreibregeln für Emoji, Satzbau und Stil — landen direkt im System-Prompt unter „Formatting rules".
          </p>
        </div>

        <div className="field">
          <span>Zusätzlich verbotene Wörter</span>
          <TagInput values={bannedWords} onChange={setBannedWords} placeholder="z. B. branchenspezifische Hype-Wörter" testId="brand-banned-word" />
          <p className="brand-field__hint">Goloom blockt bereits typische KI-Phrasen („tauche ein", „spannend", „game-changer" …).</p>
        </div>

        <ToggleSwitch
          checked={antiAiOverride}
          onChange={setAntiAiOverride}
          testId="brand-anti-ai-override"
          title="Standard-KI-Phrasen-Block deaktivieren"
          description="Nicht empfohlen. Verhindert typische LLM-Klischees. Nur deaktivieren, wenn du sehr genau weißt, was du tust."
        />
      </SectionCard>

      {/* Reach strategy */}
      <SectionCard
        icon={<Target size={18} />}
        title="Reach-Strategie"
        subtitle="Wie öffnest du Posts und wozu rufst du auf?"
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
          <div className="field">
            <span>Sprache</span>
            <Segmented<'de' | 'en'>
              value={preferredLanguage === 'en' ? 'en' : 'de'}
              options={[
                { id: 'de', label: 'Deutsch' },
                { id: 'en', label: 'English' },
              ]}
              onChange={(v) => setPreferredLanguage(v)}
              testIdPrefix="brand-language"
            />
          </div>
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
                  <span className="brand-knowledge-item__meta">{knowledgePreview(source)}</span>
                </div>
                <div className="brand-knowledge-item__actions">
                  {source.type === 'url' && source.sourceUrl ? (
                    <button
                      type="button"
                      className="btn btn--ghost btn--icon-sm"
                      data-testid={`brand-knowledge-refetch-${source.id}`}
                      aria-label={`${source.name} von URL neu laden`}
                      title="Von URL neu laden"
                      onClick={() => void handleRefetchKnowledge(source)}
                      disabled={updateKnowledge.isPending}
                    >
                      <RefreshCw size={14} />
                    </button>
                  ) : null}
                  <button
                    type="button"
                    className="btn btn--ghost btn--icon-sm"
                    aria-label={`${source.name} entfernen`}
                    onClick={() => void deleteKnowledge.mutateAsync({ teamId: team.id, sourceId: source.id })}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
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

      {/* Brand prompt preview */}
      <SectionCard
        icon={<Eye size={18} />}
        title="So sieht dein Brand im Prompt aus"
        subtitle="Verständnis-Layer: dieser System-Prompt wird bei jeder Generierung an die KI gesendet. Speichere zuerst Änderungen am Profil."
        headerExtra={
          <button
            type="button"
            className="btn btn--secondary btn--sm"
            data-testid="brand-show-prompt"
            onClick={() => void handleShowBrandPrompt()}
            disabled={promptPreview.isPending}
          >
            {promptPreview.isPending ? <><Loader2 size={14} className="spin" /> Lade…</> : <><Eye size={14} /> Brand-Prompt zeigen</>}
          </button>
        }
      >
        {brandPromptText ? (
          <details open className="brand-prompt-preview">
            <summary>
              <FileText size={14} /> System-Prompt
            </summary>
            <pre>{brandPromptText}</pre>
          </details>
        ) : (
          <p className="brand-field__hint" style={{ margin: 0 }}>
            Noch keine Vorschau geladen. „Brand-Prompt zeigen" oben rechts klicken.
          </p>
        )}
      </SectionCard>
        </>
      )}

      {activeTab === 'examples' && (
        <SectionCard
          icon={<FileText size={18} />}
          title="Beispieltexte"
          subtitle="Referenz-Posts für den System-Prompt. Die KI orientiert sich am Stil — nicht am exakten Wortlaut."
          testId="brand-examples-section"
          headerExtra={
            <Dialog.Root open={exampleDialogOpen} onOpenChange={setExampleDialogOpen}>
              <Dialog.Trigger asChild>
                <button type="button" className="btn btn--secondary btn--sm" data-testid="brand-add-example">
                  <Plus size={14} /> Beispiel hinzufügen
                </button>
              </Dialog.Trigger>
              <Dialog.Portal>
                <Dialog.Overlay className="dialog-overlay" />
                <Dialog.Content className="dialog-content" data-testid="brand-example-dialog">
                  <div className="drawer-header">
                    <Dialog.Title className="drawer-title">Beispieltext hinzufügen</Dialog.Title>
                    <Dialog.Close asChild>
                      <button type="button" className="btn btn--ghost btn--icon-sm" aria-label="Schließen">
                        <X size={20} />
                      </button>
                    </Dialog.Close>
                  </div>
                  <div className="drawer-body stack">
                    <label className="field">
                      <span>Plattform</span>
                      <select
                        data-testid="brand-example-platform"
                        value={examplePlatform}
                        onChange={(e) => setExamplePlatform(e.target.value)}
                      >
                        <option value="general">Allgemein</option>
                        <option value="mastodon">Mastodon</option>
                        <option value="bluesky">Bluesky</option>
                        <option value="friendica">Friendica</option>
                      </select>
                    </label>
                    <label className="field">
                      <span>Post-Text</span>
                      <textarea
                        data-testid="brand-example-content"
                        rows={6}
                        value={exampleContent}
                        onChange={(e) => setExampleContent(e.target.value)}
                        placeholder="Echten Post hier einfügen …"
                      />
                    </label>
                    <label className="field">
                      <span>Notiz (optional)</span>
                      <input
                        data-testid="brand-example-notes"
                        value={exampleNotes}
                        onChange={(e) => setExampleNotes(e.target.value)}
                        placeholder="z. B. Typischer Livestream-Post"
                      />
                    </label>
                    <div className="flex-row--end gap-2 mt-4">
                      <Dialog.Close asChild>
                        <button type="button" className="btn btn--ghost">
                          Abbrechen
                        </button>
                      </Dialog.Close>
                      <button
                        type="button"
                        className="btn btn--primary"
                        data-testid="brand-example-submit"
                        onClick={() => void handleAddExample()}
                        disabled={!exampleContent.trim() || createExample.isPending}
                      >
                        {createExample.isPending ? 'Speichern…' : 'Hinzufügen'}
                      </button>
                    </div>
                  </div>
                </Dialog.Content>
              </Dialog.Portal>
            </Dialog.Root>
          }
        >
          {examplesLoading ? (
            <p className="hint">Lade Beispieltexte…</p>
          ) : (styleExamples ?? []).length === 0 ? (
            <div className="brand-knowledge-empty">
              Noch keine Beispieltexte — füge echte Posts hinzu, die deinen Stil zeigen.
            </div>
          ) : (
            <div className="brand-examples-list">
              {(styleExamples ?? []).map((example) => (
                <div key={example.id} className="brand-example-item" data-testid={`brand-example-${example.id}`}>
                  <div className="brand-example-item__header">
                    <span className="brand-example-item__platform">{example.platform}</span>
                    <button
                      type="button"
                      className="btn btn--ghost btn--icon-sm"
                      aria-label="Beispieltext entfernen"
                      onClick={() => void handleDeleteExample(example.id)}
                      disabled={deleteExample.isPending}
                    >
                      <Trash2 size={14} />
                    </button>
                  </div>
                  <p className="brand-example-item__content">{example.content}</p>
                  {example.notes ? <p className="brand-example-item__notes">{example.notes}</p> : null}
                </div>
              ))}
            </div>
          )}
        </SectionCard>
      )}

      <ActionBar
        right={
          <button
            type="button"
            className="btn btn--primary"
            data-testid="brand-save-setup"
            onClick={() => void handleSave()}
            disabled={upsertProfile.isPending}
          >
            {upsertProfile.isPending ? <><Loader2 size={14} className="spin" /> Speichern…</> : <><Save size={14} /> Profil speichern</>}
          </button>
        }
      />
    </div>
  )
}
