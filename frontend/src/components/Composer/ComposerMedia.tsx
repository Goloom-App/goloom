import { useCallback, useRef, useState } from 'react'
import { Icon } from '../../icons'

export function ComposerMedia({
  mediaIds,
  onAdd,
  onRemove,
  onUpload,
  uploadLabel,
  disabled,
}: {
  mediaIds: string[]
  onAdd: (id: string) => void
  onRemove: (id: string) => void
  onUpload?: (file: File) => Promise<string>
  /** Shown when upload is unavailable (e.g. no account). */
  uploadLabel?: string
  disabled?: boolean
}) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragOver, setDragOver] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [error, setError] = useState<string | null>(null)

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
          onAdd(id)
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Upload failed')
      } finally {
        setUploading(false)
      }
    },
    [disabled, onAdd, onUpload],
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

  return (
    <div className="composer-media">
      <p className="eyebrow">Media</p>
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
          <input
            ref={inputRef}
            type="file"
            className="composer-media__file-input"
            accept="image/*,video/*"
            disabled={disabled || uploading}
            onChange={onInputChange}
          />
          <button
            type="button"
            className="composer-media__pick"
            disabled={disabled || uploading}
            onClick={() => inputRef.current?.click()}
          >
            <Icon name="image" className="inline-icon" />
            <span>{uploading ? 'Uploading…' : 'Add file'}</span>
          </button>
          <p className="hint composer-media__drop-hint">or drop a file here</p>
        </div>
      ) : (
        <p className="hint">{uploadLabel ?? 'Connect an account in this team to upload media.'}</p>
      )}
      {error ? <p className="status-banner__error" style={{ margin: 0 }}>{error}</p> : null}
      {mediaIds.length > 0 ? (
        <ul className="composer-media__chips" aria-label="Attached media">
          {mediaIds.map((id) => (
            <li key={id} className="composer-media__chip">
              <code className="mono" title={id}>
                {id.length > 28 ? `${id.slice(0, 14)}…${id.slice(-8)}` : id}
              </code>
              <button
                type="button"
                className="composer-media__chip-remove"
                aria-label="Remove attachment"
                onClick={() => onRemove(id)}
              >
                <Icon name="close" className="inline-icon" />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}
