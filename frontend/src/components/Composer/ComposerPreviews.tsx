import { useMemo } from 'react'
import { SocialPreview } from '../post/SocialPreview'
import type { SocialPreviewAttachment } from '../post/SocialPreview.types'
import type { BackendMediaItem } from '../../api'
import type { EditorDraftState } from './types'
import { effectiveBody, type ComposerPreviewsProps } from './composerUtils'

function attachmentsForDestination(
  draft: EditorDraftState,
  accountId: string,
  teamId: string | undefined | null,
  api: { mediaPreviewUrl: (teamId: string, mediaId: string) => string } | null | undefined,
  meta: Record<string, Pick<BackendMediaItem, 'filename' | 'mime_type'>>,
): SocialPreviewAttachment[] {
  const ex = new Set(draft.mediaExcludeByAccount[accountId] ?? [])
  if (!teamId || !api) {
    return []
  }
  return draft.mediaIds
    .filter((id) => !ex.has(id))
    .map((id) => ({
      id,
      previewUrl: api.mediaPreviewUrl(teamId, id),
      mimeType: meta[id]?.mime_type ?? 'image/jpeg',
      filename: meta[id]?.filename,
    }))
}

export function ComposerPreviews({ draft, teamAccounts, teamId, api, authHeader, theme, libraryItems }: ComposerPreviewsProps) {
  const selectedAccounts = useMemo(
    () => teamAccounts.filter((account) => draft.targetAccountIds.includes(account.id)),
    [draft.targetAccountIds, teamAccounts],
  )

  const libraryById = useMemo(() => {
    const o: Record<string, Pick<BackendMediaItem, 'filename' | 'mime_type'>> = {}
    for (const row of libraryItems) {
      o[row.id] = { filename: row.filename, mime_type: row.mime_type }
    }
    for (const id of draft.mediaIds) {
      if (!o[id]) {
        o[id] = {
          filename: id.length > 28 ? `${id.slice(0, 14)}…${id.slice(-8)}` : id,
          mime_type: 'application/octet-stream',
        }
      }
    }
    return o
  }, [libraryItems, draft.mediaIds])

  return (
    <>
      <p className="eyebrow">Live previews</p>
      <div className="composer-preview-stack">
        {selectedAccounts.length > 0 ? (
          selectedAccounts.map((account) => (
            <SocialPreview
              key={account.id}
              account={account}
              content={effectiveBody(draft, account.id)}
              scheduledAt={draft.scheduledAt}
              theme={theme}
              attachments={attachmentsForDestination(draft, account.id, teamId, api, libraryById)}
              authHeader={authHeader}
            />
          ))
        ) : (
          <p className="hint">Select a destination to see previews.</p>
        )}
      </div>
    </>
  )
}
