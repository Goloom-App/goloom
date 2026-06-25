import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Bot, Loader2, Send, Sparkles, X } from 'lucide-react'
import type { BackendAIChatEvent, BackendAIChatMention, BackendAIChatMessage, createApiClient } from '../../api'
import type { AccountRecord } from '../../types'
import { composerContextEvent } from './composerChatBridge'
import type { ComposerChatContext, ComposerChatTarget } from './composerChatBridge'
import { getViewContext } from './viewContextBridge'

type ApiClient = ReturnType<typeof createApiClient>

interface DraftPreview {
  id: string
  title: string
  content: string
  targetAccountIds: string[]
  scheduledAt?: string
}

/** A composer revision proposed by revise_composer_post: applied back into the
 *  open composer (default text and/or per-account overrides), never persisted. */
interface RevisionPreview {
  content: string
  accountContentOverride: Record<string, string>
}

interface ChatEntry {
  kind: 'user' | 'assistant' | 'tool' | 'preview' | 'revision' | 'error'
  text: string
  mentions?: BackendAIChatMention[]
  preview?: DraftPreview
  revision?: RevisionPreview
  updated?: boolean
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
  teamAccounts: AccountRecord[]
  onOpenInComposer: (payload: {
    title?: string
    content: string
    targetAccountIds: string[]
    scheduledAt?: string
  }) => void
  /** Apply a revision into the open composer: default text and/or per-account overrides. */
  onApplyToComposer?: (payload: { content?: string; accountContentOverride?: Record<string, string> }) => void
}

export function AIChatWidget({ api, teamId, teamAccounts, onOpenInComposer, onApplyToComposer }: AIChatWidgetProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [entries, setEntries] = useState<ChatEntry[]>([])
  const [input, setInput] = useState('')
  const [pendingMentions, setPendingMentions] = useState<BackendAIChatMention[]>([])
  const [busy, setBusy] = useState(false)
  const [composerContext, setComposerContext] = useState<{ content: string; targets: ComposerChatTarget[] } | null>(null)
  // Account ids the user toggled out of scope: edits then only touch the rest.
  const [deselectedAccountIds, setDeselectedAccountIds] = useState<string[]>([])
  const [mentionOptions, setMentionOptions] = useState<MentionOption[]>([])
  const [suggestions, setSuggestions] = useState<{ kind: 'mention' | 'command'; query: string } | null>(null)
  // Keyboard-driven selection within the @mention / slash-command dropdown.
  const [activeSuggestion, setActiveSuggestion] = useState(0)
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
    setComposerContext(null)
    setDeselectedAccountIds([])
  }, [teamId])

  useEffect(() => {
    function onComposerContext(event: Event) {
      const detail = (event as CustomEvent).detail as ComposerChatContext | undefined
      if (!detail) {
        return
      }
      if (detail.clear) {
        setComposerContext(null)
        setDeselectedAccountIds([])
        return
      }
      if (!detail.content.trim()) {
        // No text yet (e.g. an empty composer just opened): don't attach a chip,
        // but if the previous context is now empty, drop it.
        setComposerContext((prev) => (prev ? null : prev))
        return
      }
      setComposerContext({ content: detail.content, targets: detail.targets ?? [] })
      // Manual triggers (a button) open and focus the chat; auto-sync stays silent.
      if (!detail.auto) {
        setOpen(true)
        setTimeout(() => inputRef.current?.focus(), 0)
      }
    }
    window.addEventListener(composerContextEvent, onComposerContext)
    return () => window.removeEventListener(composerContextEvent, onComposerContext)
  }, [])

  useEffect(() => () => abortRef.current?.abort(), [])

  useEffect(() => {
    if (!open) {
      return
    }
    let cancelled = false
    void (async () => {
      const options: MentionOption[] = []
      for (const account of teamAccounts) {
        options.push({ type: 'account', id: account.id, name: account.name, hint: t('aiChat.mentionAccount', { provider: account.provider }) })
      }
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
  }, [api, teamId, open, teamAccounts, t])

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

  // Keep the keyboard highlight in range as the suggestion list changes.
  useEffect(() => {
    setActiveSuggestion(0)
  }, [suggestions])

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
          // Keep successful create/update results in the model history so
          // follow-up turns know the ids (and text) of what already exists.
          const isError = Boolean(event.message?.startsWith('Error:'))
          const isDraftTool = event.tool_name === 'create_draft' || event.tool_name === 'update_draft'
          const isRevisionTool = event.tool_name === 'revise_composer_post'
          if (!isError && event.tool_name && event.tool_name !== 'fetch_url' && event.message) {
            let note = `[${event.tool_name}] ${event.message}`
            if (isDraftTool && event.payload && typeof event.payload === 'object') {
              const post = event.payload as { content?: string }
              if (post.content) {
                note += `\nCurrent draft content:\n${post.content}`
              }
            }
            if (isRevisionTool && event.payload && typeof event.payload === 'object') {
              const revision = event.payload as { content?: string; account_content_override?: Record<string, string> }
              if (revision.content) {
                note += `\nProposed default text:\n${revision.content}`
              }
              for (const [accountId, body] of Object.entries(revision.account_content_override ?? {})) {
                note += `\nProposed override for ${accountId}:\n${body}`
              }
            }
            history.current = [...history.current, { role: 'assistant', content: note }]
          }
          if (isDraftTool && event.payload && typeof event.payload === 'object') {
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
                  updated: event.tool_name === 'update_draft',
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
          if (isRevisionTool && !isError && event.payload && typeof event.payload === 'object') {
            const revision = event.payload as { content?: string; account_content_override?: Record<string, string> }
            setEntries((current) => [
              ...current,
              {
                kind: 'revision',
                text: '',
                revision: {
                  content: revision.content ?? '',
                  accountContentOverride: revision.account_content_override ?? {},
                },
              },
            ])
            return
          }
          if (isError) {
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
    let modelText = text
    if (composerContext) {
      modelText += buildComposerNote(composerContext, deselectedAccountIds)
    }
    const userMessage: BackendAIChatMessage = {
      role: 'user',
      content: modelText,
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
      await api.streamAIChat(teamId, history.current, handleEvent, controller.signal, getViewContext())
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

  // Portal to <body> so the floating widget escapes the .app-main stacking
  // context (position: relative; z-index: 0). Rendered inline, its z-index is
  // capped at app-main's level and the preview-column avatar paints over it.
  return createPortal(
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
                    <p className="eyebrow">{t(entry.updated ? 'aiChat.draftUpdated' : 'aiChat.draftCreated')}</p>
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
              if (entry.kind === 'revision' && entry.revision) {
                const revision = entry.revision
                const overrideEntries = Object.entries(revision.accountContentOverride)
                const accountName = (id: string) =>
                  composerContext?.targets.find((target) => target.accountId === id)?.name ??
                  teamAccounts.find((account) => account.id === id)?.name ??
                  id
                return (
                  <article key={index} className="ai-chat-preview ai-chat-preview--revision">
                    <p className="eyebrow">{t('aiChat.revisionReady')}</p>
                    {revision.content ? (
                      <div>
                        <span className="hint">{t('aiChat.revisionDefault')}</span>
                        <p className="ai-chat-preview__content">{revision.content}</p>
                      </div>
                    ) : null}
                    {overrideEntries.map(([accountId, body]) => (
                      <div key={accountId}>
                        <span className="hint">{t('aiChat.revisionFor', { name: accountName(accountId) })}</span>
                        <p className="ai-chat-preview__content">{body}</p>
                      </div>
                    ))}
                    {onApplyToComposer ? (
                      <button
                        type="button"
                        className="btn btn--secondary btn--sm"
                        onClick={() =>
                          onApplyToComposer({
                            content: revision.content || undefined,
                            accountContentOverride:
                              overrideEntries.length > 0 ? revision.accountContentOverride : undefined,
                          })
                        }
                      >
                        {t('aiChat.applyToComposer')}
                      </button>
                    ) : null}
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

          {composerContext && (
            <div className="ai-chat-mentions">
              <button
                type="button"
                className="ai-chat-chip ai-chat-chip--context"
                onClick={() => {
                  setComposerContext(null)
                  setDeselectedAccountIds([])
                }}
                title={t('aiChat.composerContextDetach')}
              >
                {t('aiChat.composerContextAttached')} ×
              </button>
              {composerContext.targets.map((target) => {
                const active = !deselectedAccountIds.includes(target.accountId)
                return (
                  <button
                    key={target.accountId}
                    type="button"
                    className={`ai-chat-chip ai-chat-chip--scope${active ? '' : ' ai-chat-chip--inactive'}`}
                    aria-pressed={active}
                    onClick={() =>
                      setDeselectedAccountIds((current) =>
                        active ? [...current, target.accountId] : current.filter((id) => id !== target.accountId),
                      )
                    }
                    title={active ? t('aiChat.composerScopeExclude') : t('aiChat.composerScopeInclude')}
                  >
                    {t('aiChat.composerScopeChip', { provider: providerLabel(target.provider, t), name: target.name })}
                  </button>
                )
              })}
            </div>
          )}

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
            <div className="ai-chat-suggestions" role="listbox">
              {filteredSuggestions.map((suggestion, index) => (
                <button
                  key={suggestion.key}
                  type="button"
                  role="option"
                  aria-selected={index === activeSuggestion}
                  className={index === activeSuggestion ? 'ai-chat-suggestions__active' : undefined}
                  onMouseEnter={() => setActiveSuggestion(index)}
                  onClick={suggestion.apply}
                >
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
                // When the @mention / slash-command dropdown is open, the arrow keys
                // move the highlight and Enter/Tab pick it — so it works without a mouse.
                if (filteredSuggestions.length > 0) {
                  if (event.key === 'ArrowDown') {
                    event.preventDefault()
                    setActiveSuggestion((index) => (index + 1) % filteredSuggestions.length)
                    return
                  }
                  if (event.key === 'ArrowUp') {
                    event.preventDefault()
                    setActiveSuggestion((index) => (index - 1 + filteredSuggestions.length) % filteredSuggestions.length)
                    return
                  }
                  if (event.key === 'Enter' || event.key === 'Tab') {
                    event.preventDefault()
                    filteredSuggestions[Math.min(activeSuggestion, filteredSuggestions.length - 1)]?.apply()
                    return
                  }
                  if (event.key === 'Escape') {
                    event.preventDefault()
                    setSuggestions(null)
                    return
                  }
                }
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
    </>,
    document.body,
  )
}

function providerLabel(provider: string, t: (key: string) => string): string {
  const key = `aiChat.provider.${provider}`
  const label = t(key)
  // i18next returns the key unchanged when there is no translation.
  return label === key ? provider : label
}

// buildComposerNote describes the unsaved composer post to the model: the default
// text, each account's current version, and which accounts are in scope for this
// request, so it can revise one platform without touching the others.
function buildComposerNote(
  composerContext: { content: string; targets: ComposerChatTarget[] },
  deselectedAccountIds: string[],
): string {
  const inScope = composerContext.targets.filter((target) => !deselectedAccountIds.includes(target.accountId))
  const targets = inScope.length > 0 ? inScope : composerContext.targets
  const lines = targets.map((target) => {
    const version = target.hasOverride ? `own version: ${target.text}` : 'uses the default text'
    return `- ${target.name} (id=${target.accountId}, ${target.provider}, max ${target.maxChars}): ${version}`
  })
  const scoped = inScope.length > 0 && inScope.length < composerContext.targets.length
  const scopeLine = scoped
    ? '\nOnly revise the accounts listed above; leave every other account unchanged.'
    : ''
  return (
    '\n\n[Composer context — the user is editing an UNSAVED post (it has NO id; use revise_composer_post, never create_draft or update_draft). ' +
    'The default text is the long version used by accounts without an override.]\n' +
    `Default text:\n${composerContext.content || '(empty)'}\n` +
    `Accounts in scope:\n${lines.join('\n')}` +
    scopeLine
  )
}

function toolLabel(toolName: string, t: (key: string) => string): string {
  switch (toolName) {
    case 'fetch_url':
      return t('aiChat.toolFetchUrl')
    case 'create_draft':
      return t('aiChat.toolCreateDraft')
    case 'update_draft':
      return t('aiChat.toolUpdateDraft')
    case 'revise_composer_post':
      return t('aiChat.toolReviseComposer')
    case 'create_campaign':
      return t('aiChat.toolCreateCampaign')
    case 'create_recurring_automation':
      return t('aiChat.toolCreateRecurring')
    case 'create_rss_automation':
      return t('aiChat.toolCreateRss')
    case 'get_top_hashtags':
      return t('aiChat.toolTopHashtags')
    default:
      return toolName
  }
}
