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
