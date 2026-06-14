import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { BackendMediaItem, createApiClient } from '../../api'
import { translateApiError } from '../../i18n/translateApiError'
import { Icon } from '../../icons'
import { AuthMediaThumb } from '../Media/AuthMediaThumb'
import { MediaLibraryPickerModal } from '../Media/MediaLibraryPickerModal'

type Api = ReturnType<typeof createApiClient>

export function ComposerMedia({
  mediaIds,
  libraryById,
  onAddIds,
  onRemove,
  onUpload,
  teamId,
  api,
  authHeader,
  uploadLabel,
  disabled,
}: {
  mediaIds: string[]
  /** Filenames / mime from last library fetch (fallback if unknown). */
  libraryById: Record<string, Pick<BackendMediaItem, 'filename' | 'mime_type'>>
  onAddIds: (ids: string[]) => void
  onRemove: (id: string) => void
  onUpload?: (file: File) => Promise<string>
  teamId?: string | null
  api?: Api | null
  authHeader?: string
  /** Shown when upload unavailable (no team / token). */
  uploadLabel?: string
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragOver, setDragOver] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)

  const pickerReady = Boolean(teamId && api && authHeader?.trim())
  const canUpload = Boolean(onUpload) && !disabled

  const runUpload = useCallback(
    async (file: File) => {
      if (!onUpload || disabled) {
        return
      }
      setError(null)
      setUploading(true)
      try {
        const id = await onUpload(file)
        if (id) {
          onAddIds([id])
        }
      } catch (e) {
        setError(translateApiError(e instanceof Error ? e.message : t('common.uploadFailed'), t))
      } finally {
        setUploading(false)
      }
    },
    [disabled, onAddIds, onUpload, t],
  )

  const onInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const f = event.target.files?.[0]
    event.target.value = ''
    if (f) {
      void runUpload(f)
    }
  }

  const onDrop = (event: React.DragEvent) => {
    event.preventDefault()
    setDragOver(false)
    if (!canUpload || uploading) {
      return
    }
    const f = event.dataTransfer.files[0]
    if (f) {
      void runUpload(f)
    }
  }

  // The add tile opens the library picker when connected; otherwise it triggers a direct upload.
  const onAddTile = () => {
    if (pickerReady) {
      setPickerOpen(true)
    } else if (canUpload) {
      inputRef.current?.click()
    }
  }

  const addDisabled = disabled || uploading || (!pickerReady && !canUpload)

  return (
    <div className="composer-media">
      <p className="eyebrow">{t('eyebrow.media')}</p>

      <div
        className={`composer-media__grid ${dragOver ? 'composer-media__grid--drag' : ''} ${disabled || uploading ? 'composer-media__grid--disabled' : ''}`}
        onDragOver={(e) => {
          if (!canUpload || uploading) {
            return
          }
          e.preventDefault()
          setDragOver(true)
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={onDrop}
      >
        {mediaIds.map((id) => {
          const meta = libraryById[id]
          const mime = meta?.mime_type ?? 'application/octet-stream'
          const isImage = mime.startsWith('image/')
          const previewUrl = pickerReady && teamId && api ? api.mediaPreviewUrl(teamId, id) : ''

          return (
            <div key={id} className="composer-media__cell">
              <div className="composer-media__cell-thumb">
                {isImage && previewUrl && authHeader ? (
                  <AuthMediaThumb url={previewUrl} authHeader={authHeader} alt={meta?.filename ?? id} className="composer-media__cell-img" />
                ) : mime.startsWith('video/') ? (
                  <span className="composer-media__cell-icon">
                    <Icon name="film" className="inline-icon" aria-hidden />
                  </span>
                ) : previewUrl && authHeader ? (
                  <AuthMediaThumb url={previewUrl} authHeader={authHeader} alt="" className="composer-media__cell-img" />
                ) : (
                  <span className="composer-media__cell-icon">
                    <Icon name="image" className="inline-icon" aria-hidden />
                  </span>
                )}
              </div>
              <button
                type="button"
                className="composer-media__cell-remove"
                aria-label={t('common.removeAttachment', { name: meta?.filename ?? 'attachment' })}
                onClick={() => onRemove(id)}
                disabled={disabled}
              >
                <Icon name="close" className="inline-icon" />
              </button>
              <span className="composer-media__cell-name" title={meta?.filename ?? id}>
                {meta?.filename ?? (id.length > 18 ? `${id.slice(0, 10)}…` : id)}
              </span>
            </div>
          )
        })}

        <button type="button" className="composer-media__add" onClick={onAddTile} disabled={addDisabled}>
          <Icon name="plus" className="inline-icon" aria-hidden />
          <span>{uploading ? t('common.uploading') : pickerReady ? t('media.browseLibrary') : t('media.uploadNewFile')}</span>
        </button>
      </div>

      <p className="hint composer-media__hint">
        {canUpload ? t('media.dropHint') : (uploadLabel ?? t('media.selectWorkspaceMedia'))}
      </p>

      <input
        ref={inputRef}
        type="file"
        className="composer-media__file-input"
        accept="image/*,video/*"
        disabled={disabled || uploading}
        onChange={onInputChange}
      />

      {error ? <p className="status-banner__error m-0">{error}</p> : null}
      {!pickerReady ? <p className="hint m-0">{t('media.connectForLibrary')}</p> : null}

      {pickerReady && api && teamId && authHeader ? (
        <MediaLibraryPickerModal
          open={pickerOpen}
          teamId={teamId}
          api={api}
          authHeader={authHeader}
          alreadyAttached={mediaIds}
          onClose={() => setPickerOpen(false)}
          onAddIds={(ids) => onAddIds(ids)}
          onUploadNew={async (file) => {
            if (!onUpload) {
              return ''
            }
            return onUpload(file)
          }}
        />
      ) : null}
    </div>
  )
}
