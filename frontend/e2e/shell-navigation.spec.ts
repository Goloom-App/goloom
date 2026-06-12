import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

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

  test('team brand color can be saved and applies the accent variable', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })
    await signIn(page)

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
