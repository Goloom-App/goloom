import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

// The e2e server runs with APP_ENV=development and the bootstrap admin, so the
// dev/admin-only "Browser session" panel is visible and the current token is
// marked. (In production the panel is hidden — gated in App.tsx.)
test('settings shows the dev browser-session panel and marks the current token', async ({ page }) => {
  test.setTimeout(60_000)
  await page.setViewportSize({ width: 1280, height: 900 })
  await signIn(page)

  await page.getByTestId('user-menu-trigger').click()
  await page.getByRole('menuitem', { name: 'Settings' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible()

  // dev + admin → backend-override panel present.
  await expect(page.getByText('Browser session')).toBeVisible()

  // The current browser session (cookie-based __web_session) is flagged.
  const tokenRow = page.locator('.data-table tr', { hasText: 'Web session' })
  await expect(tokenRow.getByText('this browser')).toBeVisible({ timeout: 15_000 })
})
