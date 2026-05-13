import type { AccountRecord, PostRecord } from '../../types'
import { PostCard } from '../../components/post/PostCard'

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
          <h3>No published posts yet</h3>
          <p className="hint">Posted items appear here once the scheduler publishes them.</p>
        </div>
      ) : null}
    </div>
  )
}
