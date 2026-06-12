import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bot, Loader2, Send, Sparkles, X } from 'lucide-react'
import type { BackendAIChatEvent, BackendAIChatMention, BackendAIChatMessage, createApiClient } from '../../api'

type ApiClient = ReturnType<typeof createApiClient>

interface DraftPreview {
  id: string
  title: string
  content: string
  targetAccountIds: string[]
  scheduledAt?: string
}

interface ChatEntry {
  kind: 'user' | 'assistant' | 'tool' | 'preview' | 'error'
  text: string
  mentions?: BackendAIChatMention[]
  preview?: DraftPreview
}

interface MentionOption extends BackendAIChatMention {
  hint: string
}

interface SlashCommand {
  command: string
  textKey: string
}

const slashCommands: SlashCommand[] = [
  { command: '/draft', textKey: 'aiChat.commandDraft' },
  { command: '/campaign', textKey: 'aiChat.commandCampaign' },
  { command: '/recurring', textKey: 'aiChat.commandRecurring' },
  { command: '/rss', textKey: 'aiChat.commandRss' },
]

interface AIChatWidgetProps {
  api: ApiClient
  teamId: string
  onOpenInComposer: (payload: {
    title?: string
    content: string
    targetAccountIds: string[]
    scheduledAt?: string
  }) => void
}

export function AIChatWidget({ api, teamId, onOpenInComposer }: AIChatWidgetProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [entries, setEntries] = useState<ChatEntry[]>([])
  const [input, setInput] = useState('')
  const [pendingMentions, setPendingMentions] = useState<BackendAIChatMention[]>([])
  const [busy, setBusy] = useState(false)
  const [mentionOptions, setMentionOptions] = useState<MentionOption[]>([])
  const [suggestions, setSuggestions] = useState<{ kind: 'mention' | 'command'; query: string } | null>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const abortRef = useRef<AbortController | null>(null)

  // History sent to the model: user/assistant turns only.
  const history = useRef<BackendAIChatMessage[]>([])

  useEffect(() => {
    // Reset the conversation when the team changes.
    history.current = []
    setEntries([])
    setPendingMentions([])
    setInput('')
  }, [teamId])

  useEffect(() => () => abortRef.current?.abort(), [])

  useEffect(() => {
    if (!open) {
      return
    }
    let cancelled = false
    void (async () => {
      const options: MentionOption[] = []
      const [campaigns, templates, feeds] = await Promise.allSettled([
        api.listCampaignFormats(teamId),
        api.listPostTemplates(teamId),
        api.listRSSFeeds(teamId),
      ])
      if (campaigns.status === 'fulfilled') {
        for (const item of campaigns.value.items ?? []) {
          options.push({ type: 'campaign', id: item.id, name: item.name, hint: t('aiChat.mentionCampaign') })
        }
      }
      if (templates.status === 'fulfilled') {
        for (const item of templates.value.items ?? []) {
          options.push({ type: 'recurring', id: item.id, name: item.title, hint: t('aiChat.mentionRecurring') })
        }
      }
      if (feeds.status === 'fulfilled') {
        for (const item of feeds.value.items ?? []) {
          options.push({ type: 'rss', id: item.id, name: item.name, hint: t('aiChat.mentionRss') })
        }
      }
      if (!cancelled) {
        setMentionOptions(options)
      }
    })()
    return () => {
      cancelled = true
    }
  }, [api, teamId, open, t])

  useEffect(() => {
    listRef.current?.scrollTo({ top: listRef.current.scrollHeight })
  }, [entries, busy])

  const filteredSuggestions = useMemo(() => {
    if (!suggestions) {
      return []
    }
    const query = suggestions.query.toLowerCase()
    if (suggestions.kind === 'command') {
      return slashCommands
        .filter((cmd) => cmd.command.slice(1).startsWith(query))
        .map((cmd) => ({ key: cmd.command, label: cmd.command, hint: t(cmd.textKey), apply: () => applyCommand(cmd) }))
    }
    return mentionOptions
      .filter((option) => option.name.toLowerCase().includes(query))
      .slice(0, 6)
      .map((option) => ({
        key: `${option.type}:${option.id}`,
        label: `@${option.name}`,
        hint: option.hint,
        apply: () => applyMention(option),
      }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [suggestions, mentionOptions, t])

  function detectSuggestions(value: string) {
    const beforeCursor = value
    const mentionMatch = /(?:^|\s)@([\w-]*)$/.exec(beforeCursor)
    if (mentionMatch) {
      setSuggestions({ kind: 'mention', query: mentionMatch[1] ?? '' })
      return
    }
    const commandMatch = /^\/([\w-]*)$/.exec(beforeCursor)
    if (commandMatch) {
      setSuggestions({ kind: 'command', query: commandMatch[1] ?? '' })
      return
    }
    setSuggestions(null)
  }

  function applyMention(option: MentionOption) {
    setInput((current) => current.replace(/@([\w-]*)$/, `@${option.name} `))
    setPendingMentions((current) => {
      if (current.some((mention) => mention.type === option.type && mention.id === option.id)) {
        return current
      }
      return [...current, { type: option.type, id: option.id, name: option.name }]
    })
    setSuggestions(null)
    inputRef.current?.focus()
  }

  function applyCommand(cmd: SlashCommand) {
    setInput(t(cmd.textKey))
    setSuggestions(null)
    inputRef.current?.focus()
  }

  function removeMention(mention: BackendAIChatMention) {
    setPendingMentions((current) => current.filter((item) => !(item.type === mention.type && item.id === mention.id)))
  }

  const handleEvent = useCallback(
    (event: BackendAIChatEvent) => {
      switch (event.type) {
        case 'message': {
          const text = event.message ?? ''
          if (!text) {
            return
          }
          history.current = [...history.current, { role: 'assistant', content: text }]
          setEntries((current) => [...current, { kind: 'assistant', text }])
          break
        }
        case 'tool_call':
          setEntries((current) => [
            ...current,
            { kind: 'tool', text: t('aiChat.toolRunning', { tool: toolLabel(event.tool_name ?? '', t) }) },
          ])
          break
        case 'tool_result': {
          if (event.tool_name === 'create_draft' && event.payload && typeof event.payload === 'object') {
            const post = event.payload as {
              id?: string
              title?: string
              content?: string
              target_accounts?: string[]
              scheduled_at?: string
            }
            if (post.content) {
              setEntries((current) => [
                ...current,
                {
                  kind: 'preview',
                  text: '',
                  preview: {
                    id: post.id ?? '',
                    title: post.title ?? '',
                    content: post.content ?? '',
                    targetAccountIds: post.target_accounts ?? [],
                    scheduledAt: post.scheduled_at,
                  },
                },
              ])
            }
            return
          }
          if (event.message?.startsWith('Error:')) {
            setEntries((current) => [...current, { kind: 'error', text: event.message ?? '' }])
          }
          break
        }
        case 'error':
          setEntries((current) => [...current, { kind: 'error', text: event.message ?? t('aiChat.genericError') }])
          break
        default:
          break
      }
    },
    [t],
  )

  async function send() {
    const text = input.trim()
    if (!text || busy) {
      return
    }
    const mentions = pendingMentions.filter((mention) => text.includes(`@${mention.name}`))
    const userMessage: BackendAIChatMessage = {
      role: 'user',
      content: text,
      mentions: mentions.length > 0 ? mentions : undefined,
    }
    history.current = [...history.current, userMessage]
    setEntries((current) => [...current, { kind: 'user', text, mentions }])
    setInput('')
    setPendingMentions([])
    setSuggestions(null)
    setBusy(true)
    const controller = new AbortController()
    abortRef.current = controller
    try {
      await api.streamAIChat(teamId, history.current, handleEvent, controller.signal)
    } catch (cause) {
      if (!controller.signal.aborted) {
        const message = cause instanceof Error ? cause.message : String(cause)
        setEntries((current) => [...current, { kind: 'error', text: message || t('aiChat.genericError') }])
      }
    } finally {
      setBusy(false)
      abortRef.current = null
    }
  }

  return (
    <>
      <button
        type="button"
        className="ai-chat-fab"
        onClick={() => setOpen((value) => !value)}
        aria-label={t('aiChat.title')}
        data-testid="ai-chat-fab"
      >
        {open ? <X size={22} /> : <Sparkles size={22} />}
      </button>

      {open && (
        <section className="ai-chat-drawer glass-panel" aria-label={t('aiChat.title')}>
          <header className="ai-chat-drawer__header">
            <Bot size={18} />
            <div>
              <strong>{t('aiChat.title')}</strong>
              <p className="hint">{t('aiChat.subtitle')}</p>
            </div>
            <button type="button" className="icon-button" onClick={() => setOpen(false)} aria-label={t('common.close')}>
              <X size={16} />
            </button>
          </header>

          <div className="ai-chat-drawer__messages" ref={listRef}>
            {entries.length === 0 && (
              <div className="ai-chat-empty">
                <Sparkles size={20} />
                <p>{t('aiChat.emptyState')}</p>
                <p className="hint">{t('aiChat.emptyStateHint')}</p>
              </div>
            )}
            {entries.map((entry, index) => {
              if (entry.kind === 'preview' && entry.preview) {
                return (
                  <article key={index} className="ai-chat-preview">
                    <p className="eyebrow">{t('aiChat.draftCreated')}</p>
                    {entry.preview.title ? <strong>{entry.preview.title}</strong> : null}
                    <p className="ai-chat-preview__content">{entry.preview.content}</p>
                    <button
                      type="button"
                      className="btn btn--secondary btn--sm"
                      onClick={() =>
                        onOpenInComposer({
                          title: entry.preview?.title,
                          content: entry.preview?.content ?? '',
                          targetAccountIds: entry.preview?.targetAccountIds ?? [],
                          scheduledAt: entry.preview?.scheduledAt,
                        })
                      }
                    >
                      {t('aiChat.openInComposer')}
                    </button>
                  </article>
                )
              }
              return (
                <div key={index} className={`ai-chat-bubble ai-chat-bubble--${entry.kind}`}>
                  {entry.text}
                </div>
              )
            })}
            {busy && (
              <div className="ai-chat-bubble ai-chat-bubble--tool">
                <Loader2 size={14} className="spin" /> {t('aiChat.thinking')}
              </div>
            )}
          </div>

          {pendingMentions.length > 0 && (
            <div className="ai-chat-mentions">
              {pendingMentions.map((mention) => (
                <button
                  key={`${mention.type}:${mention.id}`}
                  type="button"
                  className="ai-chat-chip"
                  onClick={() => removeMention(mention)}
                  title={t('aiChat.removeMention')}
                >
                  @{mention.name} ×
                </button>
              ))}
            </div>
          )}

          {filteredSuggestions.length > 0 && (
            <div className="ai-chat-suggestions">
              {filteredSuggestions.map((suggestion) => (
                <button key={suggestion.key} type="button" onClick={suggestion.apply}>
                  <span>{suggestion.label}</span>
                  <span className="hint">{suggestion.hint}</span>
                </button>
              ))}
            </div>
          )}

          <footer className="ai-chat-drawer__input">
            <textarea
              ref={inputRef}
              rows={2}
              value={input}
              placeholder={t('aiChat.inputPlaceholder')}
              onChange={(event) => {
                setInput(event.target.value)
                detectSuggestions(event.target.value)
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                  event.preventDefault()
                  void send()
                }
              }}
              disabled={busy}
            />
            <button
              type="button"
              className="btn btn--primary btn--sm"
              onClick={() => void send()}
              disabled={busy || !input.trim()}
              aria-label={t('aiChat.send')}
            >
              <Send size={16} />
            </button>
          </footer>
        </section>
      )}
    </>
  )
}

function toolLabel(toolName: string, t: (key: string) => string): string {
  switch (toolName) {
    case 'create_draft':
      return t('aiChat.toolCreateDraft')
    case 'create_campaign':
      return t('aiChat.toolCreateCampaign')
    case 'create_recurring_automation':
      return t('aiChat.toolCreateRecurring')
    case 'create_rss_automation':
      return t('aiChat.toolCreateRss')
    default:
      return toolName
  }
}
