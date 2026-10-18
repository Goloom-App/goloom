import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'

import { ensureE2EAccount, getFirstTeamId, signIn } from './helpers'

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

test('RSS AI preview shows per-account output without importing or creating posts', async ({ page, baseURL }) => {
  const teamID = await getFirstTeamId(baseURL!, e2eBootstrapToken())
  const accountID = await ensureE2EAccount(baseURL!, e2eBootstrapToken(), teamID)
  let previewRequest: Record<string, unknown> | null = null
  let previewStarts = 0
  let previewJobReads = 0
  const sideEffectRequests: string[] = []

  await page.route('**/v1/teams', async route => {
    const response = await route.fetch()
    const data = await response.json() as { items: Record<string, unknown>[] }
    await route.fulfill({ response, json: { ...data, items: data.items.map(item => ({ ...item, is_ai_enabled: true })) } })
  })
  await page.route(`**/v1/teams/${teamID}/rss-feeds`, route => route.fulfill({ json: { items: [{
    id: 'preview-feed', team_id: teamID, name: 'Preview feed', feed_url: 'https://example.org/feed',
    is_active: true, ai_enhance_enabled: true, prompt_hint: 'Focus on the main takeaway.',
    content_template: '{title}: {summary}', title_template: '{title}', output_mode: 'draft',
    target_account_ids: [accountID], created_at: '2026-01-01T00:00:00Z', last_fetched_at: null,
  }] } }))
  await page.route(`**/v1/teams/${teamID}/rss-feeds/preview-feed/preview-items`, route => route.fulfill({ json: { items: [{
    item_key: 'article-1', title: 'New release', content: 'Release notes say faster uploads.',
    link: 'https://example.org/release', published_at: '2026-10-17T10:00:00Z',
  }] } }))
  await page.route(`**/v1/teams/${teamID}/rss-feeds/preview-feed/ai-preview`, async route => {
    previewStarts += 1
    previewRequest = route.request().postDataJSON() as Record<string, unknown>
    await route.fulfill({ status: 202, json: { job_id: 'preview-job', status: 'pending' } })
  })
  await page.route(`**/v1/teams/${teamID}/ai-jobs/preview-job`, route => {
    previewJobReads += 1
    if (previewJobReads === 1) return route.fulfill({ status: 500, json: { error: 'temporary preview status failure' } })
    return route.fulfill({ json: {
    id: 'preview-job', team_id: teamID, author_user_id: 'user', type: 'voice_engine', status: 'completed',
    payload: {
      preview: {
        source: { title: 'New release', content: 'Release notes say faster uploads.', link: 'https://example.org/release' },
        template: 'New release: Release notes say faster uploads.',
        generation_inputs: { prompt_hint: 'Focus on the main takeaway.' },
      },
      params: { target_account_ids: [accountID] },
    },
    result: { content: 'Uploads now move much faster.', account_content_override: { [accountID]: 'Uploads now move much faster on Bluesky.' } },
    error_message: null, created_at: '2026-10-18T00:00:00Z', updated_at: '2026-10-18T00:00:00Z', completed_at: '2026-10-18T00:00:01Z',
    } })
  })
  await page.route(`**/v1/teams/${teamID}/posts**`, route => {
    if (route.request().method() !== 'POST') return route.continue()
    sideEffectRequests.push(`${route.request().method()} ${route.request().url()}`)
    return route.fulfill({ status: 500, json: { error: 'preview must not create posts' } })
  })
  await page.route(`**/v1/teams/${teamID}/rss-feeds/*/import**`, route => {
    if (route.request().method() !== 'POST') return route.continue()
    sideEffectRequests.push(`${route.request().method()} ${route.request().url()}`)
    return route.fulfill({ status: 500, json: { error: 'preview must not import feed items' } })
  })

  await signIn(page)
  await page.getByRole('button', { name: /^Automation$/i }).click()
  await page.getByTestId('automation-tab-rss').click()
  await page.getByRole('button', { name: /edit feed/i }).click()
  await page.getByTestId('rss-ai-prompt').fill('Changed preview prompt.')
  await page.getByTestId('rss-content-template').fill('Draft preview: {title}')
  await page.getByTestId('rss-preview-load').click()
  await page.getByTestId('rss-ai-preview').getByRole('button', { name: /generate preview|vorschau generieren/i }).click()
  await expect(page.getByRole('alert')).toBeVisible()
  await page.getByRole('dialog').getByRole('button', { name: /cancel|abbrechen/i }).click()
  await page.getByRole('button', { name: /edit feed/i }).click()
  await page.getByTestId('rss-ai-preview').getByRole('button', { name: /resume preview|vorschau fortsetzen/i }).click()
  await expect(page.getByTestId(`rss-ai-preview-account-${accountID}`)).toContainText('Uploads now move much faster on Bluesky.')
  await expect(page.getByTestId(`rss-ai-preview-account-${accountID}`)).toContainText(/\d+ characters|\d+ Zeichen/)
  expect(previewRequest).toMatchObject({
    item_key: 'article-1',
    config: {
      prompt_hint: 'Changed preview prompt.',
      content_template: 'Draft preview: {title}',
      target_account_ids: [accountID],
    },
  })
  expect(previewStarts).toBe(1)
  expect(previewJobReads).toBe(2)
  expect(sideEffectRequests).toEqual([])
})
