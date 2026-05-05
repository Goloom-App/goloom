import { useCallback, useRef, useState } from 'react'
import type { BackendMediaItem, createApiClient } from '../../api'
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
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragOver, setDragOver] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pickerOpen, setPickerOpen] = useState(false)

  const pickerReady = Boolean(teamId && api && authHeader?.trim())

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
        setError(e instanceof Error ? e.message : 'Upload failed')
      } finally {
        setUploading(false)
      }
    },
    [disabled, onAddIds, onUpload],
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
    const f = event.dataTransfer.files[0]
    if (f) {
      void runUpload(f)
    }
  }

  const openPicker = () => {
    if (pickerReady) {
      setPickerOpen(true)
    }
  }

  return (
    <div className="composer-media">
      <p className="eyebrow">Media</p>
      <div className="composer-media__actions">
        <button type="button" className="button button--secondary" disabled={!pickerReady || disabled || uploading} onClick={openPicker}>
          <Icon name="image" className="inline-icon" />
          <span>Browse library…</span>
        </button>
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
      {onUpload ? (
        <div
          className={`composer-media__drop ${dragOver ? 'composer-media__drop--active' : ''} ${uploading || disabled ? 'composer-media__drop--disabled' : ''}`}
          onDragOver={(e) => {
            e.preventDefault()
            if (!disabled && !uploading) {
              setDragOver(true)
            }
          }}
          onDragLeave={() => setDragOver(false)}
          onDrop={onDrop}
        >
          <input ref={inputRef} type="file" className="composer-media__file-input" accept="image/*,video/*" disabled={disabled || uploading} onChange={onInputChange} />
          <button type="button" className="composer-media__pick" disabled={disabled || uploading} onClick={() => inputRef.current?.click()}>
            <Icon name="plus" className="inline-icon" />
            <span>{uploading ? 'Uploading…' : 'Upload new file'}</span>
          </button>
          <p className="hint composer-media__drop-hint">or drop a file here · added to workspace library automatically</p>
        </div>
      ) : (
        <p className="hint">{uploadLabel ?? 'Select a workspace to attach media.'}</p>
      )}
      {error ? <p className="status-banner__error" style={{ margin: 0 }}>{error}</p> : null}
      {!pickerReady ? <p className="hint" style={{ margin: 0 }}>Connect with a bearer token and pick a workspace to browse the library.</p> : null}
      {mediaIds.length > 0 ? (
        <ul className="composer-media__gallery" aria-label="Attached media">
          {mediaIds.map((id) => {
            const meta = libraryById[id]
            const mime = meta?.mime_type ?? 'application/octet-stream'
            const isImage = mime.startsWith('image/')
            const previewUrl = pickerReady && teamId && api ? api.mediaPreviewUrl(teamId, id) : ''

            return (
              <li key={id} className="composer-media__tile">
                <div className="composer-media__tile-thumb">
                  {isImage && previewUrl && authHeader ? (
                    <AuthMediaThumb url={previewUrl} authHeader={authHeader} alt={meta?.filename ?? id} className="composer-media__tile-img" />
                  ) : mime.startsWith('video/') ? (
                    <span className="composer-media__tile-video">
                      <Icon name="film" className="inline-icon" aria-hidden />
                    </span>
                  ) : previewUrl && authHeader ? (
                    <AuthMediaThumb url={previewUrl} authHeader={authHeader} alt="" className="composer-media__tile-img" />
                  ) : (
                    <span className="composer-media__tile-fallback">
                      <Icon name="image" className="inline-icon" aria-hidden />
                    </span>
                  )}
                </div>
                <div className="composer-media__tile-meta">
                  <span className="composer-media__tile-name" title={meta?.filename ?? id}>
                    {meta?.filename ?? (id.length > 20 ? `${id.slice(0, 12)}…` : id)}
                  </span>
                  <button
                    type="button"
                    className="composer-media__tile-remove"
                    aria-label={`Remove ${meta?.filename ?? 'attachment'}`}
                    onClick={() => onRemove(id)}
                  >
                    <Icon name="close" className="inline-icon" />
                  </button>
                </div>
              </li>
            )
          })}
        </ul>
      ) : null}
    </div>
  )
}
