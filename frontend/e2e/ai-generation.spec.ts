import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { signIn, createAITeam } from './helpers'

test.describe.serial('AI generation view', () => {
  let teamId: string
  let bootstrapToken: string
  const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'

  test.beforeAll(async () => {
    bootstrapToken = e2eBootstrapToken()
    teamId = await createAITeam(baseURL, bootstrapToken)
  })

  test.beforeEach(async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })
    await page.goto('/')
    await page.getByLabel(/access token|administrator token/i).fill(bootstrapToken)
    await page.getByRole('button', { name: 'Sign in with token' }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })
    await page.goto(`/?team=${teamId}`)
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 15_000 })
    await page.getByRole('button', { name: 'Generate Post' }).click()
    await expect(page.getByTestId('ai-generate-view')).toBeVisible({ timeout: 10_000 })
  })

  test('renders Voice Engine mode by default', async ({ page }) => {
    // Voice Engine should be the active/default mode
    await expect(page.getByTestId('gen-type-voice')).toBeVisible()
    await expect(page.getByTestId('gen-type-campaign')).toBeVisible()

    // Voice Engine fields should be visible
    await expect(page.getByTestId('gen-platform')).toBeVisible()
    await expect(page.getByTestId('gen-prompt')).toBeVisible()
    await expect(page.getByTestId('gen-submit')).toBeVisible()
  })

  test('can switch to Campaign Auto-Pilot mode', async ({ page }) => {
    await page.getByTestId('gen-type-campaign').click()

    // Campaign fields should be visible
    await expect(page.getByTestId('gen-campaign-format')).toBeVisible()
    await expect(page.getByTestId('gen-target-date')).toBeVisible()

    // The submit button should still be visible
    await expect(page.getByTestId('gen-submit')).toBeVisible()
  })

  test('shows validation error when triggering without prompt hint', async ({ page }) => {
    // Leave prompt empty and try to generate
    await page.getByTestId('gen-submit').click()

    // Should show validation error
    await expect(page.getByTestId('gen-status-error')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('gen-status-error')).toHaveText('Prompt hint is required')
  })

  test('shows validation error when triggering campaign without format', async ({ page }) => {
    await page.getByTestId('gen-type-campaign').click()
    await page.getByTestId('gen-submit').click()

    // Should show validation error for missing campaign format
    await expect(page.getByTestId('gen-status-error')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('gen-status-error')).toHaveText('Select a campaign format')
  })

  test('renders recent jobs section as empty initially', async ({ page }) => {
    await expect(page.getByTestId('gen-recent-jobs')).toBeVisible()
    await expect(page.getByText('No recent AI jobs.')).toBeVisible()
  })
})
