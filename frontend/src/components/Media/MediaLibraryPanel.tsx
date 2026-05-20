import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '../../icons'
import type { BackendMediaItem, createApiClient } from '../../api'

type Api = ReturnType<typeof createApiClient>

/**
 * SecureImage fetches a preview with Authorization and displays it via an object URL.
 */
function SecureImage({
  url,
  authHeader,
  alt,
  className,
}: {
  url: string
  authHeader: string
  alt?: string
  className?: string
}) {
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [error, setError] = useState(false)
  const objectUrlRef = useRef<string | null>(null)

  useEffect(() => {
    let cancelled = false
    objectUrlRef.current = null
    setBlobUrl(null)
    setError(false)

    void (async () => {
      try {
        const headers: HeadersInit = {}
        if (authHeader) {
          headers.Authorization = authHeader
        }
        const response = await fetch(url, { headers })
        if (!response.ok) {
          throw new Error('preview failed')
        }
        const blob = await response.blob()
        const u = URL.createObjectURL(blob)
        if (cancelled) {
          URL.revokeObjectURL(u)
          return
        }
        objectUrlRef.current = u
        setBlobUrl(u)
      } catch {
        if (!cancelled) {
          setError(true)
        }
      }
    })()

    return () => {
      cancelled = true
      if (objectUrlRef.current) {
        URL.revokeObjectURL(objectUrlRef.current)
        objectUrlRef.current = null
      }
    }
  }, [url, authHeader])

  if (error) {
    return (
      <div
        className={className}
        style={{
          display: 'grid',
          placeItems: 'center',
          background: 'var(--surface-soft)',
        }}
      >
        <Icon name="image" />
      </div>
    )
  }

  if (!blobUrl) {
    return (
      <div className={`${className ?? ''} media-library__preview-skeleton`.trim()} aria-hidden="true" />
    )
  }

  return <img src={blobUrl} alt={alt ?? ''} className={className} loading="lazy" />
}

export function MediaLibraryPanel({
  teamId,
  teamLabel,
  entries,
  onUpload,
  onDelete,
  uploading,
  uploadHint,
  api,
}: {
  teamId: string
  teamLabel: string
  entries: BackendMediaItem[]
  onUpload: (file: File) => Promise<void>
  onDelete?: (mediaId: string) => Promise<void>
  uploading: boolean
  uploadHint?: string
  api: Api
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragOver, setDragOver] = useState(false)
  const authHeader = api.authorizationHeader()

  const runFile = useCallback(
    (file: File) => {
      void onUpload(file)
    },
    [onUpload],
  )

  const onDrop = (event: React.DragEvent) => {
    event.preventDefault()
    setDragOver(false)
    const f = event.dataTransfer.files[0]
    if (f) {
      runFile(f)
    }
  }

  return (
    <div className="media-library media-library--page">
      <div className="media-library__header">
        <p className="eyebrow">{t('media.libraryTitle')}</p>
        <p className="hint">{t('media.libraryHint', { team: teamLabel })}</p>
      </div>

      <div
        className={`media-library__dropzone ${dragOver ? 'media-library__dropzone--active' : ''} ${uploading ? 'media-library__dropzone--disabled' : ''}`}
        onDragOver={(e) => {
          e.preventDefault()
          if (!uploading) {
            setDragOver(true)
          }
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={onDrop}
      >
        <input
          ref={inputRef}
          type="file"
          className="media-library__file-input"
          accept="image/*,video/*"
          disabled={uploading}
          onChange={(event) => {
            const f = event.target.files?.[0]
            event.target.value = ''
            if (f) {
              runFile(f)
            }
          }}
        />
        <Icon name="image" className="media-library__drop-icon" />
        <p className="media-library__drop-title">{uploading ? t('common.uploading') : t('media.dropFiles')}</p>
        <button
          type="button"
          className="button button--secondary"
          disabled={uploading}
          onClick={() => inputRef.current?.click()}
        >
          {t('common.chooseFile')}
        </button>
        {uploadHint ? <p className="hint media-library__drop-hint">{uploadHint}</p> : null}
      </div>

      <div className="media-library__body">
        <div className="media-library__grid" aria-label={t('media.libraryItemsAria')}>
          {entries.length === 0 ? (
            <p className="hint media-library__empty">{t('media.emptyLibrary')}</p>
          ) : (
            entries.map((entry) => (
              <div key={entry.id} className="media-library__tile">
                <div className="media-library__preview">
                  {entry.mime_type.startsWith('image/') ? (
                    <SecureImage
                      url={api.mediaPreviewUrl(teamId, entry.id)}
                      authHeader={authHeader}
                      alt={entry.filename}
                      className="media-library__img"
                    />
                  ) : (
                    <div className="media-library__file-placeholder">
                      <Icon name="film" />
                    </div>
                  )}
                  {onDelete && (
                    <button
                      type="button"
                      className="media-library__delete-btn"
                      onClick={() => void onDelete(entry.id)}
                      title={t('common.deleteFromLibrary')}
                    >
                      <Icon name="trash" />
                    </button>
                  )}
                </div>
                <div className="media-library__tile-info">
                  <span className="media-library__tile-label" title={entry.filename}>
                    {entry.filename}
                  </span>
                  <span className="hint">
                    {entry.mime_type.split('/')[1]?.toUpperCase()} · {(entry.size_bytes / 1024).toFixed(1)} KB
                  </span>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
