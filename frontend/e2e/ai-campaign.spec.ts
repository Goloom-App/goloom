import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { signIn, createAITeam } from './helpers'

test.describe.serial('AI campaign formats', () => {
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
    await page.getByRole('button', { name: 'Campaign Formats' }).click()
    await expect(page.getByTestId('campaign-format-view')).toBeVisible({ timeout: 10_000 })
  })

  test('renders empty state initially', async ({ page }) => {
    await expect(page.getByText('No campaign formats defined yet.')).toBeVisible()
    await expect(page.getByTestId('campaign-create-btn')).toBeVisible()
  })

  test('can create a campaign format', async ({ page }) => {
    await page.getByTestId('campaign-create-btn').click()
    await expect(page.getByTestId('campaign-dialog')).toBeVisible()

    await page.getByTestId('campaign-dialog-name').fill('E2E Test Tuesday')
    await page.getByTestId('campaign-dialog-weekday').selectOption('2')
    await page.getByTestId('campaign-dialog-structure').fill('{"topic": "test", "tone": "casual"}')
    await page.getByTestId('campaign-dialog-save').click()

    // Dialog should close and format should appear in the list
    await expect(page.getByTestId('campaign-dialog')).toHaveCount(0)
    await expect(page.getByText('E2E Test Tuesday')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('Tuesday', { exact: true })).toBeVisible()
  })

  test('shows validation error for invalid JSON structure', async ({ page }) => {
    await page.getByTestId('campaign-create-btn').click()
    await expect(page.getByTestId('campaign-dialog')).toBeVisible()

    await page.getByTestId('campaign-dialog-name').fill('Bad Format')
    await page.getByTestId('campaign-dialog-structure').fill('not valid json')
    await page.getByTestId('campaign-dialog-save').click()

    // Validation error should appear
    await expect(page.getByText('Structure must be valid JSON')).toBeVisible()
  })

  test('can create and then delete a format', async ({ page }) => {
    // First confirm existing format is visible
    await expect(page.getByText('E2E Test Tuesday')).toBeVisible({ timeout: 5_000 })

    // Create a second format
    await page.getByTestId('campaign-create-btn').click()
    await expect(page.getByTestId('campaign-dialog')).toBeVisible()
    await page.getByTestId('campaign-dialog-name').fill('Delete Me Format')
    await page.getByTestId('campaign-dialog-structure').fill('{"topic": "delete"}')

    // Wait for the create API response, then the list refetch
    const responsePromise = page.waitForResponse(resp =>
      resp.url().includes('/campaign-formats') && resp.status() === 201
    )
    await page.getByTestId('campaign-dialog-save').click()
    await responsePromise

    // Wait for dialog to close and list to update
    await expect(page.getByTestId('campaign-dialog')).toHaveCount(0, { timeout: 10_000 })

    // Both formats should now appear
    await expect(page.getByText('E2E Test Tuesday')).toBeVisible()
    const formatCard = page.getByText('Delete Me Format')
    await expect(formatCard).toBeVisible({ timeout: 10_000 })
  })
})
