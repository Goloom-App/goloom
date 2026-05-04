import type { AccountRecord, PostRecord } from '../../types'
import { PostCard } from '../../components/post/PostCard'

export function ArchiveView({
  archivedPosts,
  expandedPostId,
  setExpandedPostId,
  openEditor,
  deletePost,
  accounts,
}: {
  archivedPosts: PostRecord[]
  expandedPostId: string | null
  setExpandedPostId: (id: string) => void
  openEditor: (postId: string) => void
  deletePost: (postId: string) => void
  accounts: AccountRecord[]
}) {
  return (
    <div className="archive-view">
      {archivedPosts.map((post) => (
        <PostCard
          key={post.id}
          post={post}
          active={expandedPostId === post.id}
          onClick={() => setExpandedPostId(post.id)}
          onEdit={() => openEditor(post.id)}
          onDelete={() => void deletePost(post.id)}
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
