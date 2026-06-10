export interface ToggleSwitchProps {
  checked: boolean
  onChange: (next: boolean) => void
  title: string
  description?: string
  testId?: string
  disabled?: boolean
  compact?: boolean
}

/**
 * Boolean control with a real switch + descriptive title/copy.
 * Use instead of <input type="checkbox"> next to a label.
 * Set `compact` for inline use (e.g. in card headers) - hides text, reduces size.
 */
export function ToggleSwitch({
  checked,
  onChange,
  title,
  description,
  testId,
  disabled,
  compact,
}: ToggleSwitchProps) {
  return (
    <label className={`brand-toggle${compact ? ' brand-toggle--compact' : ''}`} data-testid={testId} aria-disabled={disabled} title={title}>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="brand-toggle__switch" aria-hidden="true" />
      {!compact && (
        <span className="brand-toggle__copy">
          <span className="brand-toggle__title">{title}</span>
          {description ? <span className="brand-toggle__desc">{description}</span> : null}
        </span>
      )}
    </label>
  )
}
