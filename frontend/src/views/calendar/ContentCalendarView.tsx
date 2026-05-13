import { addMonths, format, parseISO, startOfMonth, subMonths } from 'date-fns'
import { useEffect, useMemo, useState, type TouchEvent } from 'react'
import { Icon } from '../../icons'
import type { AccountRecord, PostRecord } from '../../types'
import { PostCard } from '../../components/post/PostCard'
import { calendarCellsForMonth } from './calendarUtils'

export function ContentCalendarView({
  isMobile,
  contentCalendarMonth,
  setContentCalendarMonth,
  contentCalendarCells,
  plannedPostsForContentCalendar,
  canEditScheduledPosts,
  calendarDragOverKey,
  setCalendarDragOverKey,
  deletePost,
  onPreview,
  accounts,
  handleCalendarPostDrop,
}: {
  isMobile: boolean
  contentCalendarMonth: Date
  setContentCalendarMonth: (updater: (m: Date) => Date) => void
  contentCalendarCells: ReturnType<typeof calendarCellsForMonth>
  plannedPostsForContentCalendar: PostRecord[]
  canEditScheduledPosts: boolean
  calendarDragOverKey: string | null
  setCalendarDragOverKey: (key: string | null | ((c: string | null) => string | null)) => void
  deletePost: (postId: string) => void
  onPreview?: (postId: string) => void
  accounts: AccountRecord[]
  handleCalendarPostDrop: (postId: string, targetDay: Date) => void | Promise<void>
}) {
  const [viewMode, setViewMode] = useState<'grid' | 'list'>(isMobile ? 'list' : 'grid')
  const [calendarTouchStart, setCalendarTouchStart] = useState<{ x: number; y: number } | null>(null)

  useEffect(() => {
    setViewMode(isMobile ? 'list' : 'grid')
  }, [isMobile])

  const groupedByDay = useMemo(() => {
    const map = new Map<string, { day: Date; posts: PostRecord[] }>()
    for (const post of plannedPostsForContentCalendar) {
      const scheduled = parseISO(post.scheduledAt)
      if (scheduled.getMonth() !== contentCalendarMonth.getMonth() || scheduled.getFullYear() !== contentCalendarMonth.getFullYear()) {
        continue
      }
      const key = format(scheduled, 'yyyy-MM-dd')
      const existing = map.get(key)
      if (existing) {
        existing.posts.push(post)
      } else {
        map.set(key, { day: scheduled, posts: [post] })
      }
    }
    return [...map.values()]
      .map((group) => ({
        ...group,
        posts: [...group.posts].sort((left, right) => parseISO(left.scheduledAt).getTime() - parseISO(right.scheduledAt).getTime()),
      }))
      .sort((left, right) => left.day.getTime() - right.day.getTime())
  }, [contentCalendarMonth, plannedPostsForContentCalendar])

  function onTouchStart(event: TouchEvent) {
    const point = event.touches[0]
    if (point) {
      setCalendarTouchStart({ x: point.clientX, y: point.clientY })
    }
  }

  function onTouchEnd(event: TouchEvent) {
    if (!calendarTouchStart) {
      return
    }
    const point = event.changedTouches[0]
    if (!point) {
      setCalendarTouchStart(null)
      return
    }

    const deltaX = point.clientX - calendarTouchStart.x
    const deltaY = point.clientY - calendarTouchStart.y
    const swipeDistance = Math.abs(deltaX)
    const verticalDistance = Math.abs(deltaY)
    if (swipeDistance > 56 && swipeDistance > verticalDistance * 1.4) {
      setContentCalendarMonth((current) => (deltaX > 0 ? startOfMonth(subMonths(current, 1)) : startOfMonth(addMonths(current, 1))))
    }
    setCalendarTouchStart(null)
  }

  function onTouchCancel() {
    setCalendarTouchStart(null)
  }

  return (
    <div
      className="content-calendar-view"
      onTouchStart={isMobile ? onTouchStart : undefined}
      onTouchEnd={isMobile ? onTouchEnd : undefined}
      onTouchCancel={isMobile ? onTouchCancel : undefined}
    >
      <div className="content-calendar__toolbar glass-panel">
        <div className="content-calendar__toolbar-left">
          <button
            type="button"
            className="button button--secondary content-calendar__nav-btn content-calendar__nav-btn--text"
            onClick={() => setContentCalendarMonth((m) => startOfMonth(subMonths(m, 1)))}
            aria-label="Previous month"
          >
            &lt;
          </button>
          <h2 className="content-calendar__month-title">{format(contentCalendarMonth, 'MMMM yyyy')}</h2>
          <button
            type="button"
            className="button button--secondary content-calendar__nav-btn content-calendar__nav-btn--text"
            onClick={() => setContentCalendarMonth((m) => startOfMonth(addMonths(m, 1)))}
            aria-label="Next month"
          >
            &gt;
          </button>
        </div>
        <div className="content-calendar__toolbar-right">
          <div className="content-calendar__view-toggle">
            <button type="button" className={`button button--secondary ${viewMode === 'grid' ? 'content-calendar__view-toggle-btn--active' : ''}`} onClick={() => setViewMode('grid')}>
              Grid
            </button>
            <button type="button" className={`button button--secondary ${viewMode === 'list' ? 'content-calendar__view-toggle-btn--active' : ''}`} onClick={() => setViewMode('list')}>
              List
            </button>
          </div>
        </div>
      </div>
      {viewMode === 'grid' ? (
      <div className="content-calendar__grid glass-panel" role="grid" aria-label="Scheduled posts by day">
        <div className="content-calendar__weekdays" role="row">
          {['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map((d) => (
            <div key={d} className="content-calendar__weekday" role="columnheader">
              {d}
            </div>
          ))}
        </div>
        <div className="content-calendar__cells">
          {contentCalendarCells.map(({ day, posts: dayPosts, inMonth }) => {
            const dayKey = format(day, 'yyyy-MM-dd')
            const isDropTarget = calendarDragOverKey === dayKey
            return (
              <div
                key={day.toISOString()}
                className={`content-calendar__cell ${inMonth ? '' : 'content-calendar__cell--muted'} ${isDropTarget ? 'content-calendar__cell--drop-target' : ''}`}
                role="gridcell"
                onDragOver={(event) => {
                  if (!canEditScheduledPosts) {
                    return
                  }
                  event.preventDefault()
                  event.dataTransfer.dropEffect = 'move'
                  setCalendarDragOverKey(dayKey)
                }}
                onDragLeave={(event) => {
                  if (!event.currentTarget.contains(event.relatedTarget as Node)) {
                    setCalendarDragOverKey((current) => (current === dayKey ? null : current))
                  }
                }}
                onDrop={(event) => {
                  event.preventDefault()
                  setCalendarDragOverKey(null)
                  const id = event.dataTransfer.getData('application/x-goloom-post-id')
                  if (id) {
                    void handleCalendarPostDrop(id, day)
                  }
                }}
              >
                <span className="content-calendar__day-num">{format(day, 'd')}</span>
                <div className="content-calendar__post-chips">
                  {dayPosts.map((post) => (
                    <div key={post.id} className="content-calendar__post-chip-row">
                      <div
                        role="button"
                        tabIndex={0}
                        className={`content-calendar__post-chip ${canEditScheduledPosts ? 'content-calendar__post-chip--draggable' : ''}`}
                        draggable={canEditScheduledPosts}
                        onDragStart={(event) => {
                          if (!canEditScheduledPosts) {
                            return
                          }
                          event.dataTransfer.setData('application/x-goloom-post-id', post.id)
                          event.dataTransfer.effectAllowed = 'move'
                        }}
                        onDragEnd={() => setCalendarDragOverKey(null)}
                        onClick={() => onPreview?.(post.id)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter' || event.key === ' ') {
                            event.preventDefault()
                            onPreview?.(post.id)
                          }
                        }}
                      >
                        <span className="content-calendar__post-time">{format(parseISO(post.scheduledAt), 'HH:mm')}</span>
                        <span className="content-calendar__post-title">{post.title || 'Untitled'}</span>
                      </div>
                      {canEditScheduledPosts ? (
                        <button
                          type="button"
                          className="content-calendar__chip-edit"
                          aria-label="Preview post"
                          onClick={(event) => {
                            event.stopPropagation()
                            onPreview?.(post.id)
                          }}
                        >
                          <Icon name="eye" className="inline-icon" />
                        </button>
                      ) : null}
                    </div>
                  ))}
                </div>
              </div>
            )
          })}
        </div>
      </div>
      ) : (
        <div className="content-calendar__list" role="list" aria-label="Scheduled posts list">
          {groupedByDay.length === 0 ? (
            <p className="hint">No scheduled posts for this month.</p>
          ) : (
            groupedByDay.map((group) => (
              <section key={format(group.day, 'yyyy-MM-dd')} className="content-calendar__list-day">
                <h3 className="content-calendar__list-day-title">{format(group.day, 'EEEE, MMM d')}</h3>
                <div className="content-calendar__list-posts">
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
            ))
          )}
        </div>
      )}
      {plannedPostsForContentCalendar.length === 0 ? (
        <p className="hint mt-1">
          No scheduled posts for this workspace. Create a post from the schedule view or the composer.
        </p>
      ) : null}
    </div>
  )
}
