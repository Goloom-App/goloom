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

export function SocialPreview({
  account,
  content,
  scheduledAt,
  theme,
  publishedPostUrl,
  layout,
}: {
  account: AccountRecord
  content: string
  scheduledAt: string
  theme: 'dark' | 'light'
  publishedPostUrl?: string
  /** When omitted, layout follows account provider. */
  layout?: PreviewLayout
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
      <div className="social-preview__footer">
        {scheduledAt ? format(parseISO(scheduledAt), 'PPpp') : 'Not scheduled'}
      </div>
    </div>
  )
}
