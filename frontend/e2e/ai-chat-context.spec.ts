import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam } from './helpers'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'

// Issue #96: the AI chat picks up the open composer as context, and @-mentions can
// be completed from the keyboard (no mouse required).
test('AI chat attaches composer context and completes @-mentions via the keyboard', async ({ page }) => {
  test.setTimeout(90_000)
  const token = e2eBootstrapToken()
  const teamId = await createAITeam(baseURL, token)

  // Seed a campaign format so the @-mention dropdown has an entry to complete.
  const seed = await fetch(`${baseURL}/v1/teams/${teamId}/campaign-formats`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: 'LaunchPromo',
      weekday: null,
      structure: {},
      required_hashtags: [],
      is_active: true,
    }),
  })
  expect(seed.ok).toBeTruthy()

  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')
  await page.getByLabel(/access token|administrator token/i).fill(token)
  await page.getByRole('button', { name: 'Sign in with token' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })
  await page.goto(`/?team=${teamId}`)
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 15_000 })

  // Open the composer and write something so there is context to attach.
  await page.getByRole('button', { name: 'Calendar', exact: true }).click()
  await page.getByRole('button', { name: 'New Post' }).click()
  await expect(page.getByTestId('composer-view')).toBeVisible({ timeout: 10_000 })
  await page
    .getByTestId('composer-view')
    .getByLabel(/message.*all destinations|nachricht.*alle ziele/i)
    .fill('Launch day is here!')

  // Open the chat — the composer draft is attached automatically.
  await page.getByTestId('ai-chat-fab').click()
  const drawer = page.locator('.ai-chat-drawer')
  await expect(drawer).toBeVisible()
  await expect(drawer.locator('.ai-chat-chip--context')).toBeVisible()

  // Keyboard completion: type "@", the dropdown opens, Enter inserts the highlighted entry.
  const chatInput = drawer.locator('textarea')
  await chatInput.click()
  await chatInput.type('@')
  await expect(drawer.locator('.ai-chat-suggestions')).toBeVisible()
  await expect(drawer.locator('.ai-chat-suggestions__active')).toContainText('LaunchPromo')
  await drawer.screenshot({ path: 'test-results/ai-chat-context.png' })
  await chatInput.press('Enter')
  await expect(chatInput).toHaveValue(/@LaunchPromo\s/)
  // The dropdown closes after completion (no accidental send).
  await expect(drawer.locator('.ai-chat-suggestions')).toHaveCount(0)
})
