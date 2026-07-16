import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam, signIn } from './helpers'

test.describe('app shell navigation', () => {
  test('sidebar collapses to icons and restores from localStorage', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })
    await signIn(page)

    const calendarLabel = page.getByRole('button', { name: 'Calendar', exact: true }).locator('.sidebar-nav-item__label')
    await expect(calendarLabel).toBeVisible()

    await page.getByTestId('sidebar-collapse-toggle').click()
    await expect(calendarLabel).toBeHidden()

    // Preference must survive a reload
    await page.reload()
    await expect(page.getByTestId('sidebar-collapse-toggle')).toBeVisible({ timeout: 30_000 })
    await expect(
      page.getByRole('button', { name: 'Calendar', exact: true }).locator('.sidebar-nav-item__label'),
    ).toBeHidden()

    await page.getByTestId('sidebar-collapse-toggle').click()
    await expect(
      page.getByRole('button', { name: 'Calendar', exact: true }).locator('.sidebar-nav-item__label'),
    ).toBeVisible()
  })

  test('user menu opens settings and admin entries', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })
    await signIn(page)

    await page.getByTestId('user-menu-trigger').click()
    await expect(page.getByRole('menuitem', { name: 'Settings' })).toBeVisible()
    await expect(page.getByRole('menuitem', { name: 'Sign out' })).toBeVisible()

    await page.getByRole('menuitem', { name: 'Settings' }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible()
  })

  test('sidebar footer shows the running version', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })
    await page.route('**/v1/version', (route) =>
      route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ current: 'v9.9.9', latest: 'v9.9.9', update_available: false }),
      }),
    )
    await signIn(page)

    const version = page.getByTestId('sidebar-version')
    await expect(version).toBeVisible({ timeout: 15_000 })
    await expect(version).toContainText('v9.9.9')
    // No update: it is plain text, not a link.
    await expect(version).not.toHaveAttribute('href', /.+/)
  })

  test('sidebar footer surfaces an available update as a release link', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })
    await page.route('**/v1/version', (route) =>
      route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ current: 'v0.1.0', latest: 'v9.9.9', update_available: true }),
      }),
    )
    await signIn(page)

    const version = page.getByTestId('sidebar-version')
    await expect(version).toBeVisible({ timeout: 15_000 })
    await expect(version).toContainText('Update available')
    await expect(version).toHaveAttribute(
      'href',
      'https://github.com/Goloom-App/goloom/releases/tag/v9.9.9',
    )
  })

  test('team brand color can be saved and applies the accent variable', async ({ page, baseURL }) => {
    test.setTimeout(60_000)
    const token = e2eBootstrapToken()
    if (!baseURL) {
      throw new Error('baseURL missing')
    }
    const teamId = await createAITeam(baseURL, token)

    await page.setViewportSize({ width: 1280, height: 720 })
    await signIn(page)
    await page.goto(`/?team=${teamId}`)
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 15_000 })

    await page.getByRole('button', { name: 'Team', exact: true }).click()
    const colorInput = page.getByTestId('team-brand-color-input')
    await expect(colorInput).toBeVisible({ timeout: 15_000 })

    await colorInput.fill('#2563eb')
    await page.getByTestId('team-brand-color-save').click()

    await expect
      .poll(
        () =>
          page.evaluate(() =>
            document.documentElement.style.getPropertyValue('--brand-primary').trim(),
          ),
        { timeout: 15_000 },
      )
      .toBe('#2563eb')

    // Reset so other tests see the default accent again
    await page.getByRole('button', { name: 'Reset to default' }).click()
    await expect
      .poll(
        () =>
          page.evaluate(() =>
            document.documentElement.style.getPropertyValue('--brand-primary').trim(),
          ),
        { timeout: 15_000 },
      )
      .toBe('')
  })
})
