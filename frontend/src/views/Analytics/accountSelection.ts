import type { BackendAccountGrowthPoint, BackendEngagementHeatmapBucket } from '../../api'

// An account selection is the list of explicitly selected account ids. The empty
// list is the canonical "all accounts" state — the default the analytics view
// opens in. Keeping "all" as the empty list means we never have to enumerate
// every account just to represent the default.
export type AccountSelection = string[]

export function isAllSelected(selection: AccountSelection): boolean {
  return selection.length === 0
}

// isAccountActive reports whether an account is currently part of the view. In
// the "all" state every account is active; otherwise only the selected ones are.
export function isAccountActive(selection: AccountSelection, id: string): boolean {
  return selection.length === 0 || selection.includes(id)
}

// toggleAccount implements the interaction from issue #80:
//   - from "all" (default), the first click filters down to just that account;
//   - afterwards each click adds or removes an account;
//   - selecting every account, or removing the last one, collapses back to "all".
export function toggleAccount(selection: AccountSelection, id: string, allIds: string[]): AccountSelection {
  if (selection.length === 0) {
    return [id]
  }
  const next = selection.includes(id) ? selection.filter((x) => x !== id) : [...selection, id]
  if (next.length === 0 || next.length >= allIds.length) {
    return []
  }
  return next
}

// activeAccountIds returns the ids that are currently in view, in the order the
// accounts are given. Used to drive the per-account data fetches.
export function activeAccountIds(selection: AccountSelection, allIds: string[]): string[] {
  if (selection.length === 0) {
    return [...allIds]
  }
  return allIds.filter((id) => selection.includes(id))
}

// aggregateGrowthSeries merges several per-account growth series into one team
// series by summing followers/following/posts per date, sorted ascending.
export function aggregateGrowthSeries(seriesByAccount: BackendAccountGrowthPoint[][]): BackendAccountGrowthPoint[] {
  const byDate = new Map<string, BackendAccountGrowthPoint>()
  for (const series of seriesByAccount) {
    for (const point of series) {
      const existing = byDate.get(point.date)
      if (existing) {
        existing.followers += point.followers
        existing.following += point.following
        existing.posts += point.posts
      } else {
        byDate.set(point.date, { ...point })
      }
    }
  }
  return Array.from(byDate.values()).sort((a, b) => a.date.localeCompare(b.date))
}

// aggregateHeatmapBuckets merges several per-account heatmaps into one by summing
// the score of buckets that share a weekday/hour.
export function aggregateHeatmapBuckets(
  bucketsByAccount: BackendEngagementHeatmapBucket[][],
): BackendEngagementHeatmapBucket[] {
  const byKey = new Map<string, BackendEngagementHeatmapBucket>()
  for (const buckets of bucketsByAccount) {
    for (const bucket of buckets) {
      const key = `${bucket.weekday}-${bucket.hour}`
      const existing = byKey.get(key)
      if (existing) {
        existing.score += bucket.score
      } else {
        byKey.set(key, { ...bucket })
      }
    }
  }
  return Array.from(byKey.values())
}
