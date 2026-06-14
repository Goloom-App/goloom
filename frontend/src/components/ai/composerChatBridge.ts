// Bridge between the composer and the AI chat widget. The composer dispatches a
// window event carrying the current draft (text + selected destinations); the
// mounted AIChatWidget listens and keeps that context attached, so the user can
// ask things like "rework this post for Bluesky" without leaving the chat.

export const composerContextEvent = 'goloom:ai-chat-composer'

export interface ComposerChatTarget {
  name: string
  provider: string
}

export interface ComposerChatContext {
  content: string
  title?: string
  targets: ComposerChatTarget[]
  /** auto = attached because the composer is open; manual = the user clicked a button (also opens the chat). */
  auto?: boolean
  /** clear = the composer closed; the chat should drop the attached context. */
  clear?: boolean
}

function dispatch(detail: ComposerChatContext) {
  window.dispatchEvent(new CustomEvent(composerContextEvent, { detail }))
}

/** Attach/refresh the composer context silently (does not open the chat). */
export function syncComposerChatContext(ctx: Omit<ComposerChatContext, 'auto' | 'clear'>) {
  dispatch({ ...ctx, auto: true })
}

/** Drop the attached composer context (composer closed). */
export function clearComposerChatContext() {
  dispatch({ content: '', targets: [], auto: true, clear: true })
}

/** Open the AI chat with the current composer draft attached as context. */
export function openAIChatWithComposerContext(ctx: Omit<ComposerChatContext, 'auto' | 'clear'>) {
  dispatch({ ...ctx, auto: false })
}
