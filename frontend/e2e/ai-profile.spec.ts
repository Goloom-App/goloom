import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { signIn, createAITeam } from './helpers'

test.describe.serial('AI team profile', () => {
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
    await page.getByRole('button', { name: 'AI Profile' }).click()
    await expect(page.getByTestId('team-profile-view')).toBeVisible({ timeout: 10_000 })
  })

  test('renders profile form with all fields', async ({ page }) => {
    await expect(page.getByTestId('profile-tonality')).toBeVisible()
    await expect(page.getByTestId('profile-language')).toBeVisible()
    await expect(page.getByTestId('profile-max-hashtags')).toBeVisible()
    await expect(page.getByTestId('profile-auto-publish')).toBeVisible()
    await expect(page.getByTestId('profile-save')).toBeVisible()
  })

  test('can update tonality and save profile', async ({ page }) => {
    await page.getByTestId('profile-tonality').fill('Professional, witty, concise')
    await page.getByTestId('profile-max-hashtags').fill('5')
    await page.getByTestId('profile-language').selectOption('de')
    await page.getByTestId('profile-save').click()

    // Success message should appear
    await expect(page.getByTestId('profile-status-success')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('profile-status-success')).toHaveText('Profile saved successfully')
  })

  test('can toggle auto-publish', async ({ page }) => {
    const checkbox = page.getByTestId('profile-auto-publish')
    await expect(checkbox).not.toBeChecked()
    await checkbox.check()
    await expect(checkbox).toBeChecked()
    await page.getByTestId('profile-save').click()
    await expect(page.getByTestId('profile-status-success')).toBeVisible({ timeout: 10_000 })
  })

  test('can add a style example', async ({ page }) => {
    await page.getByTestId('profile-add-example').click()
    await expect(page.getByTestId('example-dialog')).toBeVisible()

    await page.getByTestId('example-dialog-platform').selectOption('bluesky')
    await page.getByTestId('example-dialog-content').fill('This is an E2E test style example for Bluesky.')
    await page.getByTestId('example-dialog-notes').fill('Testing the style example flow')
    await page.getByTestId('example-dialog-submit').click()

    // Dialog should close and success message should appear
    await expect(page.getByTestId('example-dialog')).toHaveCount(0)
    await expect(page.getByTestId('profile-status-success')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('profile-status-success')).toHaveText('Style example added')
  })
})
