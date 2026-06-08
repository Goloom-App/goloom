import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam } from './helpers'

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

  test('renders generate form by default', async ({ page }) => {
    await expect(page.getByTestId('gen-prompt')).toBeVisible()
    await expect(page.getByTestId('gen-campaign')).toBeVisible()
    await expect(page.getByTestId('gen-submit')).toBeVisible()
    await expect(page.getByText('Target accounts')).toBeVisible()
  })

  test('shows validation error when triggering without prompt', async ({ page }) => {
    await page.getByTestId('gen-submit').click()

    await expect(page.getByTestId('gen-status-error')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('gen-status-error')).toHaveText('Prompt is required')
  })

  test('shows validation error when no target accounts selected', async ({ page }) => {
    await page.getByTestId('gen-prompt').fill('E2E test prompt for generation')
    await page.getByTestId('gen-submit').click()

    await expect(page.getByTestId('gen-status-error')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('gen-status-error')).toHaveText('Select at least one target account')
  })

  test('renders empty generated content placeholder initially', async ({ page }) => {
    await expect(page.getByText('Generated posts will appear here.')).toBeVisible()
  })
})
