import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam } from './helpers'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'

// Captures the new Team Settings -> Audit log section for visual review.
test('audit log section renders team activity', async ({ page }) => {
  test.setTimeout(90_000)
  const token = e2eBootstrapToken()

  // createAITeam creates a non-personal team and PATCHes it (a team.update
  // audit event). Add a post and a settings change so the table has rows.
  const teamId = await createAITeam(baseURL, token)
  await fetch(`${baseURL}/v1/teams/${teamId}/posts`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      title: 'Launch announcement',
      content: 'Hello world',
      scheduled_at: new Date(Date.now() + 86_400_000).toISOString(),
      target_accounts: [],
      draft: true,
    }),
  })
  await fetch(`${baseURL}/v1/teams/${teamId}`, {
    method: 'PATCH',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: 'Marketing Team', description: 'Q3 launch crew' }),
  })

  await page.setViewportSize({ width: 1280, height: 1100 })
  await page.goto('/')
  await page.getByLabel(/access token|administrator token/i).fill(token)
  await page.getByRole('button', { name: 'Sign in with token' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })
  await page.goto(`/?team=${teamId}`)
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 15_000 })

  await page.getByRole('button', { name: 'Team', exact: true }).click()

  const auditHeading = page.getByRole('heading', { name: 'Audit log' })
  await expect(auditHeading).toBeVisible({ timeout: 20_000 })
  await auditHeading.scrollIntoViewIfNeeded()

  const card = page.locator('section.brand-card').filter({ has: auditHeading })
  // At least one recorded action, attributed to the acting API key (tool).
  await expect(card.locator('tbody tr').first()).toBeVisible({ timeout: 15_000 })
  await expect(card.getByText(/API key:/).first()).toBeVisible()

  await card.screenshot({ path: 'test-results/audit-log.png' })
})
