import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect, type Page } from '@playwright/test'
import { signIn } from './helpers'
import { e2eBootstrapToken } from './constants'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const outputDir = path.join(__dirname, '..', '..', 'website', 'src', 'assets', 'screenshots')

const DEMO_TEAM = 'Solstice Roasters'

async function dismissStatusBanner(page: Page) {
  const banner = page.locator('.status-banner-panel')
  if (await banner.isVisible().catch(() => false)) {
    await banner.locator('button').click()
    await expect(banner).toBeHidden()
  }
}

async function shoot(page: Page, name: string) {
  await dismissStatusBanner(page)
  // Let charts and layout animations settle before capturing.
  await page.waitForTimeout(900)
  await page.locator('.app-shell').screenshot({ path: path.join(outputDir, `${name}.png`) })
}

test.describe('website screenshots', () => {
  test('capture landing page product views', async ({ page, request }) => {
    test.setTimeout(240_000)
    await page.setViewportSize({ width: 1440, height: 900 })

    // Populate the demo workspace (idempotent) so every view has real-looking data.
    const seedRes = await request.post('/v1/admin/e2e/demo-seed', {
      headers: { Authorization: `Bearer ${e2eBootstrapToken()}` },
    })
    expect(seedRes.ok(), `demo-seed failed: ${seedRes.status()} ${await seedRes.text()}`).toBe(true)

    await signIn(page)

    // Switch from the E2E seed team to the demo workspace.
    await page.locator('.sidebar-team-selector').click()
    await page.getByRole('menuitem', { name: DEMO_TEAM }).click()
    await expect(page.locator('.sidebar-team-selector')).toContainText(DEMO_TEAM, { timeout: 15_000 })

    // Dashboard
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible()
    await shoot(page, 'dashboard')

    // Content calendar (grid view, filled with the demo month)
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()
    await page.getByRole('button', { name: 'Grid', exact: true }).click()
    await expect(page.locator('.content-calendar__grid')).toBeVisible()
    await expect(page.locator('.content-calendar__grid')).toContainText('Open roastery')
    await shoot(page, 'content-calendar')

    // Composer with destinations selected and live previews
    await page.getByRole('button', { name: 'New Post' }).click()
    const composer = page.getByTestId('composer-view')
    await expect(composer).toBeVisible({ timeout: 15_000 })
    await expect(composer.getByTestId('composer-destination-toggle').first()).toBeVisible()
    const selectAll = composer.locator('.composer-destinations-bar__all')
    if ((await selectAll.textContent())?.match(/select all/i)) {
      await selectAll.click()
    }
    await composer.getByLabel(/title/i).fill('Open roastery day — save the date')
    await composer
      .getByLabel(/message.*all destinations/i)
      .fill(
        'On the 26th we open the roastery doors: tours every hour, cuppings all day, and the new harvest on the table. Free entry, bring friends! 🎉',
      )
    await shoot(page, 'composer')
    await page.getByRole('button', { name: 'Cancel' }).click()

    // Analytics with engagement history and follower trend
    await page.getByRole('button', { name: 'Analytics', exact: true }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Team Performance' })).toBeVisible({
      timeout: 15_000,
    })
    await shoot(page, 'analytics')

    // Review queue with pending automation drafts
    await page.getByTestId('nav-review-queue').click()
    await expect(page.getByTestId('review-queue')).toBeVisible()
    await expect(page.getByTestId('review-queue')).toContainText('Coffee news weekly')
    await shoot(page, 'review-queue')

    // Automation: recurring templates
    await page.getByRole('button', { name: 'Automation', exact: true }).click()
    await expect(page.getByTestId('automation-view')).toBeVisible()
    await expect(page.getByTestId('automation-view')).toContainText('Freshly roasted this week', {
      timeout: 15_000,
    })
    await shoot(page, 'automation')
  })
})
