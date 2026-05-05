import { useCallback, useEffect, useState } from 'react'
import type { BackendMediaItem, createApiClient } from '../../api'
import { MediaLibraryPanel } from '../../components/Media/MediaLibraryPanel'

type Api = ReturnType<typeof createApiClient>

export function MediaLibraryView({
  teamId,
  teamName,
  api,
  onError,
}: {
  teamId: string
  teamName: string
  api: Api
  onError: (message: string) => void
}) {
  const [entries, setEntries] = useState<BackendMediaItem[]>([])
  const [loading, setLoading] = useState(false)
  const [uploading, setUploading] = useState(false)

  const loadMedia = useCallback(async () => {
    setLoading(true)
    try {
      const { items } = await api.listTeamMedia(teamId)
      setEntries(items)
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Failed to load media library')
    } finally {
      setLoading(false)
    }
  }, [api, teamId, onError])

  useEffect(() => {
    void loadMedia()
  }, [loadMedia])

  const onUpload = useCallback(
    async (file: File) => {
      setUploading(true)
      try {
        await api.uploadTeamMediaToLibrary(teamId, file)
        await loadMedia()
      } catch (e) {
        onError(e instanceof Error ? e.message : 'Upload failed')
      } finally {
        setUploading(false)
      }
    },
    [api, loadMedia, teamId, onError],
  )

  const onDelete = useCallback(
    async (mediaId: string) => {
      if (!confirm('Are you sure you want to delete this media?')) return
      try {
        await api.deleteTeamMedia(teamId, mediaId)
        await loadMedia()
      } catch (e) {
        onError(e instanceof Error ? e.message : 'Delete failed')
      }
    },
    [api, loadMedia, teamId, onError],
  )

  return (
    <div className="media-library-view glass-panel">
      {loading && entries.length === 0 ? <p className="hint">Loading media…</p> : null}
      <MediaLibraryPanel
        teamId={teamId}
        teamLabel={teamName}
        entries={entries}
        onUpload={onUpload}
        onDelete={onDelete}
        uploading={uploading}
        api={api}
      />
    </div>
  )
}
