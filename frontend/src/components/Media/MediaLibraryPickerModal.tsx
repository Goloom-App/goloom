import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { BackendMediaItem, createApiClient } from '../../api'
import { translateApiError } from '../../i18n/translateApiError'
import { Icon } from '../../icons'
import { AuthMediaThumb } from './AuthMediaThumb'

type Api = ReturnType<typeof createApiClient>

export function MediaLibraryPickerModal({
  open,
  teamId,
  api,
  authHeader,
  alreadyAttached,
  onClose,
  onAddIds,
  onUploadNew,
}: {
  open: boolean
  teamId: string
  api: Api
  authHeader: string
  alreadyAttached: string[]
  onClose: () => void
  onAddIds: (ids: string[]) => void
  onUploadNew: (file: File) => Promise<string>
}) {
  const { t } = useTranslation()
  const [items, setItems] = useState<BackendMediaItem[]>([])
  const [loading, setLoading] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const reload = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const { items: rows } = await api.listTeamMedia(teamId)
      setItems(rows)
    } catch (e) {
      const raw = e instanceof Error ? e.message : t('status.couldNotLoadLibrary')
      setError(translateApiError(raw, t))
      setItems([])
    } finally {
      setLoading(false)
    }
  }, [api, teamId, t])

  useEffect(() => {
    if (!open) {
      setSelected(new Set())
      return
    }
    void reload()
  }, [open, reload])

  const toggle = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const attachSelected = () => {
    const ids = [...selected].filter((id) => !alreadyAttached.includes(id))
    if (ids.length > 0) {
      onAddIds(ids)
    }
    onClose()
  }

  const runUpload = async (file: File) => {
    setUploading(true)
    setError(null)
    try {
      await onUploadNew(file)
      await reload()
    } catch (e) {
      const raw = e instanceof Error ? e.message : t('common.uploadFailed')
      setError(translateApiError(raw, t))
    } finally {
      setUploading(false)
    }
  }

  if (!open) {
    return null
  }

  const attachedSet = new Set(alreadyAttached)

  return (
    <div className="modal-backdrop composer-media-picker-backdrop" onClick={onClose}>
      <div className="glass-panel composer-media-picker" onClick={(e) => e.stopPropagation()} role="dialog" aria-labelledby="composer-media-picker-title">
        <header className="composer-media-picker__head">
          <div>
            <p className="eyebrow">{t('media.libraryTitle')}</p>
            <h2 id="composer-media-picker-title">{t('media.pickerTitle')}</h2>
            <p className="hint">{t('media.pickerHint')}</p>
          </div>
          <button type="button" className="button button--secondary" onClick={onClose}>
            {t('common.close')}
          </button>
        </header>

        <div className="composer-media-picker__actions">
          <input
            ref={fileRef}
            type="file"
            accept="image/*,video/*"
            className="composer-media-picker__hidden-input"
            onChange={(ev) => {
              const f = ev.target.files?.[0]
              ev.target.value = ''
              if (f) {
                void runUpload(f)
              }
            }}
          />
          <button type="button" className="button button--secondary" disabled={uploading || loading} onClick={() => fileRef.current?.click()}>
            <Icon name="plus" className="inline-icon" />
            <span>{uploading ? t('common.uploading') : t('media.uploadNewFile')}</span>
          </button>
          <button type="button" className="button button--primary" disabled={loading || uploading || selected.size === 0} onClick={attachSelected}>
            {t('media.addSelectedToPost', { count: selected.size })}
          </button>
        </div>

        {error ? <p className="status-banner__error">{error}</p> : null}
        {loading ? <p className="hint">{t('common.loadingLibrary')}</p> : null}

        {!loading && items.length === 0 ? <p className="hint">{t('media.noFilesInLibrary')}</p> : null}

        <div className="composer-media-picker__grid" aria-label={t('media.libraryItemsPickerAria')}>
          {items.map((entry) => {
            const sel = selected.has(entry.id)
            const onPost = attachedSet.has(entry.id)
            return (
              <button
                key={entry.id}
                type="button"
                className={`composer-media-picker__tile ${sel ? 'composer-media-picker__tile--selected' : ''}`}
                onClick={() => toggle(entry.id)}
              >
                <span className="composer-media-picker__thumb">
                  {entry.mime_type.startsWith('image/') ? (
                    <AuthMediaThumb
                      url={api.mediaPreviewUrl(teamId, entry.id)}
                      authHeader={authHeader}
                      alt={entry.filename}
                      className="composer-media-picker__img"
                    />
                  ) : (
                    <span className="composer-media-picker__thumb-film">
                      <Icon name="film" />
                    </span>
                  )}
                  {onPost ? <span className="composer-media-picker__badge">{t('common.onPost')}</span> : null}
                </span>
                <span className="composer-media-picker__name" title={entry.filename}>
                  {entry.filename}
                </span>
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}
