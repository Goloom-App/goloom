export interface SegmentedOption<T extends string> {
  id: T
  label: string
}

export interface SegmentedProps<T extends string> {
  value: T
  options: SegmentedOption<T>[]
  onChange: (next: T) => void
  testIdPrefix?: string
  ariaLabel?: string
}

/**
 * Segmented control / pill-style tab group. Use instead of a small select
 * when there are 2-5 mutually exclusive options.
 */
export function Segmented<T extends string>({
  value,
  options,
  onChange,
  testIdPrefix,
  ariaLabel,
}: SegmentedProps<T>) {
  return (
    <div className="brand-segmented" role="tablist" aria-label={ariaLabel}>
      {options.map((option) => (
        <button
          key={option.id}
          type="button"
          role="tab"
          aria-selected={value === option.id}
          data-testid={testIdPrefix ? `${testIdPrefix}-${option.id}` : undefined}
          className={`brand-segmented__item${value === option.id ? ' brand-segmented__item--active' : ''}`}
          onClick={() => onChange(option.id)}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}
