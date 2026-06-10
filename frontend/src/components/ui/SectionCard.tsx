import type { ReactNode } from 'react'

export interface SectionCardProps {
  icon?: ReactNode
  title: string
  subtitle?: string
  children: ReactNode
  hero?: boolean
  headerExtra?: ReactNode
  testId?: string
  className?: string
}

/**
 * A consistent section/card layout with icon header, title, optional
 * subtitle, and an optional header-extra slot (typically a button).
 * Use `hero` for the primary highlighted card in a view.
 */
export function SectionCard({
  icon,
  title,
  subtitle,
  children,
  hero,
  headerExtra,
  testId,
  className,
}: SectionCardProps) {
  return (
    <section
      className={`brand-card${hero ? ' brand-card--hero' : ''}${className ? ` ${className}` : ''}`}
      data-testid={testId}
    >
      <header className="brand-card__header">
        {icon ? <span className="brand-card__icon">{icon}</span> : null}
        <div className="brand-card__heading">
          <h2 className="brand-card__title">{title}</h2>
          {subtitle ? <p className="brand-card__subtitle">{subtitle}</p> : null}
        </div>
        {headerExtra ? <div>{headerExtra}</div> : null}
      </header>
      <div className="brand-card__body">{children}</div>
    </section>
  )
}
