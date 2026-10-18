import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'

import { getFirstTeamId, signIn } from './helpers'

test.describe('rss feeds', () => {
  test('navigation opens RSS feeds view', async ({ page }) => {
    test.setTimeout(60_000)
    await signIn(page)
    await page.getByRole('button', { name: /^Automation$/i }).click()
    await page.getByTestId('automation-tab-rss').click()
    await expect(page.getByText(/automatically create posts/i)).toBeVisible()
    await expect(page.getByRole('button', { name: /add feed/i })).toBeVisible()
  })
})


test('shows exhausted RSS items from API', async ({ page, baseURL }) => {
  const teamID = await getFirstTeamId(baseURL!, e2eBootstrapToken())
  await page.route(`**/v1/teams/${teamID}/rss-feeds`, route => route.fulfill({ json: { items: [{
    id: 'exhausted-feed', team_id: teamID, name: 'Retry feed', feed_url: 'https://example.org/feed',
    is_active: true, target_account_ids: [], exhausted_items_count: 2,
    created_at: '2026-01-01T00:00:00Z', last_fetched_at: null,
  }] } }))
  await signIn(page)
  await page.getByRole('button', { name: /^Automation$/i }).click()
  await page.getByTestId('automation-tab-rss').click()
  await expect(page.getByTestId('rss-exhausted-items')).toHaveText('2 items exhausted automatic retries')
})

test('article enrichment is an optional feed setting', async ({ page }) => {
  await page.route('**/v1/teams', async route => {
    const response = await route.fetch()
    const data = await response.json() as { items: Record<string, unknown>[] }
    await route.fulfill({ response, json: { ...data, items: data.items.map(item => ({ ...item, is_ai_enabled: true })) } })
  })
  await signIn(page)
  await page.getByRole('button', { name: /^Automation$/i }).click()
  await page.getByTestId('automation-tab-rss').click()
  await page.getByRole('button', { name: /add feed/i }).click()
  await page.getByTestId('rss-ai-enhance').click()
  const articleToggle = page.getByTestId('rss-article-enrichment').locator('input')
  await expect(articleToggle).not.toBeChecked()
  await page.getByTestId('rss-article-enrichment').click()
  await expect(articleToggle).toBeChecked()
})
