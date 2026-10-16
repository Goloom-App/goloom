import { test, expect } from '@playwright/test'
import en from '../../locales/en.json' with { type: 'json' }
import de from '../../locales/de.json' with { type: 'json' }
import { e2eBootstrapToken } from './constants'

for (const kind of ['full authority', 'bootstrap recovery', 'scoped', 'team bound'] as const) {
  test(`token login: ${kind}`, async ({ page, request, context }) => {
    let token = e2eBootstrapToken()
    if (kind !== 'bootstrap recovery') {
      // A browser session can create team-bound tokens; an unbound API token
      // cannot delegate a different team binding.
      const session = await request.post('/v1/auth/session/token', { data: { token } })
      expect(session.ok()).toBeTruthy()
      const csrf = (await request.storageState()).cookies.find((cookie) => cookie.name === 'goloom_csrf')
      expect(csrf?.value).toBeTruthy()
      const headers = { 'X-CSRF-Token': csrf!.value }
      let teamId: string | undefined
      if (kind === 'team bound') {
        const teams = await request.get('/v1/teams', { headers })
        expect(teams.ok()).toBeTruthy()
        teamId = (await teams.json()).items[0].id
      }
      const created = await request.post('/v1/me/api-tokens', {
        headers,
        data: {
          name: `session-login-${kind}`,
          scopes: kind === 'scoped' ? ['read'] : [],
          team_id: teamId,
        },
      })
      expect(created.status()).toBe(201)
      token = (await created.json()).token
    }
    await page.goto('/?login=recovery')
    await page.getByLabel(/access token|administrator token/i).fill(token)
    const exchange = page.waitForResponse((response) => response.url().endsWith('/v1/auth/session/token'))
    await page.getByRole('button', { name: 'Sign in with token' }).click()
    const response = await exchange
    const allowed = kind === 'full authority' || kind === 'bootstrap recovery'
    expect(response.status()).toBe(allowed ? 200 : 403)
    const cookies = await context.cookies()
    if (allowed) {
      await expect(page.getByTestId('user-menu-trigger')).toBeVisible()
      expect(cookies.find((cookie) => cookie.name === 'goloom_session')?.httpOnly).toBe(true)
      expect(cookies.find((cookie) => cookie.name === 'goloom_csrf')?.value).toBeTruthy()
    } else {
      expect(response.headers()['x-error-code']).toBe('api_token_session_requires_full_access')
      await expect(page.getByText(en.api.api_token_session_requires_full_access)).toBeVisible()
      expect(cookies.some((cookie) => cookie.name === 'goloom_session' || cookie.name === 'goloom_csrf')).toBe(false)
      await expect(page.getByLabel(/access token|administrator token/i)).toBeVisible()
    }
  })
}

test('restricted token login shows German explanation', async ({ browser, request, baseURL }) => {
  const created = await request.post('/v1/me/api-tokens', {
    headers: { Authorization: `Bearer ${e2eBootstrapToken()}` },
    data: { name: 'session-login-german', scopes: ['read'] },
  })
  expect(created.status()).toBe(201)
  const { token } = await created.json()
  const context = await browser.newContext({ locale: 'de-DE', baseURL })
  try {
    await context.addInitScript(() => {
      localStorage.setItem('goloom-ui-settings', JSON.stringify({ ui: { language: 'de' } }))
    })
    const page = await context.newPage()
    await page.goto('/?login=recovery')
    await page.getByLabel(/Zugriffstoken|Administrator-Token/i).fill(token)
    await page.getByRole('button', { name: de.ui.auth.signInWithToken }).click()
    await expect(page.getByText(de.api.api_token_session_requires_full_access)).toBeVisible()
    expect((await context.cookies()).some((cookie) => cookie.name === 'goloom_session')).toBe(false)
  } finally {
    await context.close()
  }
})
