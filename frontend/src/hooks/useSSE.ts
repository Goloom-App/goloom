import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createApiClient } from '../api'
import type { BackendAIJob } from '../api'
import { mapAIJob } from '../mappers'
import type { AIJob } from '../types'
import { useAIJob } from './useAI'

const SETTINGS_STORAGE_KEY = 'goloom-ui-settings'

function getSseSettings(): { baseUrl: string; token: string } {
  if (typeof window === 'undefined') return { baseUrl: '', token: '' }
  try {
    const raw = window.localStorage.getItem(SETTINGS_STORAGE_KEY)
    if (!raw) return { baseUrl: '', token: '' }
    const parsed = JSON.parse(raw) as { general?: { apiBaseUrl?: string; bearerToken?: string } }
    return {
      baseUrl: parsed.general?.apiBaseUrl?.trim() ?? '',
      token: parsed.general?.bearerToken?.trim() ?? '',
    }
  } catch {
    return { baseUrl: '', token: '' }
  }
}

function getSseApiClient() {
  const { baseUrl, token } = getSseSettings()
  return createApiClient({
    baseUrl: baseUrl || (typeof window !== 'undefined' ? window.location.origin : ''),
    token,
  })
}

export interface SSEEvent {
  type: string
  data: string
  lastEventId: string
}

export function useAIJobStream(teamId: string): { isConnected: boolean; lastEvent: SSEEvent | null } {
  const queryClient = useQueryClient()
  const [isConnected, setIsConnected] = useState(false)
  const [lastEvent, setLastEvent] = useState<SSEEvent | null>(null)

  useEffect(() => {
    if (!teamId) return

    const { baseUrl, token } = getSseSettings()
    const base = (baseUrl || window.location.origin).replace(/\/$/, '')
    // EventSource cannot set Authorization header — auth via ?token= query param
    const url = token
      ? `${base}/v1/teams/${teamId}/ai-jobs/stream?token=${encodeURIComponent(token)}`
      : `${base}/v1/teams/${teamId}/ai-jobs/stream`

    const es = new EventSource(url)

    es.onopen = () => {
      setIsConnected(true)
    }

    es.onerror = () => {
      setIsConnected(false)
    }

    const handleJobEvent = (e: Event): void => {
      const msg = e as MessageEvent<string>
      try {
        const raw = JSON.parse(msg.data) as BackendAIJob
        const job = mapAIJob(raw)

        setLastEvent({ type: msg.type, data: msg.data, lastEventId: msg.lastEventId })

        queryClient.setQueryData<AIJob>(['ai-job', teamId, job.id], job)

        queryClient.setQueryData<AIJob[]>(['ai-jobs', teamId], (old) => {
          if (!old) return [job]
          const idx = old.findIndex((j) => j.id === job.id)
          if (idx === -1) return [job, ...old]
          const updated = [...old]
          updated[idx] = job
          return updated
        })

      } catch {
        // noop
      }
    }

    const handleHeartbeat = (): void => {
      setIsConnected(true)
    }

    es.addEventListener('job:status', handleJobEvent)
    es.addEventListener('job:result', handleJobEvent)
    es.addEventListener('heartbeat', handleHeartbeat)

    return () => {
      es.close()
      setIsConnected(false)
    }
  }, [teamId, queryClient])

  return { isConnected, lastEvent }
}

export function useAIJobStatus(
  teamId: string,
  jobId: string,
): { job: AIJob | undefined; isLoading: boolean; isConnected: boolean } {
  const { isConnected } = useAIJobStream(teamId)
  const { data, isLoading } = useAIJob(teamId, jobId)

  useQuery({
    queryKey: ['ai-job', teamId, jobId],
    queryFn: async () => mapAIJob(await getSseApiClient().getAIJob(teamId, jobId)),
    enabled: Boolean(teamId && jobId) && !isConnected,
    refetchInterval: 5000,
  })

  return {
    job: data,
    isLoading,
    isConnected,
  }
}
