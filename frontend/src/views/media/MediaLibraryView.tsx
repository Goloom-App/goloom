import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { BackendMediaItem, createApiClient } from '../../api'
import { translateApiError } from '../../i18n/translateApiError'
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
  const { t } = useTranslation()
  const [entries, setEntries] = useState<BackendMediaItem[]>([])
  const [loading, setLoading] = useState(false)
  const [uploading, setUploading] = useState(false)

  const loadMedia = useCallback(async () => {
    setLoading(true)
    try {
      const { items } = await api.listTeamMedia(teamId)
      setEntries(items)
    } catch (e) {
      const raw = e instanceof Error ? e.message : t('status.failedLoadMediaLibrary')
      onError(translateApiError(raw, t))
    } finally {
      setLoading(false)
    }
  }, [api, teamId, onError, t])

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
        const raw = e instanceof Error ? e.message : t('common.uploadFailed')
        onError(translateApiError(raw, t))
      } finally {
        setUploading(false)
      }
    },
    [api, loadMedia, teamId, onError, t],
  )

  const onDelete = useCallback(
    async (mediaId: string) => {
      if (!confirm(t('common.confirmDeleteMedia'))) return
      try {
        await api.deleteTeamMedia(teamId, mediaId)
        await loadMedia()
      } catch (e) {
        const raw = e instanceof Error ? e.message : t('status.deleteFailed')
        onError(translateApiError(raw, t))
      }
    },
    [api, loadMedia, teamId, onError, t],
  )

  const onRename = useCallback(
    async (mediaId: string, filename: string) => {
      try {
        await api.renameTeamMedia(teamId, mediaId, filename)
        await loadMedia()
      } catch (e) {
        const raw = e instanceof Error ? e.message : t('common.actionFailed')
        onError(translateApiError(raw, t))
      }
    },
    [api, loadMedia, teamId, onError, t],
  )

  return (
    <div className="media-library-view glass-panel">
      {loading && entries.length === 0 ? <p className="hint">{t('common.loadingMedia')}</p> : null}
      <MediaLibraryPanel
        teamId={teamId}
        teamLabel={teamName}
        entries={entries}
        onUpload={onUpload}
        onDelete={onDelete}
        onRename={onRename}
        uploading={uploading}
        api={api}
      />
    </div>
  )
}
