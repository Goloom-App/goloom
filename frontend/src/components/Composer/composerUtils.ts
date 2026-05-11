import type { EditorDraftState } from './types'

export function effectiveBody(draft: EditorDraftState, accountId: string | null) {
  if (!accountId || accountId === 'default') {
    return draft.content
  }
  return draft.accountContentOverride[accountId] ?? draft.content
}

export interface ComposerPreviewsProps {
  draft: EditorDraftState
  teamAccounts: import('../../types').AccountRecord[]
  teamId?: string | null
  api?: { mediaPreviewUrl: (teamId: string, mediaId: string) => string } | null
  authHeader?: string
  theme: 'dark' | 'light'
  libraryItems: import('../../api').BackendMediaItem[]
}
