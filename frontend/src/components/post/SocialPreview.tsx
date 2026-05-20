import { format, parseISO } from 'date-fns'
import { useTranslation } from 'react-i18next'

import type { AccountRecord } from '../../types'
import { Icon } from '../../icons'
import { AuthMediaThumb } from '../Media/AuthMediaThumb'
import { DestinationAvatar } from './DestinationAvatar'
import type { PreviewLayout, SocialPreviewAttachment, PreviewEngagement } from './SocialPreview.types'

function layoutClass(layout: PreviewLayout) {
  switch (layout) {
    case 'mastodon':
      return 'social-preview--layout-mastodon'
    case 'bluesky':
      return 'social-preview--layout-bluesky'
    case 'friendica':
      return 'social-preview--layout-friendica'
    default:
      return ''
  }
}

function resolveLayout(provider: AccountRecord['provider']): PreviewLayout {
  if (provider === 'bluesky') {
    return 'bluesky'
  }
  if (provider === 'friendica') {
    return 'friendica'
  }
  return 'mastodon'
}

export function SocialPreview({
  account,
  content,
  scheduledAt,
  theme,
  publishedPostUrl,
  layout,
  engagement,
  attachments,
  authHeader,
}: {
  account: AccountRecord
  content: string
  scheduledAt: string
  theme: 'dark' | 'light'
  publishedPostUrl?: string
  /** When omitted, layout follows account provider. */
  layout?: PreviewLayout
  /** When set (e.g. archive), show interaction counts similar to native posts. */
  engagement?: PreviewEngagement | null
  /** Library preview URLs require Bearer fetch (same as composer). */
  attachments?: SocialPreviewAttachment[]
  authHeader?: string
}) {
  const { t } = useTranslation()
  const resolved = layout ?? resolveLayout(account.provider)
  const canLoadMedia = Boolean(authHeader?.trim())
  return (
    <div
      className={`social-preview ${theme === 'dark' ? 'social-preview--dark' : ''} ${layoutClass(resolved)}`}
      data-provider={account.provider}
    >
      <div className="social-preview__header">
        <div className="social-preview__avatar-wrap">
          <DestinationAvatar account={account} publishedPostUrl={publishedPostUrl} />
        </div>
        <div className="social-preview__meta">
          <span className="social-preview__name">{account.name}</span>
          <span className="social-preview__handle">{account.username}</span>
        </div>
      </div>
      <div className="social-preview__body">
        {content || <span className="hint">{t('post.contentPlaceholder')}</span>}
      </div>
      {attachments && attachments.length > 0 ? (
        <div className={`social-preview__media social-preview__media--${resolved}`} aria-label={t('post.mediaAttachmentsAria')}>
          {attachments.map((item) => {
            const isVideo = item.mimeType.startsWith('video/')
            return (
              <figure key={item.id} className="social-preview__media-figure">
                {isVideo ? (
                  <span className="social-preview__media-video">
                    <Icon name="film" className="inline-icon" aria-hidden />
                    <span className="social-preview__media-video-label">{item.filename ?? t('common.video')}</span>
                  </span>
                ) : canLoadMedia ? (
                  <AuthMediaThumb
                    url={item.previewUrl}
                    authHeader={authHeader!}
                    alt={item.filename ?? ''}
                    className="social-preview__media-thumb"
                  />
                ) : (
                  <span className="social-preview__media-placeholder">
                    <Icon name="image" className="inline-icon" aria-hidden />
                  </span>
                )}
              </figure>
            )
          })}
        </div>
      ) : null}
      {engagement ? (
        <div className="social-preview__stats" aria-label={t('post.engagementAria')}>
          <span title={t('post.likesTitle')}>
            <span className="social-preview__stat-value">{engagement.likes}</span>
            <span className="social-preview__stat-label">{t('post.likes')}</span>
          </span>
          <span title={t('post.sharesTitle')}>
            <span className="social-preview__stat-value">{engagement.reposts}</span>
            <span className="social-preview__stat-label">{t('post.shares')}</span>
          </span>
          <span title={t('post.repliesTitle')}>
            <span className="social-preview__stat-value">{engagement.replies}</span>
            <span className="social-preview__stat-label">{t('post.replies')}</span>
          </span>
        </div>
      ) : null}
      <div className="social-preview__footer">
        {scheduledAt ? format(parseISO(scheduledAt), 'PPpp') : t('common.notScheduled')}
      </div>
    </div>
  )
}
