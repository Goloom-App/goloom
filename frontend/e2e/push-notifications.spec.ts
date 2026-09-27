import { expect, test, type BrowserContext, type Page, type Route, type Worker } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { getFirstTeamId, signIn } from './helpers'

// Web Push in CI cannot reach a real push service (headless Chromium rejects
// the Push API outright), so this spec is split into two halves that together
// cover the full acceptance path without a real subscription:
//
//   1. Settings UI — the backend push endpoints are stubbed via page.route with
//      stateful responses (GET/POST/DELETE subscription, PATCH prefs, test
//      send), and Notification + PushManager are stubbed via addInitScript
//      (permission granted, subscribe() returning a deterministic fake
//      subscription). The app's real flow still runs: register() on /sw.js,
//      the mutation pipeline, per-team pref toggle, enable/test/remove actions.
//      Server-side delivery is covered by the Go tests; the backend handlers
//      are already verified there, so the UI test only needs the recorded
//      subscription and its lifecycle.
//
//   2. Service worker — the sw.js push/navigation code is exercised with a
//      real service worker. A decoded push is injected directly into the
//      worker via CDP (ServiceWorker.enable / deliverPushMessage), the grouped
//      notification is captured atomically in one evaluate, and
//      notificationclick is dispatched inside the worker via its own evaluate
//      so the deep-link navigation runs for real.
//
// This spec runs on the full managed Chromium project (playwright.config.ts
// `channel: 'chromium'`, the only project): chrome-headless-shell ships without
// a Push API, so deliverPushMessage never reaches a worker there. The
// worker-side setAppBadge IPC crashes the headless renderer in both flavors, so
// setAppBadge is no-op'd while clearAppBadge is recorded on the worker handle
// before any push; the zero-count sync path is then asserted end-to-end
// (notification closed, clearAppBadge called).

function base64UrlEncode(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

// A structurally valid uncompressed P-256 point (the curve base point G,
// 0x04 || X || Y). Malformed key bytes can crash Chromium's push processor,
// and the backend derives nothing from them in these tests.
const G_POINT_BYTES = new Uint8Array([
  0x04,
  0x6b, 0x17, 0xd1, 0xf2, 0xe1, 0x2c, 0x42, 0x47,
  0xf8, 0xbc, 0xe6, 0xe5, 0x63, 0xa4, 0x40, 0xf2,
  0x77, 0x03, 0x7d, 0x81, 0x2d, 0xeb, 0x33, 0xa0,
  0xf4, 0xa1, 0x39, 0x45, 0xd8, 0x98, 0xc2, 0x96,
  0x4f, 0xe3, 0x42, 0xe2, 0xfe, 0x1a, 0x7f, 0x9b,
  0x8e, 0xe7, 0xeb, 0x4a, 0x7c, 0x0f, 0x9e, 0x16,
  0x2b, 0xce, 0x33, 0x57, 0x6b, 0x31, 0x5e, 0xce,
  0xcb, 0xb6, 0x40, 0x68, 0x37, 0xbf, 0x51, 0xf5,
])

interface StubSubscription {
  id: string
  user_id: string
  endpoint: string
  p256dh: string
  auth: string
  enabled: boolean
  created_at: string
  updated_at: string
}

interface StubPref {
  user_id: string
  team_id: string
  team_name: string
  enabled: boolean
  updated_at: string
}

// In-memory backend for the push endpoints the UI test depends on. The real
// handlers are Go-tested; here only the app's client-side lifecycle needs to
// see coherent responses. Kept small and explicit on purpose.
class FakePushBackend {
  subscriptions: StubSubscription[] = []
  prefs: StubPref[] = []

  constructor(private readonly teamID: string, initialSubscriptions: StubSubscription[] = []) {
    this.subscriptions = initialSubscriptions
    this.prefs = [
      {
        user_id: 'e2e',
        team_id: teamID,
        team_name: 'E2E Review Team',
        enabled: true,
        updated_at: new Date().toISOString(),
      },
    ]
  }

  async handle(route: Route) {
    const request = route.request()
    const pathname = new URL(request.url()).pathname
    const method = request.method()

    try {
      if (pathname === '/v1/me/push/vapid-key') {
        return await route.fulfill({ json: { public_key: base64UrlEncode(G_POINT_BYTES) } })
      }

      if (pathname === '/v1/me/push-subscriptions') {
        if (method === 'GET') {
          return await route.fulfill({ json: { items: this.subscriptions } })
        }
        if (method === 'POST') {
          const body = request.postDataJSON() as { endpoint: string; keys: { p256dh: string; auth: string } }
          const now = new Date().toISOString()
          const item: StubSubscription = {
            id: `e2e-device-${this.subscriptions.length + 1}`,
            user_id: 'e2e',
            endpoint: body.endpoint,
            p256dh: body.keys.p256dh,
            auth: body.keys.auth,
            enabled: true,
            created_at: now,
            updated_at: now,
          }
          this.subscriptions.push(item)
          return await route.fulfill({ status: 201, json: { item } })
        }
        return await route.fulfill({ status: 405, json: { error: 'method not allowed' } })
      }

      const subMatch = pathname.match(/^\/v1\/me\/push-subscriptions\/([^/]+)$/)
      if (subMatch) {
        const id = subMatch[1]
        if (method === 'PATCH') {
          const body = request.postDataJSON() as { enabled: boolean }
          const item = this.subscriptions.find((s) => s.id === id)
          if (!item) {
            return await route.fulfill({ status: 404, json: { error: 'not found' } })
          }
          item.enabled = body.enabled
          item.updated_at = new Date().toISOString()
          return await route.fulfill({ json: { item } })
        }
        if (method === 'DELETE') {
          this.subscriptions = this.subscriptions.filter((s) => s.id !== id)
          return await route.fulfill({ status: 204, body: '' })
        }
        return await route.fulfill({ status: 405, json: { error: 'method not allowed' } })
      }

      const testMatch = pathname.match(/^\/v1\/me\/push-subscriptions\/([^/]+)\/test$/)
      if (testMatch && method === 'POST') {
        // Delivery success is Go-tested; the fake endpoint cannot receive a
        // real webpush request, so the UI just needs the ok response.
        return await route.fulfill({ status: 204, body: '' })
      }

      if (pathname === '/v1/me/team-notification-prefs') {
        return await route.fulfill({ json: { items: this.prefs } })
      }

      const prefMatch = pathname.match(/^\/v1\/me\/team-notification-prefs\/([^/]+)$/)
      if (prefMatch && method === 'PATCH') {
        const teamID = prefMatch[1]
        const body = request.postDataJSON() as { enabled: boolean }
        let pref = this.prefs.find((p) => p.team_id === teamID)
        if (!pref) {
          pref = {
            user_id: 'e2e',
            team_id: teamID,
            team_name: 'E2E Review Team',
            enabled: body.enabled,
            updated_at: new Date().toISOString(),
          }
          this.prefs.push(pref)
        } else {
          pref.enabled = body.enabled
          pref.updated_at = new Date().toISOString()
        }
        return await route.fulfill({ json: { item: pref } })
      }

      return await route.fulfill({ status: 404, json: { error: 'not found' } })
    } catch (error) {
      return await route.fulfill({ status: 500, json: { error: String(error) } })
    }
  }
}

// Chromium's headless shell kills the renderer (error code 3) when a service
// worker calls navigator.setAppBadge. addInitScript only patches the window
// global, never the worker global, so the worker handle must neutralize it
// directly before any push is delivered or any app badge message reaches the
// worker. Production sw.js stays unchanged; page-level setAppBadge still runs
// for real through the app. The worker can restart once during registration,
// so evaluation is retried.
async function neutralizeWorkerBadge(worker: Worker) {
  let lastError: unknown
  for (let attempt = 0; attempt < 5; attempt++) {
    try {
      await worker.evaluate(() => {
        const navigator = self.navigator as unknown as {
          setAppBadge?: unknown
          clearAppBadge?: unknown
          __proto__: { setAppBadge?: unknown; clearAppBadge?: unknown }
        }
        const noop = () => Promise.resolve(undefined)
        const recordedClear = () => {
          const win = self as unknown as { __clearAppBadgeCalls: number }
          win.__clearAppBadgeCalls = (win.__clearAppBadgeCalls ?? 0) + 1
          return Promise.resolve(undefined)
        }
        // setAppBadge is no-op'd: its IPC crashes the headless renderer.
        try {
          Object.defineProperty(navigator.__proto__, 'setAppBadge', {
            configurable: true,
            writable: true,
            value: noop,
          })
        } catch {
          Object.defineProperty(navigator, 'setAppBadge', { configurable: true, value: noop })
        }
        // clearAppBadge is recorded (not no-op'd) so the zero-count path can
        // be asserted without tripping the same headless crash.
        try {
          Object.defineProperty(navigator.__proto__, 'clearAppBadge', {
            configurable: true,
            writable: true,
            value: recordedClear,
          })
        } catch {
          Object.defineProperty(navigator, 'clearAppBadge', { configurable: true, value: recordedClear })
        }
      })
      return
    } catch (error) {
      lastError = error
      await new Promise((resolve) => setTimeout(resolve, 500))
    }
  }
  throw lastError
}

async function stubPushApis(page: Page, { stubServiceWorkerRegister = true } = {}) {
  await page.addInitScript(({ stubRegister }: { stubRegister: boolean }) => {
    const win = window as unknown as {
      Notification: typeof Notification
      PushManager: typeof PushManager
    }
    // Notification: always permitted; the real permission prompt is irrelevant
    // here and headless Chromium would return 'denied' anyway.
    Object.defineProperty(win.Notification, 'permission', { get: () => 'granted' })
    win.Notification.requestPermission = async () => 'granted'

    // PushManager: deterministic subscription, no network, no service worker.
    const key = new Uint8Array([
      0x04,
      0x6b, 0x17, 0xd1, 0xf2, 0xe1, 0x2c, 0x42, 0x47,
      0xf8, 0xbc, 0xe6, 0xe5, 0x63, 0xa4, 0x40, 0xf2,
      0x77, 0x03, 0x7d, 0x81, 0x2d, 0xeb, 0x33, 0xa0,
      0xf4, 0xa1, 0x39, 0x45, 0xd8, 0x98, 0xc2, 0x96,
      0x4f, 0xe3, 0x42, 0xe2, 0xfe, 0x1a, 0x7f, 0x9b,
      0x8e, 0xe7, 0xeb, 0x4a, 0x7c, 0x0f, 0x9e, 0x16,
      0x2b, 0xce, 0x33, 0x57, 0x6b, 0x31, 0x5e, 0xce,
      0xcb, 0xb6, 0x40, 0x68, 0x37, 0xbf, 0x51, 0xf5,
    ])
    const auth = new Uint8Array(16).fill(0x09)
    const sub = {
      endpoint: 'https://push.example.test/goloom-e2e',
      getKey(name: string) {
        return name === 'p256dh' ? key : auth
      },
      toJSON() {
        return { endpoint: this.endpoint, keys: {} }
      },
      unsubscribe: async () => {
        const win = window as unknown as { __e2eUnsubscribed: boolean }
        win.__e2eUnsubscribed = true
        return true
      },
    }
    // getSubscription returns the fake subscription too: the app correlates
    // the *current browser device* via its endpoint and must scope enable,
    // test and remove operations to that device only.
    const pushManager = {
      subscribe: async () => sub as unknown as PushSubscription,
      getSubscription: async () => sub as unknown as PushSubscription,
      permissionState: async () => 'granted' as PermissionState,
    }
    win.PushManager.prototype.subscribe = pushManager.subscribe
    win.PushManager.prototype.getSubscription = pushManager.getSubscription
    win.PushManager.prototype.permissionState = pushManager.permissionState
    // The UI test runs without a real service worker: register() answers with
    // a fake registration whose pushManager is stubbed, so no worker target is
    // spawned, no controller is ever claimed, and the app badge message never
    // reaches sw.js (whose setAppBadge would kill the headless shell). sw.js
    // itself is exercised for real in the CDP test below.
    if (stubRegister) {
      const navigatorSW = navigator.serviceWorker as unknown as {
        register: () => Promise<unknown>
      }
      navigatorSW.register = async () => ({ pushManager, active: null })
    }
  }, { stubRegister: stubServiceWorkerRegister })
}

async function skipTour(page: Page) {
  const skipTour = page.getByRole('button', { name: 'Skip tour' })
  if (await skipTour.isVisible({ timeout: 2_000 }).catch(() => false)) {
    await skipTour.click()
  }
}

async function openPushSettings(page: Page) {
  await signIn(page)
  await skipTour(page)
  await page.getByTestId('user-menu-trigger').click()
  await page.getByRole('menuitem', { name: 'Settings' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible()
}

// Registers /sw.js and returns the worker without a race: the event listener
// is set up (or an already-present worker reused) before register() runs, so
// an early serviceworker event cannot be missed.
async function registerWorker(page: Page, context: BrowserContext): Promise<Worker> {
  const existing = context.serviceWorkers()[0]
  if (existing) {
    return existing
  }
  const waitForWorker = context.waitForEvent('serviceworker')
  await page.evaluate(() => navigator.serviceWorker.register('/sw.js'))
  return waitForWorker
}

test('settings UI: enable, test, remove and per-team toggle with a stubbed browser', async ({ page, baseURL }) => {
  test.setTimeout(60_000)
  await page.setViewportSize({ width: 1280, height: 900 })

  const teamId = String(await getFirstTeamId(baseURL!, e2eBootstrapToken()))
  const fakeBackend = new FakePushBackend(teamId)
  await page.route('**/v1/me/push/**', (route) => fakeBackend.handle(route))
  await page.route('**/v1/me/push-subscriptions**', (route) => fakeBackend.handle(route))
  await page.route('**/v1/me/team-notification-prefs**', (route) => fakeBackend.handle(route))

  await stubPushApis(page)
  await openPushSettings(page)

  // Enable runs the real subscription pipeline (register + permission + POST);
  // register() is stubbed to a fake registration, so sw.js never becomes the
  // controller and the headless-shell setAppBadge crash cannot be reached.
  await page.getByTestId('push-enable').click()
  await expect(page.getByTestId('push-test')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('push-remove')).toBeVisible()

  // The permission/status line is explicit: this stub browser has the
  // permission granted and the current device registered.
  await expect(page.getByTestId('push-permission')).toContainText('granted')

  // Test send reports success over the (stubbed) backend: 204 delivered.
  await page.getByTestId('push-test').click()
  await expect(page.getByTestId('push-test-ok')).toBeVisible({ timeout: 15_000 })

  // The current team is owner-visible, toggleable, and on by default. A single
  // click flips the pref; the onChange sends the PATCH and re-renders once the
  // pref query refetches, so the assertion polls instead of uncheck()-clicking
  // (which would retry-click while the controlled input is still checked and
  // toggle the new value back).
  const teamToggle = page.getByTestId(`push-team-pref-${teamId}`)
  await expect(teamToggle).toBeVisible()
  await expect(teamToggle.locator('input')).toBeChecked()
  // The selector shows the team name, never the raw UUID.
  await expect(teamToggle).toContainText('E2E Review Team')
  await expect(teamToggle).not.toContainText(teamId)
  await teamToggle.locator('input').evaluate((element) => {
    ;(element as HTMLInputElement).click()
  })
  await expect(teamToggle.locator('input')).not.toBeChecked({ timeout: 15_000 })

  // Removing the current device unsubscribes the browser subscription and
  // clears it on the backend.
  await page.getByTestId('push-remove').click()
  await expect(page.getByTestId('push-remove')).not.toBeVisible()
  await expect(page.getByTestId('push-enable')).toBeVisible()
  const unsubscribed = await page.evaluate(() => (window as unknown as { __e2eUnsubscribed?: boolean }).__e2eUnsubscribed)
  await expect.poll(() => Promise.resolve(unsubscribed)).toBe(true)
})

test('service worker: grouped push delivery and deep link via CDP', async ({ page, context, baseURL }) => {
  test.setTimeout(60_000)
  await page.setViewportSize({ width: 1280, height: 900 })
  // Full Chromium still needs the origin granted for showNotification.
  await context.grantPermissions(['notifications'])
  const teamId = String(await getFirstTeamId(baseURL!, e2eBootstrapToken()))
  // The app's own badge sync (real backend totals zero on a seeded DB) would
  // post review-badge messages racing the manual deliveries below and could
  // close the test notifications. Stub the counts to a stable non-zero total so
  // its message handler never closes review-* notifications mid-test.
  await page.route('**/v1/me/review-counts', (route) => route.fulfill({ json: { total: 2, by_team: { [teamId]: 2 } } }))
  // Keep the real serviceWorker.register so sw.js actually spawns.
  await stubPushApis(page, { stubServiceWorkerRegister: false })
  await openPushSettings(page)

  // Register the real service worker and disable its badge IPC first; the
  // push handler calls applyBadge before showNotification.
  const worker = await registerWorker(page, context)
  await neutralizeWorkerBadge(worker)

  const session = await context.newCDPSession(page)
  await session.send('ServiceWorker.enable')
  const registrationId = await new Promise<string>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('no service worker registration')), 15_000)
    session.on('ServiceWorker.workerRegistrationUpdated', (event) => {
      const registrations =
        (event as { registrations?: { scopeURL?: string; registrationId?: string; isDeleted?: boolean }[] }).registrations ?? []
      for (const registration of registrations) {
        if (registration.scopeURL === `${baseURL}/` && !registration.isDeleted && registration.registrationId) {
          clearTimeout(timer)
          resolve(registration.registrationId)
          return
        }
      }
    })
  })

  // Two deliveries for the same team: the second replaces the first (same tag),
  // so exactly one grouped notification remains. deliverPushMessage passes the
  // string to the worker as-is (no base64 round-trip), so plain JSON is sent.
  await neutralizeWorkerBadge(worker)
  for (const count of [1, 2]) {
    const payload = JSON.stringify({
      title: 'Acme Team',
      body: `Acme Team · ${count} open`,
      data: { team_id: teamId, team_name: 'Acme Team', count, post_id: 'seed', url: `/?team=${teamId}&section=reviewQueue` },
    })
    await session.send('ServiceWorker.deliverPushMessage', { origin: baseURL!, registrationId, data: payload })
  }

  // Capture the grouped notification atomically in one evaluate: wait until
  // exactly one notification with the team tag exists, then read it out.
  const snapshot = await page.evaluate(async (tag) => {
    const registration = await navigator.serviceWorker.getRegistration()
    const deadline = Date.now() + 15_000
    while (Date.now() < deadline) {
      const notifications = registration ? await registration.getNotifications() : []
      const grouped = notifications.filter((n) => n.tag === tag)
      if (grouped.length === 1) {
        return { title: grouped[0].title, body: grouped[0].body, tag: grouped[0].tag, count: grouped.length }
      }
      await new Promise((resolve) => setTimeout(resolve, 250))
    }
    return null
  }, `review-${teamId}`)

  if (!snapshot) {
    throw new Error(`no grouped notification with tag review-${teamId} appeared within 15s`)
  }
  expect(snapshot?.title).toBe('Acme Team')
  expect(snapshot?.body).toBe('Acme Team · 2 open')
  expect(snapshot?.tag).toBe(`review-${teamId}`)

  // Click the notification by dispatching a notificationclick event inside the
  // worker; the handler's deep-link navigation then runs for real.
  const clicked = (await worker.evaluate(async (tag) => {
    const scope = self as unknown as { registration: ServiceWorkerRegistration } & EventTarget
    const notifications = await scope.registration.getNotifications()
    const notification = notifications.find((n) => n.tag === tag)
    if (!notification) {
      return false
    }
    const event = new Event('notificationclick') as Event & { notification: Notification }
    event.notification = notification
    scope.dispatchEvent(event)
    return true
  }, `review-${teamId}`)) as boolean
  expect(clicked).toBe(true)

  await page.waitForURL(/\?team=.*&section=reviewQueue/, { timeout: 15_000 })
  await expect(page.getByTestId('review-queue')).toBeVisible()

  // Zero-count sync drop (blocker 7): when the app reports the team's open
  // count at zero, the worker closes the team's grouped notification and
  // clears the badge. The real sw.js message handler runs here.
  await page.evaluate(async (teamId) => {
    const registration = await navigator.serviceWorker.getRegistration()
    registration?.active?.postMessage({
      type: 'review-badge',
      total: 0,
      teams: [{ team_id: teamId, count: 0 }],
    })
  }, teamId)

  const cleared = await page.evaluate(async (tag) => {
    const registration = await navigator.serviceWorker.getRegistration()
    const deadline = Date.now() + 15_000
    while (Date.now() < deadline) {
      const notifications = registration ? await registration.getNotifications() : []
      if (notifications.filter((n) => n.tag === tag).length === 0) {
        return true
      }
      await new Promise((resolve) => setTimeout(resolve, 250))
    }
    return false
  }, `review-${teamId}`)
  expect(cleared).toBe(true)
  const clearCalls = await worker.evaluate(() => (self as unknown as { __clearAppBadgeCalls?: number }).__clearAppBadgeCalls ?? 0)
  expect(clearCalls).toBeGreaterThan(0)

  // Zero-total close-all (finding 5): a badge sync reporting total zero with no
  // eligible team left (empty teams list) must close every active review-*
  // notification. Seed two tagged notifications deterministically through the
  // page context — the CDP push delivery path is already exercised above, so a
  // second rapid push event is not needed here.
  await page.evaluate(async () => {
    const registration = await navigator.serviceWorker.getRegistration()
    for (const tag of ['review-extra-a', 'review-extra-b']) {
      await registration?.showNotification(tag.replace('review-', ''), { tag, body: 'open' })
    }
  })
  const seeded = await page.evaluate(async (tags) => {
    const registration = await navigator.serviceWorker.getRegistration()
    const deadline = Date.now() + 10_000
    while (Date.now() < deadline) {
      const notifications = (await registration?.getNotifications()) ?? []
      const present = tags.filter((tag) => notifications.some((n) => n.tag === tag))
      if (present.length === tags.length) {
        return present
      }
      await new Promise((resolve) => setTimeout(resolve, 100))
    }
    return null
  }, ['review-extra-a', 'review-extra-b'])
  expect(seeded).toEqual(['review-extra-a', 'review-extra-b'])

  await page.evaluate(async () => {
    const registration = await navigator.serviceWorker.getRegistration()
    registration?.active?.postMessage({ type: 'review-badge', total: 0, teams: [] })
  })
  const allClosed = await page.evaluate(async () => {
    const registration = await navigator.serviceWorker.getRegistration()
    const deadline = Date.now() + 10_000
    while (Date.now() < deadline) {
      const notifications = (await registration?.getNotifications()) ?? []
      if (notifications.every((n) => !n.tag.startsWith('review-'))) {
        return true
      }
      await new Promise((resolve) => setTimeout(resolve, 100))
    }
    return false
  })
  expect(allClosed).toBe(true)
})

test('service worker: shipped worker keeps the async message closes alive via waitUntil', async ({ request }) => {
  // closeTeamNotifications/closeAllReviewNotifications are async: the close-all
  // must be kept alive with event.waitUntil(Promise.all(...)), or the service
  // worker can be suspended mid-close leaving stale notifications under a
  // cleared badge. The behavioral zero-total test above proves the closes run;
  // this asserts the worker actually shipped to the browser keeps them alive
  // instead of firing-and-forgetting.
  const response = await request.get('/sw.js')
  expect(response.ok()).toBe(true)
  const source = await response.text()
  expect(source).toContain('event.waitUntil(Promise.all(closeTasks))')
  expect(source).not.toContain('void closeAllReviewNotifications()')
  expect(source).not.toContain('void closeTeamNotifications')
})

test('badge sync: polls and applies for a cookie session without a stored bearer token', async ({ page }) => {
  test.setTimeout(60_000)
  await page.setViewportSize({ width: 1280, height: 900 })

  let countsRequests = 0
  await page.route('**/v1/me/review-counts', (route) => {
    countsRequests++
    return route.fulfill({ json: { total: 2, by_team: { team: 2 } } })
  })
  // Record page-level badge calls (stubbed so no real platform badge appears).
  await page.addInitScript(() => {
    const win = window as unknown as { __pageBadgeCalls: number }
    win.__pageBadgeCalls = 0
    const proto = Navigator.prototype as unknown as { setAppBadge?: unknown; clearAppBadge?: unknown }
    Object.defineProperty(proto, 'setAppBadge', { configurable: true, value: () => { win.__pageBadgeCalls++ } })
    Object.defineProperty(proto, 'clearAppBadge', { configurable: true, value: () => { win.__pageBadgeCalls++ } })
  })

  // signIn establishes a cookie session (sessionFromToken): the bootstrap token
  // is never stored client-side, so a bearer-token gate would keep badge sync
  // off. The App-level hook must follow the authenticated /v1/me probe instead.
  await signIn(page)

  await expect
    .poll(() => page.evaluate(() => (window as unknown as { __pageBadgeCalls?: number }).__pageBadgeCalls ?? 0), { timeout: 30_000 })
    .toBeGreaterThan(0)
  expect(countsRequests).toBeGreaterThan(0)

  const settings = await page.evaluate(() => JSON.parse(localStorage.getItem('goloom-ui-settings') ?? '{}'))
  expect((settings as { general?: { bearerToken?: string } }).general?.bearerToken?.trim() ?? '').toBe('')
})

test('removing the current device leaves other devices enabled and unsubscribes the browser', async ({ page, baseURL }) => {
  test.setTimeout(60_000)
  await page.setViewportSize({ width: 1280, height: 900 })

  const teamId = String(await getFirstTeamId(baseURL!, e2eBootstrapToken()))
  const now = new Date().toISOString()
  const currentDevice: StubSubscription = {
    id: 'e2e-device-current',
    user_id: 'e2e',
    endpoint: 'https://push.example.test/goloom-e2e',
    p256dh: 'p256dh-current',
    auth: 'auth-current',
    enabled: true,
    created_at: now,
    updated_at: now,
  }
  const otherDevice: StubSubscription = {
    id: 'e2e-device-other',
    user_id: 'e2e',
    endpoint: 'https://push.example.test/other-device',
    p256dh: 'p256dh-other',
    auth: 'auth-other',
    enabled: true,
    created_at: now,
    updated_at: now,
  }
  const fakeBackend = new FakePushBackend(teamId, [currentDevice, otherDevice])
  await page.route('**/v1/me/push/**', (route) => fakeBackend.handle(route))
  await page.route('**/v1/me/push-subscriptions**', (route) => fakeBackend.handle(route))
  await page.route('**/v1/me/team-notification-prefs**', (route) => fakeBackend.handle(route))

  await stubPushApis(page)
  await openPushSettings(page)

  // This device exists on the account (endpoint matches getSubscription), so
  // only it exposes a remove control; the other device is listed read-only.
  await expect(page.getByTestId('push-remove')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('push-remove')).toHaveCount(1)
  await expect(page.getByText('https://push.example.test/other-device')).toBeVisible()

  await page.getByTestId('push-remove').click()
  await expect(page.getByTestId('push-remove')).not.toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('push-enable')).toBeVisible()

  // The browser subscription was unsubscribed (per-blocker requirement) and
  // the other device survives removal untouched.
  const unsubscribed = await page.evaluate(() => (window as unknown as { __e2eUnsubscribed?: boolean }).__e2eUnsubscribed)
  await expect.poll(() => Promise.resolve(unsubscribed)).toBe(true)
  expect(fakeBackend.subscriptions).toEqual([otherDevice])
  await expect(page.getByText('https://push.example.test/other-device')).toBeVisible({ timeout: 15_000 })
})