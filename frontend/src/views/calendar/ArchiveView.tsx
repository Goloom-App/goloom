import { useTranslation } from 'react-i18next'

import { PostCard } from '../../components/post/PostCard'
import type { AccountRecord, PostRecord } from '../../types'

export function ArchiveView({
  archivedPosts,
  deletePost,
  onPreview,
  accounts,
}: {
  archivedPosts: PostRecord[]
  deletePost: (postId: string) => void
  onPreview?: (postId: string) => void
  accounts: AccountRecord[]
}) {
  const { t } = useTranslation()

  return (
    <div className="archive-view" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-8)' }}>
      {archivedPosts.map((post) => (
        <PostCard
          key={post.id}
          post={post}
          onDelete={() => void deletePost(post.id)}
          onPreview={onPreview ? () => onPreview(post.id) : undefined}
          accounts={accounts}
          isArchived
          publishedLinks={post.publishedLinks}
        />
      ))}
      {archivedPosts.length === 0 ? (
        <div className="empty-state">
          <h3>{t('calendar.archive.noPublishedTitle')}</h3>
          <p className="hint">{t('calendar.archive.hint')}</p>
        </div>
      ) : null}
    </div>
  )
}
