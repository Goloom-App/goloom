import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { useTranslation } from 'react-i18next'
import { Icon } from '../../icons'
import type { BackendMediaItem, createApiClient } from '../../api'

type Api = ReturnType<typeof createApiClient>

/**
 * useAuthedBlobUrl fetches a protected resource with the Authorization header and
 * exposes it as an object URL, revoking it on cleanup.
 */
function useAuthedBlobUrl(url: string, authHeader: string): { blobUrl: string | null; error: boolean } {
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

  return { blobUrl, error }
}

function SecureImage({ url, authHeader, alt, className }: { url: string; authHeader: string; alt?: string; className?: string }) {
  const { blobUrl, error } = useAuthedBlobUrl(url, authHeader)

  if (error) {
    return (
      <div className={className} style={{ display: 'grid', placeItems: 'center', background: 'var(--surface-soft)' }}>
        <Icon name="image" />
      </div>
    )
  }
  if (!blobUrl) {
    return <div className={`${className ?? ''} media-library__preview-skeleton`.trim()} aria-hidden="true" />
  }
  return <img src={blobUrl} alt={alt ?? ''} className={className} loading="lazy" />
}

function SecureVideo({ url, authHeader, className }: { url: string; authHeader: string; className?: string }) {
  const { blobUrl, error } = useAuthedBlobUrl(url, authHeader)
  if (error || !blobUrl) {
    return (
      <div className={className} style={{ display: 'grid', placeItems: 'center', background: 'var(--surface-soft)' }}>
        <Icon name="film" />
      </div>
    )
  }
  return <video src={blobUrl} className={className} controls />
}

function MediaLightbox({
  entries,
  index,
  teamId,
  api,
  authHeader,
  onClose,
  onNavigate,
  onRename,
  onDelete,
}: {
  entries: BackendMediaItem[]
  index: number
  teamId: string
  api: Api
  authHeader: string
  onClose: () => void
  onNavigate: (next: number) => void
  onRename?: (mediaId: string, filename: string) => Promise<void>
  onDelete?: (mediaId: string) => Promise<void>
}) {
  const { t } = useTranslation()
  const entry = entries[index]
  const [name, setName] = useState(entry?.filename ?? '')
  const [saving, setSaving] = useState(false)

  // Reset the rename field whenever the viewed item changes.
  useEffect(() => {
    setName(entry?.filename ?? '')
  }, [entry?.id, entry?.filename])

  if (!entry) {
    return null
  }

  const isImage = entry.mime_type.startsWith('image/')
  const dirty = name.trim() !== '' && name.trim() !== entry.filename

  async function saveRename() {
    if (!onRename || !dirty) {
      return
    }
    setSaving(true)
    try {
      await onRename(entry.id, name.trim())
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog.Root open onOpenChange={(open) => { if (!open) onClose() }}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content media-lightbox" aria-describedby={undefined}>
          <Dialog.Title className="media-lightbox__title">{entry.filename}</Dialog.Title>
          <div className="media-lightbox__stage">
            {entries.length > 1 ? (
              <button
                type="button"
                className="media-lightbox__nav media-lightbox__nav--prev"
                aria-label={t('media.previous')}
                onClick={() => onNavigate((index - 1 + entries.length) % entries.length)}
              >
                <Icon name="chevron-left" />
              </button>
            ) : null}
            {isImage ? (
              <SecureImage url={api.mediaPreviewUrl(teamId, entry.id)} authHeader={authHeader} alt={entry.filename} className="media-lightbox__media" />
            ) : (
              <SecureVideo url={api.mediaPreviewUrl(teamId, entry.id)} authHeader={authHeader} className="media-lightbox__media" />
            )}
            {entries.length > 1 ? (
              <button
                type="button"
                className="media-lightbox__nav media-lightbox__nav--next"
                aria-label={t('media.next')}
                onClick={() => onNavigate((index + 1) % entries.length)}
              >
                <Icon name="chevron-right" />
              </button>
            ) : null}
          </div>

          <div className="media-lightbox__footer">
            {onRename ? (
              <label className="field media-lightbox__rename">
                <span>{t('media.renameTitle')}</span>
                <div className="inline-cluster">
                  <input
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    onKeyDown={(e) => { if (e.key === 'Enter') void saveRename() }}
                  />
                  <button type="button" className="button button--primary" onClick={() => void saveRename()} disabled={!dirty || saving}>
                    {t('media.rename')}
                  </button>
                </div>
              </label>
            ) : null}
            <div className="inline-cluster">
              <span className="hint">
                {entry.mime_type.split('/')[1]?.toUpperCase()} · {(entry.size_bytes / 1024).toFixed(1)} KB
                {entry.width && entry.height ? ` · ${entry.width}×${entry.height}` : ''}
              </span>
              {onDelete ? (
                <button type="button" className="button button--secondary" onClick={() => void onDelete(entry.id).then(onClose)}>
                  {t('common.delete')}
                </button>
              ) : null}
              <Dialog.Close asChild>
                <button type="button" className="button button--secondary">{t('common.close')}</button>
              </Dialog.Close>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

export function MediaLibraryPanel({
  teamId,
  teamLabel,
  entries,
  onUpload,
  onDelete,
  onRename,
  uploading,
  uploadHint,
  api,
}: {
  teamId: string
  teamLabel: string
  entries: BackendMediaItem[]
  onUpload: (file: File) => Promise<void>
  onDelete?: (mediaId: string) => Promise<void>
  onRename?: (mediaId: string, filename: string) => Promise<void>
  uploading: boolean
  uploadHint?: string
  api: Api
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragOver, setDragOver] = useState(false)
  const [query, setQuery] = useState('')
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null)
  const authHeader = api.authorizationHeader()

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) {
      return entries
    }
    return entries.filter((entry) => entry.filename.toLowerCase().includes(q))
  }, [entries, query])

  const runFile = useCallback((file: File) => { void onUpload(file) }, [onUpload])

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
        onDragOver={(e) => { e.preventDefault(); if (!uploading) { setDragOver(true) } }}
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
        <button type="button" className="button button--secondary" disabled={uploading} onClick={() => inputRef.current?.click()}>
          {t('common.chooseFile')}
        </button>
        {uploadHint ? <p className="hint media-library__drop-hint">{uploadHint}</p> : null}
      </div>

      {entries.length > 0 ? (
        <label className="field media-library__search">
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('media.searchPlaceholder')}
            aria-label={t('media.searchPlaceholder')}
          />
        </label>
      ) : null}

      <div className="media-library__body">
        <div className="media-library__grid" aria-label={t('media.libraryItemsAria')}>
          {entries.length === 0 ? (
            <p className="hint media-library__empty">{t('media.emptyLibrary')}</p>
          ) : filtered.length === 0 ? (
            <p className="hint media-library__empty">{t('media.noSearchResults', { query: query.trim() })}</p>
          ) : (
            filtered.map((entry, idx) => (
              <div key={entry.id} className="media-library__tile">
                <div className="media-library__preview">
                  <button
                    type="button"
                    className="media-library__open-btn"
                    onClick={() => setLightboxIndex(idx)}
                    title={t('media.openPreview')}
                    aria-label={t('media.openPreview')}
                  >
                    {entry.mime_type.startsWith('image/') ? (
                      <SecureImage url={api.mediaPreviewUrl(teamId, entry.id)} authHeader={authHeader} alt={entry.filename} className="media-library__img" />
                    ) : (
                      <div className="media-library__file-placeholder">
                        <Icon name="film" />
                      </div>
                    )}
                  </button>
                  {onDelete && (
                    <button type="button" className="media-library__delete-btn" onClick={() => void onDelete(entry.id)} title={t('common.deleteFromLibrary')}>
                      <Icon name="trash" />
                    </button>
                  )}
                </div>
                <div className="media-library__tile-info">
                  <span className="media-library__tile-label" title={entry.filename}>{entry.filename}</span>
                  <span className="hint">
                    {entry.mime_type.split('/')[1]?.toUpperCase()} · {(entry.size_bytes / 1024).toFixed(1)} KB
                  </span>
                </div>
              </div>
            ))
          )}
        </div>
      </div>

      {lightboxIndex !== null && filtered[lightboxIndex] ? (
        <MediaLightbox
          entries={filtered}
          index={lightboxIndex}
          teamId={teamId}
          api={api}
          authHeader={authHeader}
          onClose={() => setLightboxIndex(null)}
          onNavigate={setLightboxIndex}
          onRename={onRename}
          onDelete={onDelete}
        />
      ) : null}
    </div>
  )
}
