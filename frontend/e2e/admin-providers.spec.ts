import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

// Issue #98: the admin Providers tab adapts its texts and advanced config to the
// selected provider, and shows the OAuth auto-discover note only for OAuth providers.
test('admin providers tab adapts to the selected provider', async ({ page }) => {
  test.setTimeout(60_000)
  await page.setViewportSize({ width: 1280, height: 900 })

  await signIn(page)

  await page.getByTestId('user-menu-trigger').click()
  await page.getByRole('menuitem', { name: 'Admin' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Administration' })).toBeVisible({ timeout: 15_000 })

  await page.getByRole('tab', { name: 'Providers' }).click()
  await expect(page.getByRole('heading', { name: 'Provider onboarding' })).toBeVisible()

  const providerSelect = page.locator('.admin-provider-form__provider select')
  const callout = page.locator('.admin-callout--info')

  // Mastodon (default) is OAuth-capable → the highlighted auto-discover note shows.
  await providerSelect.selectOption('mastodon')
  await expect(callout).toBeVisible()
  await expect(callout).toContainText('Mastodon')

  await page.locator('.admin-provider-form__advanced summary').click()
  await expect(page.getByLabel('Client ID')).toBeVisible()

  await page.screenshot({ path: 'test-results/admin-providers-mastodon.png' })

  // Bluesky uses per-user app passwords → no OAuth note, no client-id field.
  await providerSelect.selectOption('bluesky')
  await expect(callout).toHaveCount(0)
  await expect(page.getByLabel('Client ID')).toHaveCount(0)
  await expect(page.locator('.admin-provider-form__advanced')).toContainText('no extra configuration')
})
