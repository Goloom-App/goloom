import { useState } from 'react'
import { format, parseISO } from 'date-fns'
import { useTranslation } from 'react-i18next'
import { Clock, Inbox, Pencil, Send, Trash2 } from 'lucide-react'

import { DestinationStack } from '../../components/post/DestinationAvatar'
import { sharedAccountLabels } from '../../schedule'
import { useReviewQueue } from '../../hooks/useReviewQueue'
import type { AccountRecord, ReviewQueueItem, TeamRecord } from '../../types'

interface ReviewQueueViewProps {
  team: TeamRecord
  accounts: AccountRecord[]
  canEdit: boolean
  onEdit: (postId: string) => void
  onPublishNow: (item: ReviewQueueItem) => Promise<void>
  onSchedule: (item: ReviewQueueItem, scheduledAt: string) => Promise<void>
  onDiscard: (postId: string) => Promise<void>
}

function toInputDateTime(iso: string) {
  const d = parseISO(iso)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function ReviewQueueView({
  team,
  accounts,
  canEdit,
  onEdit,
  onPublishNow,
  onSchedule,
  onDiscard,
}: ReviewQueueViewProps) {
  const { t } = useTranslation()
  const { data: items, isLoading, refetch } = useReviewQueue(team.id)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [scheduleAt, setScheduleAt] = useState<Record<string, string>>({})
  const [error, setError] = useState<string | null>(null)

  if (isLoading) {
    return <p className="hint">{t('review.loading')}</p>
  }

  const handle = async (postId: string, action: () => Promise<void>) => {
    setError(null)
    setBusyId(postId)
    try {
      await action()
      await refetch()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('review.actionFailed'))
    } finally {
      setBusyId(null)
    }
  }

  return (
    <div className="glass-panel stack stack--lg" data-testid="review-queue">
      <div>
        <h2 className="section-card__title flex-row--center gap-2">
          <Inbox size={20} />
          {t('review.title')}
        </h2>
        <p className="hint">{t('review.hint')}</p>
      </div>

      {error ? <p className="status-banner__error">{error}</p> : null}

      {!items?.length ? (
        <p className="hint" data-testid="review-queue-empty">
          {t('review.empty')}
        </p>
      ) : (
        <div className="stack stack--sm">
          {items.map((item) => (
            <article
              key={item.id}
              className="glass-panel glass-panel--compact stack stack--sm"
              data-testid="review-queue-item"
              data-post-id={item.id}
            >
              <div className="flex-row--between flex-wrap gap-2">
                <div className="stack stack--xs">
                  <div className="flex-row--center gap-2 flex-wrap">
                    <span className="badge">{t('review.automationBadge')}</span>
                    {item.isOverdue ? (
                      <span className="badge badge--warning" data-testid="review-overdue-badge">
                        {t('review.overdue')}
                      </span>
                    ) : null}
                    {item.rssFeedName ? <span className="hint">{item.rssFeedName}</span> : null}
                  </div>
                  <h3 className="subsection-title">{item.title || t('common.untitledPost')}</h3>
                  <p className="hint" style={{ fontSize: '0.85rem' }}>
                    {item.content}
                  </p>
                </div>
                <DestinationStack accounts={sharedAccountLabels(item, accounts)} />
              </div>

              <div className="flex-row--center gap-1 hint" style={{ fontSize: '0.8rem' }}>
                <Clock size={12} />
                <span>
                  {t('review.suggested')}: {format(parseISO(item.scheduledAt), 'PPp')}
                </span>
              </div>

              {canEdit ? (
                <div className="flex-row--center gap-2 flex-wrap">
                  <button
                    type="button"
                    className="btn btn--secondary btn--sm"
                    data-testid="review-edit"
                    disabled={busyId === item.id}
                    onClick={() => onEdit(item.id)}
                  >
                    <Pencil size={14} /> {t('review.edit')}
                  </button>
                  <button
                    type="button"
                    className="btn btn--primary btn--sm"
                    data-testid="review-publish-now"
                    disabled={busyId === item.id}
                    onClick={() => handle(item.id, () => onPublishNow(item))}
                  >
                    <Send size={14} /> {t('review.publishNow')}
                  </button>
                  <label className="field" style={{ marginBottom: 0 }}>
                    <span className="sr-only">{t('review.scheduleAt')}</span>
                    <input
                      type="datetime-local"
                      data-testid="review-schedule-at"
                      value={scheduleAt[item.id] ?? toInputDateTime(item.scheduledAt)}
                      onChange={(e) => setScheduleAt((prev) => ({ ...prev, [item.id]: e.target.value }))}
                    />
                  </label>
                  <button
                    type="button"
                    className="btn btn--secondary btn--sm"
                    data-testid="review-schedule"
                    disabled={busyId === item.id}
                    onClick={() =>
                      handle(item.id, () =>
                        onSchedule(item, new Date(scheduleAt[item.id] ?? toInputDateTime(item.scheduledAt)).toISOString()),
                      )
                    }
                  >
                    {t('review.schedule')}
                  </button>
                  <button
                    type="button"
                    className="btn btn--ghost btn--sm"
                    data-testid="review-discard"
                    disabled={busyId === item.id}
                    onClick={() => {
                      if (!window.confirm(t('review.discardConfirm'))) return
                      void handle(item.id, () => onDiscard(item.id))
                    }}
                  >
                    <Trash2 size={14} /> {t('review.discard')}
                  </button>
                </div>
              ) : null}
            </article>
          ))}
        </div>
      )}
    </div>
  )
}
