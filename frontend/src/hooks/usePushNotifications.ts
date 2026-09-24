import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getApiClient } from './useAI'

const SUBSCRIPTIONS_KEY = ['me', 'push-subscriptions'] as const
const PREFS_KEY = ['me', 'team-notification-prefs'] as const
const COUNTS_KEY = ['me', 'review-counts'] as const

export function pushSupported(): boolean {
  return typeof window !== 'undefined' && 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window
}

function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4)
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(base64)
  const output = new Uint8Array(new ArrayBuffer(raw.length))
  for (let i = 0; i < raw.length; i++) {
    output[i] = raw.charCodeAt(i)
  }
  return output
}

function base64Url(bytes: ArrayBuffer | null): string {
  if (!bytes) {
    return ''
  }
  const binary = String.fromCharCode(...new Uint8Array(bytes))
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

export type ByTeamCounts = Record<string, number>

// applyAppBadge mirrors the server-side open review count into the platform
// badge. On zero it clears the badge (clearAppBadge where available, setAppBadge
// fallback). The service worker receives both the total and the per-team counts
// so it can close a team's grouped notification once that team has nothing open.
function applyAppBadge(total: number, byTeam: ByTeamCounts = {}) {
  const count = Math.max(0, total)
  const nav = navigator as Navigator & {
    setAppBadge?: (count: number) => Promise<void>
    clearAppBadge?: () => Promise<void>
  }
  if (count === 0) {
    if (nav.clearAppBadge) {
      void nav.clearAppBadge().catch(() => {})
    } else {
      void nav.setAppBadge?.(0)?.catch(() => {})
    }
  } else {
    void nav.setAppBadge?.(count)?.catch(() => {})
  }
  const teams = Object.entries(byTeam).map(([team_id, c]) => ({ team_id, count: Math.max(0, c) }))
  navigator.serviceWorker?.controller?.postMessage({ type: 'review-badge', total: count, teams })
}

// currentPushSubscription resolves the *browser's own* subscription for this
// origin. It is the device anchor for all device-scoped operations: enabling,
// testing and removal must never touch subscriptions registered from other
// devices with the same account.
async function currentPushSubscription(): Promise<PushSubscription | null> {
  const reg = await navigator.serviceWorker.register('/sw.js')
  return reg.pushManager.getSubscription()
}

async function subscribeOnDevice(): Promise<string> {
  const reg = await navigator.serviceWorker.register('/sw.js')
  const permission = await Notification.requestPermission()
  if (permission !== 'granted') {
    throw new Error('notification-permission-denied')
  }
  const api = getApiClient()
  const { public_key: publicKey } = await api.getMyVAPIDPublicKey()
  const sub = await reg.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: urlBase64ToUint8Array(publicKey),
  })
  const { item } = await api.createMyPushSubscription(sub.endpoint, {
    p256dh: base64Url(sub.getKey('p256dh')),
    auth: base64Url(sub.getKey('auth')),
  })
  return item.id
}

// useReviewBadgeSync keeps the app badge aligned with the server-side open
// review count while the app is running: mounts at app level (not just in the
// settings tab), polls every 30s and mirrors the total into the browser badge
// plus the service worker, whose notifications push the same value. Enabled by
// the app's authenticated state — the /v1/me probe — not by a stored bearer
// token, so OIDC cookie-session users sync badges too.
export function useReviewBadgeSync(authenticated: boolean) {
  return useQuery({
    queryKey: COUNTS_KEY,
    queryFn: async () => {
      const counts = await getApiClient().myReviewCounts()
      if (pushSupported()) {
        applyAppBadge(counts.total, counts.by_team)
      }
      return counts
    },
    enabled: pushSupported() && authenticated,
    refetchInterval: 30_000,
  })
}

export function usePushNotifications() {
  const client = useQueryClient()
  const api = getApiClient()

  const subscriptionsQuery = useQuery({
    queryKey: SUBSCRIPTIONS_KEY,
    queryFn: async () => (await api.listMyPushSubscriptions()).items,
    enabled: pushSupported(),
  })
  const prefsQuery = useQuery({
    queryKey: PREFS_KEY,
    queryFn: async () => (await api.listMyTeamNotificationPrefs()).items,
    enabled: pushSupported(),
  })
  const countsQuery = useQuery({
    queryKey: COUNTS_KEY,
    queryFn: async () => {
      const counts = await api.myReviewCounts()
      if (pushSupported()) {
        applyAppBadge(counts.total, counts.by_team)
      }
      return counts
    },
    enabled: pushSupported(),
    refetchInterval: 30_000,
  })

  const [currentEndpoint, setCurrentEndpoint] = useState<string>('')

  // Re-resolve the browser's own subscription whenever the server list changes
  // (subscribe/disable/remove mutate it) so the "this device" marker and the
  // device-scoped actions stay correct.
  useEffect(() => {
    let cancelled = false
    void (async () => {
      if (!pushSupported()) {
        return
      }
      try {
        const sub = await currentPushSubscription()
        if (!cancelled) {
          setCurrentEndpoint(sub?.endpoint ?? '')
        }
      } catch {
        if (!cancelled) {
          setCurrentEndpoint('')
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [subscriptionsQuery.data])

  const subscriptions = subscriptionsQuery.data ?? []
  const currentSub = subscriptions.find((sub) => sub.endpoint === currentEndpoint)

  const subscribeMutation = useMutation({
    mutationFn: subscribeOnDevice,
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: SUBSCRIPTIONS_KEY })
      void client.invalidateQueries({ queryKey: COUNTS_KEY })
    },
  })

  // Device-scoped master toggle: only the browser's own subscription changes,
  // never the subscriptions registered from other devices.
  const setDeviceEnabled = useMutation({
    mutationFn: async (enabled: boolean) => {
      if (!currentSub) {
        return
      }
      await api.updateMyPushSubscription(currentSub.id, { enabled })
    },
    onSuccess: () => void client.invalidateQueries({ queryKey: SUBSCRIPTIONS_KEY }),
  })

  const subscribeDeviceEnabled = useMutation({
    mutationFn: async (enabled: boolean) => {
      if (!currentSub) {
        await subscribeOnDevice()
        return
      }
      await api.updateMyPushSubscription(currentSub.id, { enabled })
    },
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: SUBSCRIPTIONS_KEY })
    },
  })

  // Removing the current device must also unsubscribe the browser
  // subscription; other devices' subscriptions are left untouched.
  const removeCurrentDevice = useMutation({
    mutationFn: async () => {
      const browserSub = await currentPushSubscription()
      if (browserSub) {
        await browserSub.unsubscribe().catch(() => {})
      }
      if (currentSub) {
        await api.deleteMyPushSubscription(currentSub.id)
      }
    },
    onSuccess: () => void client.invalidateQueries({ queryKey: SUBSCRIPTIONS_KEY }),
  })

  const sendTest = useMutation({
    mutationFn: async () => {
      if (!currentSub) {
        return
      }
      await api.sendMyPushTest(currentSub.id)
    },
  })

  const setTeamEnabled = useMutation({
    mutationFn: async ({ teamID, enabled }: { teamID: string; enabled: boolean }) => api.setMyTeamNotificationPref(teamID, enabled),
    onSuccess: () => void client.invalidateQueries({ queryKey: PREFS_KEY }),
  })

  return {
    supported: pushSupported(),
    loading: subscriptionsQuery.isLoading || prefsQuery.isLoading,
    subscriptions,
    currentSub,
    prefs: prefsQuery.data ?? [],
    counts: countsQuery.data,
    permission: pushSupported() ? Notification.permission : 'unsupported',
    subscribe: subscribeMutation.mutateAsync,
    subscribeError: subscribeMutation.isError,
    setDeviceEnabled: setDeviceEnabled.mutateAsync,
    subscribeDeviceEnabled: subscribeDeviceEnabled.mutateAsync,
    removeCurrentDevice: removeCurrentDevice.mutateAsync,
    sendTest: sendTest.mutateAsync,
    testBusy: sendTest.isPending,
    setTeamEnabled: setTeamEnabled.mutateAsync,
  }
}