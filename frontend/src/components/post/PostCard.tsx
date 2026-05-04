import { format, parseISO } from 'date-fns'
import { Icon } from '../../icons'
import type { AccountRecord, PostRecord } from '../../types'
import { sharedAccountLabels } from '../../schedule'
import { DestinationStack } from './DestinationAvatar'

export function PostCard({
  post,
  active,
  onClick,
  onEdit,
  onDelete,
  accounts,
  isArchived = false,
  publishedLinks,
}: {
  post: PostRecord
  active: boolean
  onClick: () => void
  onEdit: () => void
  onDelete: () => void
  accounts: AccountRecord[]
  isArchived?: boolean
  publishedLinks?: Record<string, string>
}) {
  return (
    <article className={`post-card ${active ? 'post-card--active' : ''}`} onClick={onClick}>
      <div className="post-card__header">
        <span className="post-card__meta">{format(parseISO(post.scheduledAt), 'HH:mm')}</span>
        <DestinationStack accounts={sharedAccountLabels(post, accounts)} publishedLinks={isArchived ? publishedLinks : undefined} />
      </div>
      <h3 className="post-card__title">
        {post.status === 'draft' ? <span className="post-card__badge">Draft</span> : null}
        {post.title || 'Untitled Post'}
      </h3>
      <p className="post-card__content">{post.content}</p>
      {active && (
        <div className="inline-cluster" style={{ marginTop: '1rem' }}>
          <button
            type="button"
            className="button button--secondary"
            onClick={(e) => {
              e.stopPropagation()
              onEdit()
            }}
          >
            <Icon name="edit" className="inline-icon" />
            <span>Edit</span>
          </button>
          {!isArchived && (
            <button
              type="button"
              className="button button--secondary"
              onClick={(e) => {
                e.stopPropagation()
                onDelete()
              }}
            >
              <Icon name="trash" className="inline-icon" />
              <span>Delete</span>
            </button>
          )}
        </div>
      )}
    </article>
  )
}
