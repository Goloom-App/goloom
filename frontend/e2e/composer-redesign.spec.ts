import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

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
})
