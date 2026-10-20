import { useRef } from 'react'
import type { ReactNode } from 'react'

interface PageTab<T extends string> {
  id: T
  label: ReactNode
}

interface PageTabsProps<T extends string> {
  label: string
  value: T
  options: readonly PageTab<T>[]
  onChange: (value: T) => void
  actions?: ReactNode
  idPrefix: string
  panelId: string
  tourPrefix?: string
  testIdPrefix?: string
}

/** Page-level navigation. Inline form choices belong in Segmented instead. */
export function PageTabs<T extends string>({
  label, value, options, onChange, actions, idPrefix, panelId, tourPrefix, testIdPrefix,
}: PageTabsProps<T>) {
  const buttons = useRef<(HTMLButtonElement | null)[]>([])

  return (
    <div className="page-tabs">
      <nav className="page-tabs__list view-toggle view-toggle--scrollable" role="tablist" aria-label={label}>
        {options.map((option, index) => (
          <button
            key={option.id}
            ref={(element) => { buttons.current[index] = element }}
            id={`${idPrefix}-${option.id}`}
            type="button"
            role="tab"
            aria-selected={value === option.id}
            aria-controls={panelId}
            tabIndex={value === option.id ? 0 : -1}
            data-tour={tourPrefix ? `${tourPrefix}-${option.id}` : undefined}
            data-testid={testIdPrefix ? `${testIdPrefix}-${option.id}` : undefined}
            className={`view-toggle__btn ${value === option.id ? 'view-toggle__btn--active' : ''}`}
            onClick={() => onChange(option.id)}
            onKeyDown={(event) => {
              let next: number
              switch (event.key) {
                case 'ArrowRight': next = (index + 1) % options.length; break
                case 'ArrowLeft': next = (index - 1 + options.length) % options.length; break
                case 'Home': next = 0; break
                case 'End': next = options.length - 1; break
                default: return
              }
              event.preventDefault()
              onChange(options[next].id)
              buttons.current[next]?.focus()
              buttons.current[next]?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
            }}
          >
            {option.label}
          </button>
        ))}
      </nav>
      {actions && <div className="page-tabs__actions">{actions}</div>}
    </div>
  )
}
