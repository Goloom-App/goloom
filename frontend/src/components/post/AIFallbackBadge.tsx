import { useTranslation } from 'react-i18next'

export function AIFallbackBadge({ reason }: { reason?: string }) {
  const { t } = useTranslation()
  if (!reason) return null
  return <span className="badge badge--warning" data-testid="ai-fallback-badge"
    title={t(`review.aiFallbackReasons.${reason}`, { defaultValue: t('review.aiFallbackReasons.unknown') })}>
    {t('review.aiFallbackBadge')}
  </span>
}
