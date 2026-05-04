import { useCallback, useEffect, useMemo, useState } from 'react'
import type { createApiClient } from '../../api'
import { MediaLibraryPanel, type LocalMediaEntry } from '../../components/Media/MediaLibraryPanel'
import type { AccountRecord } from '../../types'

const storageKey = (teamId: string) => `goloom.media-library.${teamId}`

type Api = ReturnType<typeof createApiClient>

export function MediaLibraryView({
  teamId,
  teamName,
  teamAccounts,
  api,
  onError,
}: {
  teamId: string
  teamName: string
  teamAccounts: AccountRecord[]
  api: Api
  onError: (message: string) => void
}) {
  const [entries, setEntries] = useState<LocalMediaEntry[]>([])
  const [uploadAccountId, setUploadAccountId] = useState(() => teamAccounts[0]?.id ?? '')
  const [uploading, setUploading] = useState(false)

  useEffect(() => {
    if (teamAccounts[0] && !teamAccounts.find((a) => a.id === uploadAccountId)) {
      setUploadAccountId(teamAccounts[0].id)
    }
  }, [teamAccounts, uploadAccountId])

  useEffect(() => {
    try {
      const raw = localStorage.getItem(storageKey(teamId))
      if (raw) {
        const parsed = JSON.parse(raw) as LocalMediaEntry[]
        if (Array.isArray(parsed)) {
          setEntries(parsed)
        }
      } else {
        setEntries([])
      }
    } catch {
      setEntries([])
    }
  }, [teamId])

  const uploadHint = useMemo(() => {
    if (teamAccounts.length === 0) {
      return 'Add a connected account to this team before uploading.'
    }
    return `Upload uses the selected account’s provider.`
  }, [teamAccounts.length])

  const onUpload = useCallback(
    async (file: File) => {
      if (!uploadAccountId) {
        onError('Select an account for upload.')
        return
      }
      setUploading(true)
      try {
        const { media_id: mediaId } = await api.uploadTeamMedia(teamId, uploadAccountId, file)
        const entry: LocalMediaEntry = {
          id: mediaId,
          filename: file.name || 'upload',
          accountId: uploadAccountId,
          createdAt: new Date().toISOString(),
        }
        setEntries((prev) => {
          const next = [entry, ...prev]
          try {
            localStorage.setItem(storageKey(teamId), JSON.stringify(next))
          } catch {
            onError('Could not persist media list (storage full or private mode).')
          }
          return next
        })
      } catch (e) {
        onError(e instanceof Error ? e.message : 'Upload failed')
      } finally {
        setUploading(false)
      }
    },
    [api, onError, teamId, uploadAccountId],
  )

  return (
    <div className="media-library-view glass-panel">
      {teamAccounts.length > 0 ? (
        <label className="field media-library-view__account-pick">
          <span>Upload as account</span>
          <select value={uploadAccountId} onChange={(e) => setUploadAccountId(e.target.value)}>
            {teamAccounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name} · {a.provider}
              </option>
            ))}
          </select>
        </label>
      ) : null}
      <MediaLibraryPanel teamLabel={teamName} entries={entries} onUpload={onUpload} uploading={uploading} uploadHint={uploadHint} />
    </div>
  )
}
