import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createApiClient } from '../api'
import {
  mapAIJob,
  mapAIServiceConfig,
  mapCampaignFormat,
  mapRSSFeedConfig,
  mapStyleExample,
  mapTeamProfile,
} from '../mappers'
import { initialSettings } from '../data'
import type { SettingsState } from '../types'

const SETTINGS_STORAGE_KEY = 'goloom-ui-settings'
type ApiClient = ReturnType<typeof createApiClient>

function loadStoredSettings(): SettingsState {
  if (typeof window === 'undefined') {
    return initialSettings
  }
  const raw = window.localStorage.getItem(SETTINGS_STORAGE_KEY)
  if (!raw) {
    return initialSettings
  }
  try {
    const parsed = JSON.parse(raw) as Partial<SettingsState>
    return {
      ...initialSettings,
      ...parsed,
      ui: { ...initialSettings.ui, ...parsed.ui },
      general: { ...initialSettings.general, ...parsed.general },
      oidc: { ...initialSettings.oidc, ...parsed.oidc },
      security: { ...initialSettings.security, ...parsed.security },
      scheduler: { ...initialSettings.scheduler, ...parsed.scheduler },
      providers: { ...initialSettings.providers, ...parsed.providers },
    }
  } catch {
    return initialSettings
  }
}

export function getApiClient(): ApiClient {
  const settings = loadStoredSettings()
  return createApiClient({
    baseUrl: settings.general.apiBaseUrl.trim() || (typeof window !== 'undefined' ? window.location.origin : ''),
    token: settings.general.bearerToken.trim(),
  })
}

function teamKey(teamId: string) {
  return ['ai', teamId]
}

function jobKey(teamId: string, jobId: string) {
  return ['ai-job', teamId, jobId]
}

export function useTeamProfile(teamId: string) {
  return useQuery({
    queryKey: [...teamKey(teamId), 'profile'],
    queryFn: async () => mapTeamProfile(await getApiClient().getTeamProfile(teamId)),
    enabled: Boolean(teamId),
  })
}

export function useUpsertTeamProfile() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      teamId,
      data,
    }: {
      teamId: string
      data: Parameters<ApiClient['upsertTeamProfile']>[1]
    }) => mapTeamProfile(await getApiClient().upsertTeamProfile(teamId, data)),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: teamKey(variables.teamId) })
      void queryClient.invalidateQueries({ queryKey: ['ai-context', variables.teamId] })
    },
  })
}

export function useCampaignFormats(teamId: string) {
  return useQuery({
    queryKey: [...teamKey(teamId), 'campaign-formats'],
    queryFn: async () => (await getApiClient().listCampaignFormats(teamId)).items.map(mapCampaignFormat),
    enabled: Boolean(teamId),
  })
}

export function useCreateCampaignFormat() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, data }: { teamId: string; data: Parameters<ApiClient['createCampaignFormat']>[1] }) =>
      mapCampaignFormat(await getApiClient().createCampaignFormat(teamId, data)),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: [...teamKey(variables.teamId), 'campaign-formats'] })
      void queryClient.invalidateQueries({ queryKey: ['ai-context', variables.teamId] })
    },
  })
}

export function useUpdateCampaignFormat() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      teamId,
      formatId,
      data,
    }: {
      teamId: string
      formatId: string
      data: Parameters<ApiClient['updateCampaignFormat']>[2]
    }) => mapCampaignFormat(await getApiClient().updateCampaignFormat(teamId, formatId, data)),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: [...teamKey(variables.teamId), 'campaign-formats'] })
      void queryClient.invalidateQueries({ queryKey: ['ai-context', variables.teamId] })
    },
  })
}

export function useDeleteCampaignFormat() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, formatId }: { teamId: string; formatId: string }) => {
      await getApiClient().deleteCampaignFormat(teamId, formatId)
    },
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: [...teamKey(variables.teamId), 'campaign-formats'] })
      void queryClient.invalidateQueries({ queryKey: ['ai-context', variables.teamId] })
    },
  })
}

export function useStyleExamples(teamId: string) {
  return useQuery({
    queryKey: [...teamKey(teamId), 'style-examples'],
    queryFn: async () => (await getApiClient().listStyleExamples(teamId)).items.map(mapStyleExample),
    enabled: Boolean(teamId),
  })
}

export function useCreateStyleExample() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, data }: { teamId: string; data: Parameters<ApiClient['createStyleExample']>[1] }) =>
      mapStyleExample(await getApiClient().createStyleExample(teamId, data)),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: [...teamKey(variables.teamId), 'style-examples'] })
      void queryClient.invalidateQueries({ queryKey: ['ai-context', variables.teamId] })
    },
  })
}

export function useDeleteStyleExample() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, exampleId }: { teamId: string; exampleId: string }) => {
      await getApiClient().deleteStyleExample(teamId, exampleId)
    },
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: [...teamKey(variables.teamId), 'style-examples'] })
      void queryClient.invalidateQueries({ queryKey: ['ai-context', variables.teamId] })
    },
  })
}

export function useTriggerAIJob() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      teamId,
      type,
      params,
    }: {
      teamId: string
      type: Parameters<ApiClient['triggerAIJob']>[1]
      params: Parameters<ApiClient['triggerAIJob']>[2]
    }) => getApiClient().triggerAIJob(teamId, type, params),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: ['ai-jobs', variables.teamId] })
    },
  })
}

export function useAIJobs(teamId: string) {
  return useQuery({
    queryKey: ['ai-jobs', teamId],
    queryFn: async () => (await getApiClient().listAIJobs(teamId)).items.map(mapAIJob),
    enabled: Boolean(teamId),
    refetchInterval: 5000,
  })
}

export function useAIJob(teamId: string, jobId: string) {
  return useQuery({
    queryKey: jobKey(teamId, jobId),
    queryFn: async () => mapAIJob(await getApiClient().getAIJob(teamId, jobId)),
    enabled: Boolean(teamId && jobId),
  })
}

export function useCancelAIJob() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, jobId }: { teamId: string; jobId: string }) =>
      mapAIJob(await getApiClient().cancelAIJob(teamId, jobId)),
    onSuccess: (job, variables) => {
      queryClient.setQueryData(jobKey(variables.teamId, job.id), job)
      queryClient.setQueryData<ReturnType<typeof mapAIJob>[]>(['ai-jobs', variables.teamId], (old) => {
        if (!old) return [job]
        const idx = old.findIndex((j) => j.id === job.id)
        if (idx === -1) return [job, ...old]
        const updated = [...old]
        updated[idx] = job
        return updated
      })
    },
  })
}

export function useRSSFeeds(teamId: string) {
  return useQuery({
    queryKey: [...teamKey(teamId), 'rss-feeds'],
    queryFn: async () => (await getApiClient().listRSSFeeds(teamId)).items.map(mapRSSFeedConfig),
    enabled: Boolean(teamId),
    refetchInterval: 30_000,
  })
}

export function useCreateRSSFeed() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, data }: { teamId: string; data: Parameters<ApiClient['createRSSFeed']>[1] }) =>
      mapRSSFeedConfig(await getApiClient().createRSSFeed(teamId, data)),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: [...teamKey(variables.teamId), 'rss-feeds'] })
    },
  })
}

export function useUpdateRSSFeed() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      teamId,
      feedId,
      data,
    }: {
      teamId: string
      feedId: string
      data: Parameters<ApiClient['updateRSSFeed']>[2]
    }) => mapRSSFeedConfig(await getApiClient().updateRSSFeed(teamId, feedId, data)),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: [...teamKey(variables.teamId), 'rss-feeds'] })
    },
  })
}

export function useDeleteRSSFeed() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ teamId, feedId }: { teamId: string; feedId: string }) => {
      await getApiClient().deleteRSSFeed(teamId, feedId)
    },
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: [...teamKey(variables.teamId), 'rss-feeds'] })
    },
  })
}

export function useAIServiceConfig(teamId: string) {
  return useQuery({
    queryKey: [...teamKey(teamId), 'ai-service-config'],
    queryFn: async () => mapAIServiceConfig(await getApiClient().getAIServiceConfig(teamId)),
    enabled: Boolean(teamId),
  })
}

export function useUpsertAIServiceConfig() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      teamId,
      data,
    }: {
      teamId: string
      data: Parameters<ApiClient['upsertAIServiceConfig']>[1]
    }) => mapAIServiceConfig(await getApiClient().upsertAIServiceConfig(teamId, data)),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: [...teamKey(variables.teamId), 'ai-service-config'] })
    },
  })
}
