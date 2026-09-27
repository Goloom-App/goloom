// Goloom Service Worker: browser push delivery + app badge for review items.
// Grouped per team (tag = review-<teamId>) so multiple new reviews collapse
// into one notification per team; the badge always reflects the server-side
// open count (cleared at zero).

self.addEventListener('install', (event) => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

function urlFor(data) {
  if (data && typeof data.url === 'string' && data.url.startsWith('/')) {
    return new URL(data.url, self.location.origin).toString()
  }
  return self.location.origin + '/'
}

async function closeTeamNotifications(teamId) {
  const tag = `review-${String(teamId)}`
  try {
    const notifications = await self.registration.getNotifications()
    for (const notification of notifications) {
      if (notification.tag === tag) {
        notification.close()
      }
    }
  } catch {
    // getNotifications may reject on some platforms; closing is best-effort.
  }
}

// A zero total with no eligible teams left leaves `teams` empty: without a
// team to close, every active review notification must still go away.
async function closeAllReviewNotifications() {
  try {
    const notifications = await self.registration.getNotifications()
    for (const notification of notifications) {
      if (typeof notification.tag === 'string' && notification.tag.startsWith('review-')) {
        notification.close()
      }
    }
  } catch {
    // best-effort, mirroring closeTeamNotifications
  }
}

function applyBadge(total) {
  const count = Math.max(0, Number(total) || 0)
  if (count === 0) {
    if ('clearAppBadge' in self.navigator) {
      self.navigator.clearAppBadge().catch(() => {})
    } else if ('setAppBadge' in self.navigator) {
      self.navigator.setAppBadge(0).catch(() => {})
    }
    return
  }
  if ('setAppBadge' in self.navigator) {
    self.navigator.setAppBadge(count).catch(() => {})
  }
}

self.addEventListener('push', (event) => {
  let notif
  try {
    notif = event.data ? event.data.json() : null
  } catch {
    notif = null
  }
  if (!notif || !notif.data) {
    return
  }
  const data = notif.data
  const tag = `review-${String(data.team_id)}`
  const title = notif.title || 'Goloom'
  const body = notif.body || data.team_name
  // total is the user's badge sum across all teams; count stays per-team for
  // the unread body. Fall back to count for payloads without total.
  applyBadge(data.total != null ? data.total : data.count)
  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      tag,
      icon: '/icon-192.png',
      badge: '/icon-512.png',
      data: { url: urlFor(data) },
    }),
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const url = (event.notification.data && event.notification.data.url) || self.location.origin + '/'
  event.waitUntil(
    (async () => {
      const windowClients = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
      const existing = windowClients.find((client) => new URL(client.url).origin === self.location.origin)
      if (existing) {
        await existing.navigate(url)
        return existing.focus()
      }
      return self.clients.openWindow(url)
    })(),
  )
})

// Tabs push their current server-side review count so the badge stays in sync
// while the app is open. A zero total closes every active review-* notification
// (no team is left eligible, so the teams list may be empty); a non-zero total
// still closes each listed team whose own count dropped to zero. The async
// closes are kept alive with event.waitUntil so the service worker cannot be
// suspended mid-close and leave stale notifications under a cleared badge.
self.addEventListener('message', (event) => {
  const message = event.data
  if (message && message.type === 'review-badge') {
    const total = Number(message.total) || 0
    const closeTasks = []
    if (total <= 0) {
      closeTasks.push(closeAllReviewNotifications())
    } else if (Array.isArray(message.teams)) {
      for (const team of message.teams) {
        if (Number(team.count) <= 0) {
          closeTasks.push(closeTeamNotifications(team.team_id))
        }
      }
    }
    if (closeTasks.length > 0) {
      event.waitUntil(Promise.all(closeTasks))
    }
    applyBadge(total)
  }
})