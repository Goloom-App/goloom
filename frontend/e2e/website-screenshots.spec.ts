import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const outputDir = path.join(__dirname, '..', '..', 'website', 'src', 'assets', 'screenshots')

test.describe('website screenshots', () => {
  test('capture landing page product views', async ({ page }) => {
    test.setTimeout(120_000)
    await page.setViewportSize({ width: 1440, height: 900 })

    await signIn(page)

    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()
    await page.getByRole('button', { name: 'Grid', exact: true }).click()
    await expect(page.locator('.content-calendar__grid')).toBeVisible()
    await page.locator('.app-shell').screenshot({
      path: path.join(outputDir, 'content-calendar.png'),
    })

    await page.getByRole('button', { name: 'New Post' }).click()
    await expect(page.getByTestId('composer-view')).toBeVisible({ timeout: 15_000 })
    await page.locator('.app-shell').screenshot({
      path: path.join(outputDir, 'composer.png'),
    })

    await page.getByRole('button', { name: 'Cancel' }).click()
    await page.getByRole('button', { name: 'Analytics', exact: true }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Team Performance' })).toBeVisible({
      timeout: 15_000,
    })
    await page.locator('.app-shell').screenshot({
      path: path.join(outputDir, 'analytics.png'),
    })
  })
})
