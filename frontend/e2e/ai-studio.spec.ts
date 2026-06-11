import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam } from './helpers'

test.describe.serial('AI profile and generator', () => {
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
  })

  test('AI Profile renders brand dimension fields', async ({ page }) => {
    await page.getByRole('button', { name: 'AI Profile', exact: true }).click()
    await expect(page.getByTestId('brand-profile-view')).toBeVisible({ timeout: 10_000 })

    await expect(page.getByTestId('brand-assistant-panel')).toBeVisible()
    await expect(page.getByTestId('brand-archetype')).toBeVisible()
    await expect(page.getByTestId('brand-persona')).toBeVisible()
    await expect(page.getByTestId('brand-industry')).toBeVisible()
    await expect(page.getByTestId('brand-main-value')).toBeVisible()
    await expect(page.getByTestId('brand-audience')).toBeVisible()
    await expect(page.getByTestId('brand-sentence-style')).toBeVisible()
    await expect(page.getByTestId('brand-humor')).toBeVisible()
    await expect(page.getByTestId('brand-hook')).toBeVisible()
    await expect(page.getByTestId('brand-cta')).toBeVisible()
    await expect(page.getByTestId('brand-anti-ai-override')).toBeVisible()
    await expect(page.getByTestId('brand-formatting-rule')).toBeVisible()
    await expect(page.getByTestId('brand-profile-tab-profile')).toBeVisible()
    await expect(page.getByTestId('brand-profile-tab-examples')).toBeVisible()
    await expect(page.getByTestId('brand-knowledge-section')).toBeVisible()
    await expect(page.getByTestId('brand-save-setup')).toBeVisible()
    await expect(page.getByTestId('brand-show-prompt')).toBeVisible()
  })

  test('can save brand profile with free-text dimensions', async ({ page }) => {
    await page.getByRole('button', { name: 'AI Profile', exact: true }).click()
    await expect(page.getByTestId('brand-profile-view')).toBeVisible({ timeout: 10_000 })

    await page.getByTestId('brand-archetype').fill('Selfhosting Podcast')
    await page.getByTestId('brand-persona').fill('Maximilian, 38, redet wie mit Kollegen am Stehtisch.')
    await page.getByTestId('brand-industry').fill('Open Source Hosting')
    await page.getByTestId('brand-main-value').fill('Heimserver ohne Vendor Lock-in')
    await page.getByTestId('brand-audience').fill('Hobby-Sysadmins über 30')
    await page.getByTestId('brand-sentence-style').fill('Kurze Sätze, gerne Halbsätze.')
    await page.getByTestId('brand-humor').fill('Trocken mit IT-Insider-Witzen.')
    await page.getByTestId('brand-hook').fill('Mit einer konkreten Beobachtung einsteigen.')
    await page.getByTestId('brand-cta').fill('Zum Kommentar einladen.')
    await page.getByTestId('brand-save-setup').click()

    await expect(page.getByTestId('brand-status-success')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('brand-status-success')).toContainText('Profil gespeichert')
  })

  test('AI profile assistant validates empty input', async ({ page }) => {
    await page.getByRole('button', { name: 'AI Profile', exact: true }).click()
    await expect(page.getByTestId('brand-profile-view')).toBeVisible({ timeout: 10_000 })

    await page.getByTestId('brand-assistant-toggle').click()
    await expect(page.getByTestId('brand-assistant-brief')).toBeVisible()
    await page.getByTestId('brand-assistant-submit').click()
    await expect(page.getByTestId('brand-status-error')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('brand-status-error')).toContainText('Bitte beschreibe')
  })

  test('can save formatting rules and manage style examples', async ({ page }) => {
    await page.getByRole('button', { name: 'AI Profile', exact: true }).click()
    await expect(page.getByTestId('brand-profile-view')).toBeVisible({ timeout: 10_000 })

    await page.getByTestId('brand-formatting-rule').fill('Max. 2 Emojis pro Post')
    await page.getByTestId('brand-formatting-rule').press('Enter')
    await page.getByTestId('brand-save-setup').click()
    await expect(page.getByTestId('brand-status-success')).toBeVisible({ timeout: 10_000 })

    await page.getByTestId('brand-profile-tab-examples').click()
    await expect(page.getByTestId('brand-examples-section')).toBeVisible()
    await page.getByTestId('brand-add-example').click()
    await expect(page.getByTestId('brand-example-dialog')).toBeVisible()
    await page.getByTestId('brand-example-content').fill('E2E example post with #hashtag and a clear CTA.')
    await page.getByTestId('brand-example-notes').fill('Typischer Ankündigungs-Post')
    await page.getByTestId('brand-example-submit').click()

    await expect(page.getByTestId('brand-status-success')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('E2E example post with #hashtag and a clear CTA.')).toBeVisible()
  })

  test('can add a knowledge source', async ({ page }) => {
    await page.getByRole('button', { name: 'AI Profile', exact: true }).click()
    await expect(page.getByTestId('brand-profile-view')).toBeVisible({ timeout: 10_000 })

    await page.getByTestId('brand-knowledge-section').getByPlaceholder('z. B. Produkt-FAQ').fill('E2E Product FAQ')
    await page
      .getByTestId('brand-knowledge-section')
      .getByPlaceholder('Fakten, Zitate, Produktinfos…')
      .fill('Our product supports Mastodon, Bluesky, and Friendica scheduling.')
    await page.getByTestId('brand-knowledge-section').getByRole('button', { name: 'Wissensquelle hinzufügen' }).click()

    await expect(page.getByTestId('brand-status-success')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('E2E Product FAQ')).toBeVisible()
  })

  test('AI Generator validates occasion before generation', async ({ page }) => {
    await page.getByRole('button', { name: 'AI Generator', exact: true }).click()
    await expect(page.getByTestId('ai-generator-view')).toBeVisible({ timeout: 10_000 })

    await expect(page.getByTestId('gen-occasion')).toBeVisible()
    await expect(page.getByTestId('gen-output-format-post')).toBeVisible()
    await page.getByTestId('gen-generate').click()

    await expect(page.getByTestId('gen-status-error')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('gen-status-error')).toHaveText('Bitte einen Anlass angeben')
  })

  test('AI Generator shows placeholder without generated content', async ({ page }) => {
    await page.getByRole('button', { name: 'AI Generator', exact: true }).click()
    await expect(page.getByTestId('ai-generator-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('Noch kein Post generiert')).toBeVisible()
  })
})
