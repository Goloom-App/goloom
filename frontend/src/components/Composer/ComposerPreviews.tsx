import { useMemo } from 'react'
import { SocialPreview } from '../post/SocialPreview'
import type { SocialPreviewAttachment } from '../post/SocialPreview.types'
import type { AccountRecord } from '../../types'
import type { BackendMediaItem, createApiClient } from '../../api'
import type { EditorDraftState } from './types'

type Api = ReturnType<typeof createApiClient>

function attachmentsForDestination(
  draft: EditorDraftState,
  accountId: string,
  teamId: string | undefined | null,
  api: Api | null | undefined,
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

export function effectiveBody(draft: EditorDraftState, accountId: string | null) {
  if (!accountId || accountId === 'default') {
    return draft.content
  }
  return draft.accountContentOverride[accountId] ?? draft.content
}

interface ComposerPreviewsProps {
  draft: EditorDraftState
  teamAccounts: AccountRecord[]
  teamId?: string | null
  api?: Api | null
  authHeader?: string
  theme: 'dark' | 'light'
  libraryItems: BackendMediaItem[]
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
