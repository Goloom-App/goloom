export interface ToggleSwitchProps {
  checked: boolean
  onChange: (next: boolean) => void
  title: string
  description?: string
  testId?: string
  disabled?: boolean
}

/**
 * Boolean control with a real switch + descriptive title/copy.
 * Use instead of <input type="checkbox"> next to a label.
 */
export function ToggleSwitch({
  checked,
  onChange,
  title,
  description,
  testId,
  disabled,
}: ToggleSwitchProps) {
  return (
    <label className="brand-toggle" data-testid={testId} aria-disabled={disabled}>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="brand-toggle__switch" aria-hidden="true" />
      <span className="brand-toggle__copy">
        <span className="brand-toggle__title">{title}</span>
        {description ? <span className="brand-toggle__desc">{description}</span> : null}
      </span>
    </label>
  )
}
