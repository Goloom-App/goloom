import { addMinutes, areIntervalsOverlapping, compareAsc, parseISO } from 'date-fns'

import type { AccountRecord, PostRecord } from './types'

export const SLOT_MINUTES = 30

export function postsForTeam(posts: PostRecord[], teamId: string) {
  return posts
    .filter((post) => post.teamId === teamId)
    .sort((left, right) =>
      compareAsc(parseISO(left.scheduledAt), parseISO(right.scheduledAt)),
    )
}

export function resolveScheduleChange(
  posts: PostRecord[],
  updatedPost: PostRecord,
): PostRecord[] {
  const nextPosts = posts.map((post) =>
    post.id === updatedPost.id ? { ...updatedPost } : { ...post },
  )
  const indexById = new Map(nextPosts.map((post, index) => [post.id, index]))
  const queue = [updatedPost.id]

  while (queue.length > 0) {
    const currentId = queue.shift()
    if (!currentId) {
      continue
    }

    const currentIndex = indexById.get(currentId)
    if (currentIndex === undefined) {
      continue
    }

    const current = nextPosts[currentIndex]
    const currentStart = parseISO(current.scheduledAt)
    const currentEnd = addMinutes(currentStart, current.durationMinutes)

    for (let index = 0; index < nextPosts.length; index += 1) {
      const candidate = nextPosts[index]
      if (candidate.id === current.id) {
        continue
      }

      if (!sharesAccount(current, candidate)) {
        continue
      }

      const candidateStart = parseISO(candidate.scheduledAt)
      const candidateEnd = addMinutes(candidateStart, candidate.durationMinutes)
      const overlaps = areIntervalsOverlapping(
        { start: currentStart, end: currentEnd },
        { start: candidateStart, end: candidateEnd },
      )

      if (!overlaps) {
        continue
      }

      nextPosts[index] = {
        ...candidate,
        scheduledAt: addMinutes(currentStart, SLOT_MINUTES).toISOString(),
      }
      queue.push(candidate.id)
    }
  }

  return nextPosts.sort((left, right) =>
    compareAsc(parseISO(left.scheduledAt), parseISO(right.scheduledAt)),
  )
}

export function sharedAccountLabels(
  post: PostRecord,
  accounts: AccountRecord[],
): AccountRecord[] {
  return accounts.filter((account) => post.targetAccountIds.includes(account.id))
}

function sharesAccount(left: PostRecord, right: PostRecord) {
  return left.targetAccountIds.some((accountId) =>
    right.targetAccountIds.includes(accountId),
  )
}
