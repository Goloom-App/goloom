import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam } from './helpers'

test.describe.serial('AI studio brand wizard', () => {
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
    await page.getByRole('button', { name: 'AI Studio' }).click()
    await expect(page.getByTestId('brand-wizard-view')).toBeVisible({ timeout: 10_000 })
  })

  test('renders setup step with brand dimension fields', async ({ page }) => {
    await expect(page.getByTestId('brand-industry')).toBeVisible()
    await expect(page.getByTestId('brand-main-value')).toBeVisible()
    await expect(page.getByTestId('brand-audience')).toBeVisible()
    await expect(page.getByTestId('brand-sentence-style')).toBeVisible()
    await expect(page.getByTestId('brand-humor')).toBeVisible()
    await expect(page.getByTestId('brand-hook')).toBeVisible()
    await expect(page.getByTestId('brand-cta')).toBeVisible()
    await expect(page.getByTestId('brand-knowledge-section')).toBeVisible()
    await expect(page.getByTestId('brand-save-setup')).toBeVisible()
  })

  test('can save brand profile setup', async ({ page }) => {
    await page.getByTestId('brand-industry').fill('Open Source Hosting')
    await page.getByTestId('brand-main-value').fill('Reliable infrastructure without vendor lock-in')
    await page.getByTestId('brand-audience').fill('Tech-savvy homelab community')
    await page.getByTestId('brand-sentence-style').selectOption('short_punchy')
    await page.getByTestId('brand-humor').selectOption('dry_sarcastic')
    await page.getByTestId('brand-hook').selectOption('ask_question')
    await page.getByTestId('brand-cta').selectOption('community_discussion')
    await page.getByTestId('brand-save-setup').click()

    await expect(page.getByTestId('brand-status-success')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('brand-status-success')).toContainText('Profil gespeichert')
  })

  test('can add a knowledge source', async ({ page }) => {
    await page.getByTestId('brand-knowledge-section').getByPlaceholder('z. B. Produkt-FAQ').fill('E2E Product FAQ')
    await page
      .getByTestId('brand-knowledge-section')
      .getByPlaceholder('Fakten, Zitate, Produktinfos…')
      .fill('Our product supports Mastodon, Bluesky, and Friendica scheduling.')
    await page.getByTestId('brand-knowledge-section').getByRole('button', { name: 'Wissensquelle hinzufügen' }).click()

    await expect(page.getByTestId('brand-status-success')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('E2E Product FAQ')).toBeVisible()
  })

  test('task step validates occasion before generation', async ({ page }) => {
    await page.getByRole('button', { name: '2. Aufgabe' }).click()
    await expect(page.getByTestId('brand-occasion')).toBeVisible()
    await expect(page.getByTestId('brand-output-format')).toBeVisible()
    await page.getByTestId('brand-generate').click()

    await expect(page.getByTestId('brand-status-error')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('brand-status-error')).toHaveText('Bitte einen Anlass angeben')
  })

  test('editor step shows placeholder without generated content', async ({ page }) => {
    await page.getByRole('button', { name: '3. Editor' }).click()
    await expect(page.getByText('Noch kein generierter Post.')).toBeVisible()
    await page.getByRole('button', { name: 'Zur Aufgabe' }).click()
    await expect(page.getByTestId('brand-occasion')).toBeVisible()
  })
})
