import type { LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { AppSection } from '../../types'

interface NavReviewIndicatorProps {
  sectionId: AppSection
  icon: LucideIcon
  count: number
  overdueCount?: number
  showCount?: boolean
}

export function NavReviewIcon({ sectionId, icon: Icon, count, overdueCount = 0 }: NavReviewIndicatorProps) {
  const showBadge = sectionId === 'reviewQueue' && count > 0
  return (
    <span className="sidebar-nav-item__icon-wrap">
      <Icon size={18} />
      {showBadge ? (
        <span
          className={`sidebar-nav-item__notify-dot${overdueCount > 0 ? ' sidebar-nav-item__notify-dot--warning' : ''}`}
          aria-hidden
        />
      ) : null}
    </span>
  )
}

export function NavReviewCount({
  sectionId,
  count,
  overdueCount = 0,
  showCount = true,
}: Omit<NavReviewIndicatorProps, 'icon'>) {
  const { t } = useTranslation()
  if (sectionId !== 'reviewQueue' || count <= 0 || !showCount) {
    return null
  }
  const display = count > 99 ? '99+' : String(count)
  return (
    <span
      className={`sidebar-nav-item__count${overdueCount > 0 ? ' sidebar-nav-item__count--warning' : ''}`}
      data-testid="nav-review-badge"
      aria-label={t('nav.reviewQueuePending', { count })}
    >
      {display}
    </span>
  )
}
