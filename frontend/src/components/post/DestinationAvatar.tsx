import type { AccountRecord } from '../../types'
import { avatarBackground } from './avatarUtils'

export function DestinationAvatar({
  account,
  compact = false,
  publishedPostUrl,
}: {
  account: AccountRecord
  compact?: boolean
  /** When set, the platform badge links to the published post (e.g. archive). */
  publishedPostUrl?: string
}) {
  const initials = account.username.replace('@', '').slice(0, 2).toUpperCase()
  const badge = (
    <span className="destination-avatar__badge" title={publishedPostUrl ? `Open on ${account.provider}` : account.provider}>
      <img src={`/icons/platforms/${account.provider}.svg`} alt="" />
    </span>
  )
  return (
    <div className={`destination-avatar ${compact ? 'destination-avatar--compact' : ''}`}>
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
        {publishedPostUrl ? (
          <a
            href={publishedPostUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="destination-avatar__badge-link"
            onClick={(event) => event.stopPropagation()}
          >
            {badge}
          </a>
        ) : (
          badge
        )}
      </div>
    </div>
  )
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
