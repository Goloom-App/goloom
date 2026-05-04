import { useCallback, useRef, useState } from 'react'
import { Icon } from '../../icons'

export type LocalMediaEntry = {
  id: string
  filename: string
  accountId: string
  createdAt: string
}

/**
 * Team media workspace: drag-and-drop upload and grid of uploaded asset IDs (session + localStorage).
 */
export function MediaLibraryPanel({
  teamLabel,
  entries,
  onUpload,
  uploading,
  uploadHint,
}: {
  teamLabel: string
  entries: LocalMediaEntry[]
  onUpload: (file: File) => Promise<void>
  uploading: boolean
  uploadHint?: string
}) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragOver, setDragOver] = useState(false)

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
        <p className="eyebrow">Media library</p>
        <p className="hint">Uploads for {teamLabel}. Files are sent to the provider you pick below; IDs are stored for attaching to posts.</p>
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
        <p className="media-library__drop-title">{uploading ? 'Uploading…' : 'Drop files here'}</p>
        <button
          type="button"
          className="button button--secondary"
          disabled={uploading}
          onClick={() => inputRef.current?.click()}
        >
          Choose file
        </button>
        {uploadHint ? <p className="hint media-library__drop-hint">{uploadHint}</p> : null}
      </div>

      <div className="media-library__body media-library__body--simple">
        <div className="media-library__grid" aria-label="Uploaded media">
          {entries.length === 0 ? (
            <p className="hint media-library__empty">No uploads yet. Add an image or video above.</p>
          ) : (
            entries.map((entry) => (
              <div key={`${entry.id}-${entry.createdAt}`} className="media-library__tile media-library__tile--static">
                <span className="media-library__tile-label">{entry.filename}</span>
                <code className="mono media-library__tile-id" title={entry.id}>
                  {entry.id.length > 36 ? `${entry.id.slice(0, 18)}…` : entry.id}
                </code>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
