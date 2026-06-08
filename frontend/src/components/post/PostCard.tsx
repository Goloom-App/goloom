import { format, parseISO } from 'date-fns'
import { useState, useRef, type TouchEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Icon } from '../../icons'
import type { AccountRecord, PostRecord } from '../../types'
import { sharedAccountLabels } from '../../schedule'
import { DestinationStack } from './DestinationAvatar'

export function PostCard({
  post,
  onDelete,
  onPreview,
  accounts,
  isArchived = false,
  publishedLinks,
}: {
  post: PostRecord
  onDelete: () => void
  onPreview?: () => void
  accounts: AccountRecord[]
  isArchived?: boolean
  publishedLinks?: Record<string, string>
}) {
  const { t } = useTranslation()
  const [swipeX, setSwipeX] = useState(0)
  const touchStart = useRef<number | null>(null)
  const isSwiping = useRef(false)

  const onTouchStart = (e: TouchEvent) => {
    touchStart.current = e.touches[0].clientX
    isSwiping.current = true
  }

  const onTouchMove = (e: TouchEvent) => {
    if (touchStart.current === null) return
    const delta = e.touches[0].clientX - touchStart.current
    if (delta < 0) {
      setSwipeX(delta)
    }
  }

  const onTouchEnd = () => {
    if (swipeX < -100) {
      if (window.confirm(t('common.confirmDeletePostShort'))) {
        onDelete()
      }
    }
    setSwipeX(0)
    touchStart.current = null
    isSwiping.current = false
  }

  return (
    <div className="post-card-container" style={{ position: 'relative', overflow: 'hidden', borderRadius: 'var(--radius-lg)' }}>
      <div
        className="post-card-delete-bg"
        style={{
          position: 'absolute',
          inset: 0,
          background: 'var(--danger)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          paddingRight: 'var(--space-6)',
          color: 'white',
          opacity: Math.min(1, Math.abs(swipeX) / 100),
        }}
      >
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '4px' }}>
          <Icon name="trash" style={{ width: '24px', height: '24px' }} />
          <span style={{ fontSize: '0.7rem', fontWeight: 700 }}>{t('common.deleteSwipe')}</span>
        </div>
      </div>
      <article
        className="post-card"
        data-testid="post-card"
        style={{
          transform: `translateX(${swipeX}px)`,
          transition: isSwiping.current ? 'none' : 'transform 0.3s var(--ease-out)',
          position: 'relative',
          zIndex: 1,
        }}
        onClick={(e) => {
          e.stopPropagation()
          onPreview?.()
        }}
        onTouchStart={onTouchStart}
        onTouchMove={onTouchMove}
        onTouchEnd={onTouchEnd}
      >
        <div className="post-card__header">
          <span className="post-card__meta">
            {isArchived ? format(parseISO(post.scheduledAt), 'EEEE, MMM d · HH:mm') : format(parseISO(post.scheduledAt), 'HH:mm')}
          </span>
          <div className="flex-row--center gap-2">
            <DestinationStack accounts={sharedAccountLabels(post, accounts)} publishedLinks={isArchived ? publishedLinks : undefined} />
          </div>
        </div>
        <h3 className="post-card__title">
          {post.status === 'draft' ? <span className="post-card__badge">{t('common.draft')}</span> : null}
          {post.source === 'imported' ? <span className="post-card__badge">{t('common.importedBadge')}</span> : null}
          {post.title || t('common.untitledPost')}
        </h3>
        <p className="post-card__content">{post.content}</p>
      </article>
    </div>
  )
}
