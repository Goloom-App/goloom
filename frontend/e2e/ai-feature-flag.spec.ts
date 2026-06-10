import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam } from './helpers'

test.describe.serial('ai feature flag', () => {
  let teamId: string
  let bootstrapToken: string
  const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'

  test.beforeAll(async () => {
    bootstrapToken = e2eBootstrapToken()
    teamId = await createAITeam(baseURL, bootstrapToken)
  })

  test('AI navigation items are hidden when team has AI disabled', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })

    // Sign in — personal team has isAiEnabled=false
    await page.goto('/')
    await page.getByLabel(/access token|administrator token/i).fill(bootstrapToken)
    await page.getByRole('button', { name: 'Sign in with token' }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })

    // AI nav items should NOT appear for personal team
    await expect(page.getByRole('button', { name: 'AI Studio' })).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Campaign Formats' })).toHaveCount(0)

    // Non-AI nav items should still be visible
    await expect(page.getByRole('button', { name: 'Calendar', exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Home', exact: true })).toBeVisible()
  })

  test('AI navigation items are visible when team has AI enabled', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })

    await page.goto('/')
    await page.getByLabel(/access token|administrator token/i).fill(bootstrapToken)
    await page.getByRole('button', { name: 'Sign in with token' }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })

    await page.goto(`/?team=${teamId}`)
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })

    // AI nav items should now be visible
    await expect(page.getByRole('button', { name: 'AI Studio' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Campaign Formats' })).toBeVisible()
  })
})
