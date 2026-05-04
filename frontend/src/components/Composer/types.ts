import type { PostRecord } from '../../types'

export type EditorDraftState = {
  title: string
  content: string
  scheduledAt: string
  targetAccountIds: string[]
  status: PostRecord['status']
  /** Per-account body when it differs from `content`; omitted key means inherit `content`. */
  accountContentOverride: Record<string, string>
}
