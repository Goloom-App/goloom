import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

// Bugs #100 / #101: the dashboard performance sparklines (incl. "Network Trend")
// render and survive a refresh without swapping back to the loading placeholder.
test('dashboard sparklines render and a refresh keeps the charts mounted', async ({ page }) => {
  test.setTimeout(60_000)
  await page.setViewportSize({ width: 1280, height: 1000 })

  await signIn(page)
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 15_000 })

  const network = page.locator('.dashboard-spark').filter({ hasText: 'Network Trend' })
  await expect(network).toBeVisible()
  // The chart (or its empty state) is mounted, not stuck on the loading placeholder.
  await expect(network.locator('svg, .dashboard-spark__placeholder')).toBeVisible()

  await page.screenshot({ path: 'test-results/dashboard-charts.png' })

  // Refreshing must not flip the already-loaded charts back to "Loading…".
  await page.getByRole('button', { name: 'Refresh' }).click()
  await expect(network).toBeVisible()
  await expect(network).not.toContainText('Loading')
})
