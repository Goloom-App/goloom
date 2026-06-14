import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

// The web session is an HttpOnly cookie (not a bearer token in localStorage), and
// cookie-authenticated state-changing requests require the CSRF double-submit token.
test('web session uses an HttpOnly cookie with CSRF protection', async ({ page }) => {
  test.setTimeout(60_000)
  await signIn(page)

  const cookies = await page.context().cookies()
  const session = cookies.find((c) => c.name === 'goloom_session')
  const csrf = cookies.find((c) => c.name === 'goloom_csrf')
  expect(session, 'session cookie present').toBeTruthy()
  expect(session?.httpOnly, 'session cookie HttpOnly').toBe(true)
  expect(csrf, 'csrf cookie present').toBeTruthy()
  expect(csrf?.httpOnly, 'csrf cookie readable by JS').toBe(false)

  // No web bearer token is persisted in localStorage.
  const storedBearer = await page.evaluate(() => {
    try {
      const raw = localStorage.getItem('goloom-ui-settings')
      return raw ? (JSON.parse(raw)?.general?.bearerToken ?? '') : ''
    } catch {
      return ''
    }
  })
  expect(storedBearer).toBeFalsy()

  // A cookie-authenticated POST without the CSRF header is rejected.
  const statusNoCsrf = await page.evaluate(async () => {
    const r = await fetch('/v1/auth/logout', { method: 'POST', credentials: 'include' })
    return r.status
  })
  expect(statusNoCsrf).toBe(403)
})
