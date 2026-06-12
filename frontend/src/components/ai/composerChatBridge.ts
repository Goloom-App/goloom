// Bridge between the composer and the AI chat widget: the composer dispatches
// a window event carrying the current draft text; the mounted AIChatWidget
// listens and opens with that draft attached as conversation context.

export const composerContextEvent = 'goloom:ai-chat-composer'

/** Opens the AI chat with the given composer draft attached as context. */
export function openAIChatWithComposerDraft(content: string) {
  window.dispatchEvent(new CustomEvent(composerContextEvent, { detail: { content } }))
}
