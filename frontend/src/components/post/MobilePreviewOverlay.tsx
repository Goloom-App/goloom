import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'

import { Icon } from '../../icons'
import { SocialPreview } from './SocialPreview'
import { engagementForAccount } from '../../postMetrics'
import { sharedAccountLabels } from '../../schedule'
import type { BackendPostMetric } from '../../api'
import type { AccountRecord, PostRecord, PostVersionRecord } from '../../types'

interface MobilePreviewOverlayProps {
  post: PostRecord
  accounts: AccountRecord[]
  postVersions: PostVersionRecord[]
  previewPostMetrics: BackendPostMetric[]
  theme: 'light' | 'dark'
  onClose: () => void
  onDelete: (postId: string) => void
  onEdit: (postId: string) => void
  onDuplicate: (postId: string) => void
}

/** Bottom-sheet preview for mobile; swipe down to dismiss. */
export function MobilePreviewOverlay({
  post,
  accounts,
  postVersions,
  previewPostMetrics,
  theme,
  onClose,
  onDelete,
  onEdit,
  onDuplicate,
}: MobilePreviewOverlayProps) {
  const { t } = useTranslation()
  const [touchStart, setTouchStart] = useState<number | null>(null)
  const [translateY, setTranslateY] = useState(0)

  const close = () => {
    setTranslateY(0)
    onClose()
  }

  return (
    <div
      className="mobile-preview-overlay"
      data-testid="mobile-preview-overlay"
      style={{ opacity: Math.max(0, 1 - translateY / 300) }}
      onClick={close}
    >
      <div
        className="mobile-preview-container glass-panel"
        style={{
          transform: `translateY(${translateY}px)`,
          transition: touchStart === null ? 'transform 0.3s cubic-bezier(0.2, 0.8, 0.2, 1)' : 'none',
        }}
        onClick={(e) => e.stopPropagation()}
        onTouchStart={(e) => setTouchStart(e.touches[0].clientY)}
        onTouchMove={(e) => {
          if (touchStart === null) return
          const delta = e.touches[0].clientY - touchStart
          if (delta > 0) setTranslateY(delta)
        }}
        onTouchEnd={() => {
          if (translateY > 120) {
            onClose()
          }
          setTouchStart(null)
          setTranslateY(0)
        }}
      >
        <header className="mobile-preview-header">
          <div className="flex-row--center gap-3">
            <button type="button" className="btn btn--ghost btn--xs" onClick={close}>
              <X size={20} />
            </button>
            <h3 style={{ margin: 0 }}>{t('common.preview')}</h3>
          </div>
          <div className="flex-row--center gap-2">
            <button
              type="button"
              className="button button--secondary button--sm"
              style={{ color: 'var(--danger)' }}
              onClick={() => {
                if (window.confirm(t('common.confirmDeletePost'))) {
                  close()
                  onDelete(post.id)
                }
              }}
            >
              <Icon name="trash" className="inline-icon" />
              <span className="desktop-only">{t('common.delete')}</span>
            </button>
            {post.status !== 'posted' && post.source !== 'imported' && (
              <button
                type="button"
                className="button button--secondary button--sm"
                data-testid="preview-edit-button"
                onClick={() => {
                  close()
                  onEdit(post.id)
                }}
              >
                <Icon name="edit" className="inline-icon" />
                <span>{t('common.edit')}</span>
              </button>
            )}
            {post.status === 'posted' && (
              <button
                type="button"
                className="button button--secondary button--sm"
                onClick={() => {
                  close()
                  onDuplicate(post.id)
                }}
              >
                <Icon name="plus" className="inline-icon" />
                <span>Re-use</span>
              </button>
            )}
          </div>
        </header>
        <div className="mobile-preview-scrollable">
          {sharedAccountLabels(post, accounts).map((account) => (
            <SocialPreview
              key={account.id}
              account={account}
              content={postVersions.find((v) => v.postId === post.id && v.accountId === account.id)?.content || post.content}
              scheduledAt={post.scheduledAt}
              theme={theme}
              publishedPostUrl={post.status === 'posted' ? post.publishedLinks?.[account.id] : undefined}
              engagement={post.status === 'posted' ? engagementForAccount(previewPostMetrics, account.id) : null}
            />
          ))}
        </div>
      </div>
    </div>
  )
}
