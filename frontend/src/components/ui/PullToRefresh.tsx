import { useTranslation } from 'react-i18next'
import { usePullToRefresh } from '../../hooks/usePullToRefresh'
import type { UsePullToRefreshOptions } from '../../hooks/usePullToRefresh'

interface PullToRefreshProps extends UsePullToRefreshOptions {
  children: React.ReactNode
  className?: string
}

export function PullToRefresh({
  onRefresh,
  threshold = 80,
  disabled = false,
  children,
  className,
}: PullToRefreshProps) {
  const { t } = useTranslation()
  const { pullDistance, isRefreshing, pullPhase, containerRef } = usePullToRefresh({
    onRefresh,
    threshold,
    disabled,
  })

  const showIndicator = pullPhase !== 'idle' || isRefreshing

  const label =
    pullPhase === 'threshold'
      ? t('common.releaseToRefresh', { defaultValue: 'Release to refresh' })
      : t('common.pullToRefresh', { defaultValue: 'Pull to refresh' })

  return (
    <main ref={containerRef} className={`pull-to-refresh${className ? ` ${className}` : ''}`}>
      {showIndicator && (
        <div
          className="pull-to-refresh__indicator"
          style={{
            height: `${Math.min(pullDistance, threshold)}px`,
            opacity: Math.min(pullDistance / threshold, 1),
          }}
          aria-hidden="true"
        >
          <div
            className={`pull-to-refresh__spinner ${isRefreshing ? 'pull-to-refresh__spinner--active' : ''}`}
            style={
              !isRefreshing
                ? { transform: `rotate(${180 + (pullDistance / threshold) * 180}deg)` }
                : undefined
            }
          >
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
              {isRefreshing ? (
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              ) : (
                <polyline points="6 9 12 15 18 9" />
              )}
            </svg>
          </div>
          <span className="pull-to-refresh__label">{label}</span>
        </div>
      )}
      {children}
    </main>
  )
}
