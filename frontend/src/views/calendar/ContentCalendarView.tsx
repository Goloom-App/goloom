import { addMonths, format, parseISO, startOfMonth, subMonths } from 'date-fns'
import { Icon } from '../../icons'
import type { PostRecord } from '../../types'
import { calendarCellsForMonth } from './calendarUtils'

export function ContentCalendarView({
  contentCalendarMonth,
  setContentCalendarMonth,
  contentCalendarCells,
  plannedPostsForContentCalendar,
  canEditScheduledPosts,
  calendarDragOverKey,
  setCalendarDragOverKey,
  setExpandedPostId,
  openEditor,
  handleCalendarPostDrop,
}: {
  contentCalendarMonth: Date
  setContentCalendarMonth: (updater: (m: Date) => Date) => void
  contentCalendarCells: ReturnType<typeof calendarCellsForMonth>
  plannedPostsForContentCalendar: PostRecord[]
  canEditScheduledPosts: boolean
  calendarDragOverKey: string | null
  setCalendarDragOverKey: (key: string | null | ((c: string | null) => string | null)) => void
  setExpandedPostId: (id: string) => void
  openEditor: (postId: string) => void
  handleCalendarPostDrop: (postId: string, targetDay: Date) => void | Promise<void>
}) {
  return (
    <div className="content-calendar-view">
      <div className="content-calendar__toolbar glass-panel">
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
                        onClick={() => setExpandedPostId(post.id)}
                        onKeyDown={(event) => {
                          if (event.key === 'Enter' || event.key === ' ') {
                            event.preventDefault()
                            setExpandedPostId(post.id)
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
                          aria-label="Edit post"
                          onClick={(event) => {
                            event.stopPropagation()
                            openEditor(post.id)
                          }}
                        >
                          <Icon name="edit" className="inline-icon" />
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
      {plannedPostsForContentCalendar.length === 0 ? (
        <p className="hint" style={{ marginTop: '1rem' }}>
          No scheduled posts for this workspace. Create a post from the schedule view or the composer.
        </p>
      ) : null}
    </div>
  )
}
