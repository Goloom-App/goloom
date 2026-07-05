import { useState } from 'react'
import { format, parseISO } from 'date-fns'
import { useTranslation } from 'react-i18next'
import { Clock, Eye, Inbox, Pencil, Send, Trash2 } from 'lucide-react'

import { DestinationStack } from '../../components/post/DestinationAvatar'
import { SectionCard } from '../../components/ui'
import { sharedAccountLabels } from '../../schedule'
import { useReviewQueue } from '../../hooks/useReviewQueue'
import type { AccountRecord, ReviewQueueItem, TeamRecord } from '../../types'

interface ReviewQueueViewProps {
  team: TeamRecord
  accounts: AccountRecord[]
  canEdit: boolean
  selectedPostId: string | null
  onSelect: (postId: string) => void
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
  selectedPostId,
  onSelect,
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
    <div className="brand-wizard stack stack--lg" data-testid="review-queue">
      <SectionCard
        icon={<Inbox size={18} />}
        title={t('review.title')}
        subtitle={t('review.hint')}
      >
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
              className={`review-card glass-panel glass-panel--compact ${selectedPostId === item.id ? 'review-card--selected' : ''}`}
              data-testid="review-queue-item"
              data-post-id={item.id}
              onClick={() => onSelect(item.id)}
            >
              <div className="review-card__header">
                <div className="flex-row--center gap-2 flex-wrap">
                  <span className="badge">{t('review.automationBadge')}</span>
                  {item.isOverdue ? (
                    <span className="badge badge--warning" data-testid="review-overdue-badge">
                      {t('review.overdue')}
                    </span>
                  ) : null}
                  {item.rssFeedName ? <span className="hint">{item.rssFeedName}</span> : null}
                </div>
                <div className="flex-row--center gap-2">
                  <DestinationStack accounts={sharedAccountLabels(item, accounts)} />
                  <button
                    type="button"
                    className="btn btn--ghost btn--sm review-card__preview-btn"
                    data-testid="review-open-preview"
                    aria-label={t('review.openPreview')}
                    title={t('review.openPreview')}
                    onClick={() => onSelect(item.id)}
                  >
                    <Eye size={14} />
                  </button>
                </div>
              </div>

              <h3 className="subsection-title review-card__title">{item.title || t('common.untitledPost')}</h3>
              <p className="review-card__content">{item.content}</p>

              <div className="review-card__footer">
                <span className="flex-row--center gap-1 hint review-card__meta">
                  <Clock size={12} />
                  {t('review.suggested')}: {format(parseISO(item.scheduledAt), 'PPp')}
                </span>

                {canEdit ? (
                  <div className="review-card__actions">
                    <button
                      type="button"
                      className="btn btn--secondary btn--sm"
                      data-testid="review-edit"
                      disabled={busyId === item.id}
                      onClick={() => onEdit(item.id)}
                    >
                      <Pencil size={14} /> {t('review.edit')}
                    </button>
                    <span className="review-card__schedule">
                      <label className="field review-card__schedule-field">
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
                    </span>
                    <button
                      type="button"
                      className="btn btn--primary btn--sm"
                      data-testid="review-publish-now"
                      disabled={busyId === item.id}
                      onClick={() => handle(item.id, () => onPublishNow(item))}
                    >
                      <Send size={14} /> {t('review.publishNow')}
                    </button>
                    <button
                      type="button"
                      className="btn btn--ghost btn--sm review-card__discard"
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
              </div>
            </article>
          ))}
        </div>
      )}
      </SectionCard>
    </div>
  )
}
