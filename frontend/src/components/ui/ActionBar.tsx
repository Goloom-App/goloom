import type { ReactNode } from 'react'

export interface ActionBarProps {
  left?: ReactNode
  right?: ReactNode
  /** When true, the bar sticks to the bottom of its scroll container. */
  sticky?: boolean
  testId?: string
}

/**
 * Sticky action footer with optional left- and right-aligned button groups.
 * Drop into the bottom of a stack/wizard view.
 */
export function ActionBar({ left, right, sticky = true, testId }: ActionBarProps) {
  return (
    <div
      className={`brand-actionbar${sticky ? '' : ' brand-actionbar--static'}`}
      data-testid={testId}
    >
      <div className="brand-actionbar__group">{left}</div>
      <div className="brand-actionbar__group">{right}</div>
    </div>
  )
}
