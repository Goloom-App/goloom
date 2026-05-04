import { format, parseISO } from 'date-fns'
import type { AccountRecord } from '../../types'
import { DestinationAvatar } from './DestinationAvatar'

export type PreviewLayout = 'card' | 'mastodon' | 'bluesky' | 'friendica'

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
}) {
  const resolved = layout ?? resolveLayout(account.provider)
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
