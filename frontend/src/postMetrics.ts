import type { BackendPostMetric } from './api'
import type { PreviewEngagement } from './components/post/SocialPreview.types'

/** Build per-account engagement from normalized API metric rows (likes, reposts, replies). */
export function engagementForAccount(metrics: BackendPostMetric[], accountId: string): PreviewEngagement {
  let likes = 0
  let reposts = 0
  let replies = 0
  for (const row of metrics) {
    if (row.account_id !== accountId) {
      continue
    }
    switch (row.metric) {
      case 'likes':
        likes = row.value
        break
      case 'reposts':
        reposts = row.value
        break
      case 'replies':
        replies = row.value
        break
      default:
        break
    }
  }
  return { likes, reposts, replies }
}
