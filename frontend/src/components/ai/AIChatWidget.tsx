import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Bot, Loader2, Send, Sparkles, X } from 'lucide-react'
import type { BackendAIChatEvent, BackendAIChatMention, BackendAIChatMessage, createApiClient } from '../../api'
import type { AccountRecord } from '../../types'
import { composerContextEvent } from './composerChatBridge'
import type { ComposerChatContext } from './composerChatBridge'
import { getViewContext, setViewFocus, setViewVisible } from './viewContextBridge'
import type { ViewContext } from './viewContextBridge'

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

/** A write action the assistant proposed but did not run (scheduling, deletion,
 *  automations). The user confirms or dismisses it; on confirm it runs via the
 *  confirm-action endpoint. */
interface ConfirmCardData {
  tool: string
  args: Record<string, unknown>
  summary: string
  resolved?: 'confirmed' | 'declined'
}

interface ChatEntry {
  kind: 'user' | 'assistant' | 'tool' | 'preview' | 'revision' | 'confirm' | 'error'
  text: string
  mentions?: BackendAIChatMention[]
  preview?: DraftPreview
  revision?: RevisionPreview
  confirm?: ConfirmCardData
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
  { command: '/clear', textKey: 'aiChat.commandClear' },
]

interface AIChatWidgetProps {
  api: ApiClient
  teamId: string
  focusedPostId?: string
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

export function AIChatWidget({ api, teamId, focusedPostId, teamAccounts, onOpenInComposer, onApplyToComposer }: AIChatWidgetProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [entries, setEntries] = useState<ChatEntry[]>([])
  const [input, setInput] = useState('')
  const [pendingMentions, setPendingMentions] = useState<BackendAIChatMention[]>([])
  const [busy, setBusy] = useState(false)
  const [composerContext, setComposerContext] = useState<ComposerChatContext | null>(null)
  // Account ids the user toggled out of scope: edits then only touch the rest.
  const [deselectedAccountIds, setDeselectedAccountIds] = useState<string[]>([])
  const [mentionOptions, setMentionOptions] = useState<MentionOption[]>([])
  const [suggestions, setSuggestions] = useState<{ kind: 'mention' | 'command'; query: string } | null>(null)
  // Keyboard-driven selection within the @mention / slash-command dropdown.
  const [activeSuggestion, setActiveSuggestion] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const abortRef = useRef<AbortController | null>(null)

  // History sent to the model: user/assistant turns only; tool outcomes ride
  // along as tool_results on the assistant message they belong to.
  const history = useRef<BackendAIChatMessage[]>([])

  // Attach a compact tool outcome to the current assistant turn so follow-up
  // requests still know ids (e.g. post_id) without replaying full payloads.
  const recordToolResult = useCallback((tool: string, summary: string) => {
    const entry = { tool, summary }
    const last = history.current[history.current.length - 1]
    if (last && last.role === 'assistant') {
      history.current = [
        ...history.current.slice(0, -1),
        { ...last, tool_results: [...(last.tool_results ?? []), entry] },
      ]
    } else {
      history.current = [...history.current, { role: 'assistant', content: '', tool_results: [entry] }]
    }
  }, [])

  const [composerDetached, setComposerDetached] = useState(false)

  // Wipe the in-memory conversation and start a fresh agent session. Used by the
  // team switch and the /clear command; aborts any in-flight stream first.
  const resetConversation = useCallback((detachComposer = true) => {
    abortRef.current?.abort()
    abortRef.current = null
    history.current = []
    setEntries([])
    setPendingMentions([])
    setInput('')
    setComposerContext(null)
    setDeselectedAccountIds([])
    setSuggestions(null)
    setBusy(false)
    setComposerDetached(detachComposer)
    setViewFocus(undefined)
    setViewVisible(undefined)
  }, [])

  useEffect(() => {
    // Reset the conversation when the team changes.
    resetConversation(false)
  }, [teamId, resetConversation])

  useEffect(() => {
    function onComposerContext(event: Event) {
      const detail = (event as CustomEvent).detail as ComposerChatContext | undefined
      if (!detail) {
        return
      }
      if (detail.clear) {
        setComposerContext(null)
        setDeselectedAccountIds([])
        setViewFocus(undefined)
        setViewVisible(undefined)
        setComposerDetached(false)
        return
      }
      // Detach (including /clear) survives auto-sync edits until the composer
      // closes, or its existing "Open AI chat" action explicitly re-attaches.
      if (composerDetached && detail.auto) {
        return
      }
      setComposerDetached(false)
      if (!detail.content.trim()) {
        // No text yet (e.g. an empty composer just opened): don't attach a chip,
        // but if the previous context is now empty, drop it.
        setComposerContext((prev) => (prev ? null : prev))
        setViewFocus(undefined)
        setViewVisible(undefined)
        return
      }
      setComposerContext({ content: detail.content, title: detail.title, targets: detail.targets ?? [] })
      // Mirror the open composer draft into the view context so the agent can see,
      // via get_current_view, which post is open and what it currently says.
      setViewFocus(focusedPostId ? { type: 'post', id: focusedPostId } : { type: 'composerDraft', label: detail.title })
      setViewVisible({
        composer: {
          content: detail.content,
          title: detail.title,
          targets: (detail.targets ?? []).map((target) => ({
            accountId: target.accountId,
            name: target.name,
            provider: target.provider,
            maxChars: target.maxChars,
            text: target.text,
            hasOverride: target.hasOverride,
          })),
        },
      })
      // Manual triggers (a button) open and focus the chat; auto-sync stays silent.
      if (!detail.auto) {
        setOpen(true)
        setTimeout(() => inputRef.current?.focus(), 0)
      }
    }
    window.addEventListener(composerContextEvent, onComposerContext)
    return () => window.removeEventListener(composerContextEvent, onComposerContext)
  }, [composerDetached, focusedPostId])

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
    setSuggestions(null)
    if (cmd.command === '/clear') {
      resetConversation()
      return
    }
    setInput(t(cmd.textKey))
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
          const isDraftTool = event.tool_name === 'draft_post' || event.tool_name === 'modify_post'
          const isRevisionTool = event.tool_name === 'revise_composer_post'
          const confirmation =
            event.payload && typeof event.payload === 'object'
              ? (event.payload as { confirmation?: { tool?: string; args?: Record<string, unknown>; summary?: string } }).confirmation
              : undefined
          if (!isError && event.tool_name && ECHOED_TOOL_RESULTS.has(event.tool_name) && event.message) {
            recordToolResult(event.tool_name, summarizeToolResult(event.message, event.payload))
          }
          if (confirmation?.tool) {
            setEntries((current) => [
              ...current,
              {
                kind: 'confirm',
                text: '',
                confirm: {
                  tool: confirmation.tool ?? '',
                  args: confirmation.args ?? {},
                  summary: confirmation.summary ?? '',
                },
              },
            ])
            return
          }
          if (isDraftTool && event.payload && typeof event.payload === 'object') {
            const post = event.payload as {
              post_id?: string
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
                  updated: event.tool_name === 'modify_post',
                  preview: {
                    id: post.post_id ?? post.id ?? '',
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
            // The server-side agent loop retries failed tool calls itself (e.g.
            // shortening content that overran a platform limit), so show a muted
            // "adjusting" step rather than an alarming error bubble. Real
            // terminal failures still surface via the 'error' event and the
            // stream catch in send().
            setEntries((current) => [...current, { kind: 'tool', text: t('aiChat.toolAdjusting') }])
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
    [t, recordToolResult],
  )

  // Run a proposed write after the user confirms it. Team access and scope are
  // re-checked server-side, so this is a deliberate user action, not a bypass.
  async function confirmProposedAction(index: number, data: ConfirmCardData) {
    setBusy(true)
    try {
      const res = await api.confirmAgentAction(teamId, data.tool, data.args)
      setEntries((current) =>
        current.map((entry, i) => (i === index ? { ...entry, confirm: { ...data, resolved: 'confirmed' } } : entry)),
      )
      if (ECHOED_TOOL_RESULTS.has(data.tool)) {
        recordToolResult(data.tool, summarizeToolResult(`confirmed by the user. ${res.summary ?? ''}`, res.payload))
      }
      const post =
        res.payload && typeof res.payload === 'object'
          ? (res.payload as { post_id?: string; content?: string; title?: string; target_accounts?: string[]; scheduled_at?: string })
          : null
      if (post?.content) {
        setEntries((current) => [
          ...current,
          {
            kind: 'preview',
            text: '',
            preview: {
              id: post.post_id ?? '',
              title: post.title ?? '',
              content: post.content ?? '',
              targetAccountIds: post.target_accounts ?? [],
              scheduledAt: post.scheduled_at,
            },
          },
        ])
      }
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : String(cause)
      setEntries((current) => [...current, { kind: 'error', text: message || t('aiChat.genericError') }])
    } finally {
      setBusy(false)
    }
  }

  function declineProposedAction(index: number, data: ConfirmCardData) {
    setEntries((current) =>
      current.map((entry, i) => (i === index ? { ...entry, confirm: { ...data, resolved: 'declined' } } : entry)),
    )
    history.current = [...history.current, { role: 'user', content: `Dismissed the proposed ${data.tool}.` }]
  }

  async function send() {
    const text = input.trim()
    if (text === '/clear') {
      // Start a fresh session; allowed even mid-stream (reset aborts it).
      resetConversation()
      return
    }
    if (!text || busy) {
      return
    }
    const mentions = pendingMentions.filter((mention) => text.includes(`@${mention.name}`))
    const viewContext = outgoingViewContext(getViewContext(), composerContext, deselectedAccountIds)
    // Persisted identity comes from the current selection, never a stale composer event.
    if (focusedPostId) viewContext.focus = { type: 'post', id: focusedPostId }
    else if (viewContext.focus?.type === 'post') delete viewContext.focus
    const scopedComposer = viewContext.visible?.composer
    const modelText = scopedComposer ? text + buildComposerNote(scopedComposer, focusedPostId) : text
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
      await api.streamAIChat(teamId, history.current, handleEvent, controller.signal, viewContext)
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
              if (entry.kind === 'confirm' && entry.confirm) {
                const c = entry.confirm
                const proposedContent = typeof c.args.content === 'string' ? (c.args.content as string) : ''
                const proposedAt = typeof c.args.scheduled_at === 'string' ? new Date(c.args.scheduled_at as string) : null
                const proposedAtLabel = proposedAt && !Number.isNaN(proposedAt.getTime()) ? proposedAt.toLocaleString() : ''
                const rawTargets = c.args.target_accounts ?? c.args.target_account_ids
                const proposedTargets = Array.isArray(rawTargets)
                  ? (rawTargets as unknown[])
                      .filter((id): id is string => typeof id === 'string')
                      .map((id) => teamAccounts.find((account) => account.id === id)?.name ?? id)
                  : []
                const proposedVisibility = typeof c.args.visibility === 'string' ? (c.args.visibility as string) : ''
                return (
                  <article key={index} className="ai-chat-preview ai-chat-preview--confirm">
                    <p className="eyebrow">{t('aiChat.confirmTitle', { tool: toolLabel(c.tool, t) })}</p>
                    {c.summary ? <p className="ai-chat-preview__content">{c.summary}</p> : null}
                    {proposedContent ? <p className="ai-chat-preview__content">{proposedContent}</p> : null}
                    {proposedAtLabel ? (
                      <p className="hint">{t('aiChat.confirmScheduledAt', { time: proposedAtLabel })}</p>
                    ) : null}
                    {proposedTargets.length > 0 ? (
                      <p className="hint">{t('aiChat.confirmTargets', { names: proposedTargets.join(', ') })}</p>
                    ) : null}
                    {proposedVisibility ? (
                      <p className="hint">{t('aiChat.confirmVisibility', { value: proposedVisibility })}</p>
                    ) : null}
                    {c.resolved ? (
                      <p className="hint">{t(c.resolved === 'confirmed' ? 'aiChat.confirmDone' : 'aiChat.confirmDeclined')}</p>
                    ) : (
                      <div className="ai-chat-confirm__actions">
                        <button
                          type="button"
                          className="btn btn--primary btn--sm"
                          disabled={busy}
                          onClick={() => confirmProposedAction(index, c)}
                        >
                          {t('aiChat.confirmApprove')}
                        </button>
                        <button
                          type="button"
                          className="btn btn--ghost btn--sm"
                          disabled={busy}
                          onClick={() => declineProposedAction(index, c)}
                        >
                          {t('aiChat.confirmDecline')}
                        </button>
                      </div>
                    )}
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
                  setComposerDetached(true)
                  setViewFocus(undefined)
                  setViewVisible(undefined)
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

const TOOL_SUMMARY_MAX = 300

// Only write/card tools echo their outcome (ids, status) back in the history.
// Read tools such as fetch_url return untrusted text and must stay out. Keep in
// sync with chatToolResultAllowlist in internal/ai/chat.go.
const ECHOED_TOOL_RESULTS = new Set([
  'draft_post',
  'modify_post',
  'revise_composer_post',
  'create_campaign',
  'schedule_post',
  'delete_post',
  'create_recurring',
  'create_rss_feed',
])

// summarizeToolResult builds the short tool outcome sent back with the history:
// the (capped) result text plus the post id when the payload carries one.
function summarizeToolResult(message: string, payload: unknown): string {
  let summary = message.replace(/\s+/g, ' ').trim()
  if (summary.length > TOOL_SUMMARY_MAX) {
    summary = `${summary.slice(0, TOOL_SUMMARY_MAX)}…`
  }
  const postId = payload && typeof payload === 'object' ? (payload as { post_id?: unknown }).post_id : undefined
  if (typeof postId === 'string' && postId && !summary.includes(postId)) {
    summary = `post_id=${postId} ${summary}`.trim()
  }
  return summary
}

function providerLabel(provider: string, t: (key: string) => string): string {
  const key = `aiChat.provider.${provider}`
  const label = t(key)
  // i18next returns the key unchanged when there is no translation.
  return label === key ? provider : label
}

// Derive both request snapshots from current scope, never stale bridge data.
function outgoingViewContext(
  base: ViewContext | undefined,
  composer: ComposerChatContext | null,
  deselectedAccountIds: string[],
): ViewContext & { visible?: Record<string, unknown> & { composer?: ComposerChatContext } } {
  const context = { ...base }
  const visible = { ...context.visible }
  delete visible.composer
  const targets = composer?.targets.filter((target) => !deselectedAccountIds.includes(target.accountId)) ?? []
  if (composer && targets.length > 0) {
    visible.composer = { content: composer.content, title: composer.title, targets }
  } else if (context.focus?.type === 'composerDraft') {
    delete context.focus
  }
  return { ...context, visible }
}

// Scope was already filtered with the outgoing view context; never restore
// excluded accounts when no targets remain.
function buildComposerNote(composerContext: ComposerChatContext, focusedPostId?: string): string {
  const lines = composerContext.targets.map((target) => {
    const version = target.hasOverride ? `own version: ${target.text}` : 'uses the default text'
    return `- ${target.name} (id=${target.accountId}, ${target.provider}, max ${target.maxChars}): ${version}`
  })
  return (
    '\n\n[Composer context — the user is editing ' +
    (focusedPostId ? 'UNSAVED changes to the focused saved post' : 'an UNSAVED post (it has NO id)') +
    '; use revise_composer_post for these unsaved edits, never create_draft or update_draft. ' +
    'The default text is the long version used by accounts without an override.]\n' +
    `Default text:\n${composerContext.content || '(empty)'}\n` +
    `Accounts in scope:\n${lines.join('\n')}` +
    '\nOnly revise the accounts listed above; leave every other account unchanged.'
  )
}

function toolLabel(toolName: string, t: (key: string) => string): string {
  switch (toolName) {
    case 'fetch_url':
      return t('aiChat.toolFetchUrl')
    case 'draft_post':
      return t('aiChat.toolCreateDraft')
    case 'modify_post':
      return t('aiChat.toolModifyPost')
    case 'revise_composer_post':
      return t('aiChat.toolReviseComposer')
    case 'schedule_post':
      return t('aiChat.toolSchedule')
    case 'delete_post':
      return t('aiChat.toolDelete')
    case 'create_campaign':
      return t('aiChat.toolCreateCampaign')
    case 'create_recurring':
      return t('aiChat.toolCreateRecurring')
    case 'create_rss_feed':
      return t('aiChat.toolCreateRss')
    case 'get_current_view':
      return t('aiChat.toolCurrentView')
    case 'get_brand_profile':
      return t('aiChat.toolBrandProfile')
    case 'get_calendar':
    case 'get_posts':
    case 'search_posts':
    case 'find_free_slot':
    case 'get_campaign':
    case 'get_platforms':
    case 'get_analytics':
    case 'get_analytics_timeslots':
    case 'get_hashtag_performance':
      return t('aiChat.toolLookup')
    default:
      return toolName
  }
}
