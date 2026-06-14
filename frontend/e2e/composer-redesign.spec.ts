import { test, expect } from '@playwright/test'
import { openContentCalendar, signIn } from './helpers'

// Visual + smoke coverage for the redesigned composer: the destinations top bar
// (replacing the old 260px sidebar) and the unified media grid both render.
test.describe('composer redesign', () => {
  test('desktop: destinations top bar and media grid render', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 900 })

    await signIn(page)
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()

    await page.getByRole('button', { name: 'New Post' }).click()
    const composer = page.getByTestId('composer-view')
    await expect(composer).toBeVisible({ timeout: 10_000 })

    // Destinations now live in a top bar inside the editor column (not a side column).
    await expect(composer.locator('.composer-destinations-bar')).toBeVisible()

    // The media area is a single unified grid with an "add" tile.
    await expect(composer.locator('.composer-media__grid')).toBeVisible()
    await expect(composer.locator('.composer-media__add')).toBeVisible()

    await composer.screenshot({ path: 'test-results/composer-redesign.png' })
  })

  test('mobile: compact header toggle and slim destination strip', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 390, height: 844 })

    await signIn(page)
    await openContentCalendar(page)

    await page.getByRole('button', { name: 'Post', exact: true }).click()
    const overlay = page.getByTestId('composer-overlay')
    await expect(overlay).toBeVisible({ timeout: 10_000 })

    // Edit/preview is now a single icon toggle in the header (no full-width tab row).
    const toggle = overlay.locator('.composer-mobile-toggle')
    await expect(toggle).toBeVisible()
    await expect(overlay.locator('.composer-mobile-tabs')).toHaveCount(0)

    // Destinations render as the slim icon strip, not the bordered desktop bar.
    await expect(overlay.locator('.composer-destination-strip')).toBeVisible()
    await expect(overlay.locator('.composer-destinations-bar')).toHaveCount(0)

    await overlay.screenshot({ path: 'test-results/composer-mobile-edit.png' })

    // Toggling switches to the preview panel and flips the icon to a pencil (edit).
    await toggle.click()
    await expect(overlay.locator('.composer-previews-mobile')).toBeVisible()
    await overlay.screenshot({ path: 'test-results/composer-mobile-preview.png' })
  })
})
