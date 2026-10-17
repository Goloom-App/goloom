import { useTranslation } from 'react-i18next'

export function AIFallbackBadge({ reason, jobId }: { reason?: string; jobId?: string }) {
  const { t } = useTranslation()
  if (!reason) return null
  return <span className="badge badge--warning" data-testid="ai-fallback-badge"
    title={[t(`review.aiFallbackReasons.${reason}`, { defaultValue: t('review.aiFallbackReasons.unknown') }), jobId ? t('review.aiFallbackJob', { id: jobId }) : ''].filter(Boolean).join(' ')}>
    {t('review.aiFallbackBadge')}
  </span>
}
