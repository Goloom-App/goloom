import { format, parseISO } from 'date-fns'
import { useTranslation } from 'react-i18next'

import { PostCard } from '../../components/post/PostCard'
import type { AccountRecord, PostRecord } from '../../types'
import { groupUpcomingIntoMonths } from './calendarUtils'

export function ScheduleView({
  upcomingPosts,
  deletePost,
  onPreview,
  accounts,
}: {
  upcomingPosts: PostRecord[]
  deletePost: (postId: string) => void
  onPreview?: (postId: string) => void
  accounts: AccountRecord[]
}) {
  const { t } = useTranslation()

  if (upcomingPosts.length === 0) {
    return (
      <div className="timeline-view">
        <div className="empty-state">
          <h3>{t('calendar.schedule.noUpcomingTitle')}</h3>
          <p className="hint">{t('calendar.schedule.createPostHint')}</p>
        </div>
      </div>
    )
  }
  return (
    <div className="timeline-view">
      {groupUpcomingIntoMonths(upcomingPosts).map((month) => (
        <div key={month.monthKey} className="timeline-month-block">
          <h2 className="timeline-month-heading">{month.monthLabel}</h2>
          {month.days.map((group) => (
            <section key={group.key} className="timeline-day-section">
              <p
                className="eyebrow"
                style={{
                  marginBottom: '1rem',
                  fontWeight: group.posts.length > 1 ? 700 : 500,
                }}
              >
                {format(parseISO(group.posts[0].scheduledAt), 'EEEE, d MMMM')}
                {group.posts.length > 1 ? ` · ${t('calendar.schedule.postsOnDay', { count: group.posts.length })}` : null}
              </p>
              <div className="posts-grid">
                {group.posts.map((post) => (
                  <PostCard
                    key={post.id}
                    post={post}
                    onDelete={() => void deletePost(post.id)}
                    onPreview={onPreview ? () => onPreview(post.id) : undefined}
                    accounts={accounts}
                  />
                ))}
              </div>
            </section>
          ))}
        </div>
      ))}
    </div>
  )
}
