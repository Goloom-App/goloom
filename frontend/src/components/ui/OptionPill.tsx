import type { ReactNode } from 'react'
import { Check, Plus } from 'lucide-react'

export interface OptionPillProps {
  active: boolean
  onClick: () => void
  children: ReactNode
  testId?: string
}

/**
 * Toggleable pill option. Use for multi-select tag-like choices
 * (mood adjustments, feature flags, filter chips).
 */
export function OptionPill({ active, onClick, children, testId }: OptionPillProps) {
  return (
    <button
      type="button"
      data-testid={testId}
      className={`brand-option-pill${active ? ' brand-option-pill--active' : ''}`}
      onClick={onClick}
    >
      <span className="brand-option-pill__check">
        {active ? <Check size={14} /> : <Plus size={14} />}
      </span>
      {children}
    </button>
  )
}
