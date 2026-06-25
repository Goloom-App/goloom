// Generic bridge that tells the AI chat assistant what the user is currently
// looking at. Views push their state here (active section, the focused entity,
// and an optional snapshot of visible data); the mounted AIChatWidget reads the
// latest value and attaches it to each chat request as `view_context`, so the
// agent can ground its help in the current screen ("this post", "here").
//
// This generalises the older composer-only bridge (composerChatBridge.ts): the
// composer is just one of several views that report context.

export interface ViewFocus {
  /** Kind of the focused entity, e.g. 'post', 'campaign', 'automation'. */
  type: string
  /** Stable id of the focused entity, when there is one. */
  id?: string
  /** Human label, for readability in the snapshot. */
  label?: string
}

export interface ViewContext {
  /** Active app section, e.g. 'composer', 'contentCalendar', 'aiCampaigns'. */
  section?: string
  /** The entity the user is focused on within the section. */
  focus?: ViewFocus
  /** Snapshot of the data visible on screen (shape is view-specific). */
  visible?: Record<string, unknown>
}

let current: ViewContext = {}

/** Replace the active section, preserving any focus/visible already reported. */
export function setViewSection(section: string): void {
  current = { ...current, section }
}

/** Set (or clear, with undefined) the focused entity for the current view. */
export function setViewFocus(focus: ViewFocus | undefined): void {
  current = { ...current, focus }
}

/** Set (or clear) the visible-data snapshot for the current view. */
export function setViewVisible(visible: Record<string, unknown> | undefined): void {
  current = { ...current, visible }
}

/** Replace the whole view context at once. */
export function setViewContext(ctx: ViewContext): void {
  current = ctx
}

/** Latest view context, or undefined when nothing has been reported. */
export function getViewContext(): ViewContext | undefined {
  if (!current.section && !current.focus && !current.visible) {
    return undefined
  }
  return current
}
