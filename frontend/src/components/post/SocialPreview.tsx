import { format, parseISO } from 'date-fns'
import type { AccountRecord } from '../../types'
import { Icon } from '../../icons'
import { AuthMediaThumb } from '../Media/AuthMediaThumb'
import { DestinationAvatar } from './DestinationAvatar'

export type PreviewLayout = 'card' | 'mastodon' | 'bluesky' | 'friendica'

export type SocialPreviewAttachment = {
  id: string
  previewUrl: string
  mimeType: string
  filename?: string
}

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

export type PreviewEngagement = {
  likes: number
  reposts: number
  replies: number
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
        {content || <span className="hint">Post content will appear here...</span>}
      </div>
      {attachments && attachments.length > 0 ? (
        <div className={`social-preview__media social-preview__media--${resolved}`} aria-label="Media attachments">
          {attachments.map((item) => {
            const isVideo = item.mimeType.startsWith('video/')
            return (
              <figure key={item.id} className="social-preview__media-figure">
                {isVideo ? (
                  <span className="social-preview__media-video">
                    <Icon name="film" className="inline-icon" aria-hidden />
                    <span className="social-preview__media-video-label">{item.filename ?? 'Video'}</span>
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
        <div className="social-preview__stats" aria-label="Engagement">
          <span title="Likes">
            <span className="social-preview__stat-value">{engagement.likes}</span>
            <span className="social-preview__stat-label">likes</span>
          </span>
          <span title="Reposts / shares">
            <span className="social-preview__stat-value">{engagement.reposts}</span>
            <span className="social-preview__stat-label">shares</span>
          </span>
          <span title="Replies">
            <span className="social-preview__stat-value">{engagement.replies}</span>
            <span className="social-preview__stat-label">replies</span>
          </span>
        </div>
      ) : null}
      <div className="social-preview__footer">
        {scheduledAt ? format(parseISO(scheduledAt), 'PPpp') : 'Not scheduled'}
      </div>
    </div>
  )
}
