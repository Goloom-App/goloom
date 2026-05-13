import type { AccountRecord, PostRecord } from '../../types'
import { PostCard } from '../../components/post/PostCard'

export function ArchiveView({
  archivedPosts,
  expandedPostId,
  setExpandedPostId,
  openEditor,
  duplicatePost,
  deletePost,
  onPreview,
  accounts,
}: {
  archivedPosts: PostRecord[]
  expandedPostId: string | null
  setExpandedPostId: (id: string) => void
  openEditor: (postId: string) => void
  duplicatePost: (postId: string) => void
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
          active={expandedPostId === post.id}
          onClick={() => setExpandedPostId(post.id)}
          onEdit={() => openEditor(post.id)}
          onDuplicate={() => duplicatePost(post.id)}
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
