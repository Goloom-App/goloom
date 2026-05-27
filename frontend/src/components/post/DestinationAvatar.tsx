import type { AccountRecord } from '../../types'
import { avatarBackground } from './avatarUtils'

export function DestinationAvatar({
  account,
  compact = false,
  publishedPostUrl,
  error = false,
}: {
  account: AccountRecord
  compact?: boolean
  /** When set, the platform badge links to the published post (e.g. archive). */
  publishedPostUrl?: string
  /** Highlight when this destination exceeds its character limit. */
  error?: boolean
}) {
  const initials = account.username.replace('@', '').slice(0, 2).toUpperCase()
  const badge = (
    <span className="destination-avatar__badge" title={publishedPostUrl ? `Open on ${account.provider}` : account.provider}>
      <img src={`/icons/platforms/${account.provider}.svg`} alt="" />
    </span>
  )
  const avatar = (
    <div className={`destination-avatar ${compact ? 'destination-avatar--compact' : ''} ${error ? 'destination-avatar--error' : ''}`}>
      <div className="destination-avatar__disk">
        <div className="destination-avatar__inner">
          {account.avatarUrl ? (
            <img className="destination-avatar__photo" src={account.avatarUrl} alt="" referrerPolicy="no-referrer" />
          ) : (
            <div className="destination-avatar__fallback" style={{ background: avatarBackground(account.color) }} aria-hidden="true">
              {initials}
            </div>
          )}
        </div>
        {badge}
      </div>
    </div>
  )

  if (publishedPostUrl) {
    return (
      <a
        href={publishedPostUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="destination-avatar__badge-link"
        title={`Open on ${account.provider}`}
        onClick={(event) => event.stopPropagation()}
      >
        {avatar}
      </a>
    )
  }

  return avatar
}

export function DestinationStack({
  accounts,
  publishedLinks,
}: {
  accounts: AccountRecord[]
  publishedLinks?: Record<string, string>
}) {
  return (
    <div className="destination-stack">
      {accounts.map((account) => (
        <DestinationAvatar
          key={account.id}
          account={account}
          compact
          publishedPostUrl={publishedLinks?.[account.id]}
        />
      ))}
    </div>
  )
}
