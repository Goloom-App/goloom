import { useQuery } from '@tanstack/react-query'

import { getApiClient } from './useAI'
import { mapReviewQueueItem } from '../mappers'

function teamKey(teamId: string) {
  return ['team', teamId] as const
}

export function useReviewQueue(teamId: string) {
  return useQuery({
    queryKey: [...teamKey(teamId), 'review-queue'],
    queryFn: async () => (await getApiClient().listReviewQueue(teamId)).items.map(mapReviewQueueItem),
    enabled: Boolean(teamId),
    refetchInterval: 30_000,
  })
}

export function useReviewQueueCount(teamId: string) {
  const { data } = useReviewQueue(teamId)
  const count = data?.length ?? 0
  const overdueCount = data?.filter((item) => item.isOverdue).length ?? 0
  return { count, overdueCount, hasPending: count > 0 }
}
