import { test, expect } from '@playwright/test'

import { e2eBootstrapToken, E2E_REVIEW_POST_TITLE } from './constants'
import { getFirstTeamId, openReviewQueue, seedAutomationReviewDraft, signIn } from './helpers'

test.describe('review queue', () => {
  test('shows automation draft and can discard it', async ({ page, baseURL }) => {
    test.setTimeout(60_000)
    const token = e2eBootstrapToken()
    if (!baseURL) {
      throw new Error('baseURL missing')
    }
    const teamId = await getFirstTeamId(baseURL, token)
    await seedAutomationReviewDraft(baseURL, token, teamId)

    await signIn(page)
    await openReviewQueue(page)

    const item = page.getByTestId('review-queue-item').filter({ hasText: E2E_REVIEW_POST_TITLE })
    await expect(item).toBeVisible({ timeout: 15_000 })
    await expect(item.getByTestId('review-overdue-badge')).toBeVisible()

    page.once('dialog', (dialog) => dialog.accept())
    await item.getByTestId('review-discard').click()
    // Other specs seed their own uniquely-titled drafts into the same queue,
    // so assert this item disappeared rather than the whole queue being empty.
    await expect(item).toHaveCount(0, { timeout: 15_000 })
  })

  test('edit works for a draft that arrived after the dashboard load', async ({ page, baseURL }) => {
    // Automation drafts routinely land while the app is already open: the
    // queue polls every 30s, but the posts cache stays stale. Edit must not
    // silently no-op for such drafts.
    test.setTimeout(90_000)
    const token = e2eBootstrapToken()
    if (!baseURL) {
      throw new Error('baseURL missing')
    }
    const teamId = await getFirstTeamId(baseURL, token)
    const title = `E2E late draft ${Date.now()}`

    await signIn(page)
    await openReviewQueue(page)
    await seedAutomationReviewDraft(baseURL, token, teamId, title, 'Arrived after the dashboard load.')
    // Nudge react-query's focus refetch instead of waiting out the 30s poll.
    await page.evaluate(() => {
      window.dispatchEvent(new Event('focus'))
    })

    const item = page.getByTestId('review-queue-item').filter({ hasText: title })
    await expect(item).toBeVisible({ timeout: 45_000 })
    await item.getByTestId('review-edit').click()
    await expect(page.getByTestId('composer-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByLabel(/title/i)).toHaveValue(title)
  })

  test('selecting a card shows the full text in the preview sidebar', async ({ page, baseURL }) => {
    test.setTimeout(60_000)
    const token = e2eBootstrapToken()
    if (!baseURL) {
      throw new Error('baseURL missing')
    }
    const teamId = await getFirstTeamId(baseURL, token)
    const title = `E2E preview draft ${Date.now()}`
    const longContent = 'A very long automation draft body that the compact card clamps. '.repeat(8).trim()
    await seedAutomationReviewDraft(baseURL, token, teamId, title, longContent)

    await signIn(page)
    await openReviewQueue(page)

    const item = page.getByTestId('review-queue-item').filter({ hasText: title })
    await expect(item).toBeVisible({ timeout: 15_000 })
    await item.getByTestId('review-open-preview').click()
    await expect(page.getByTestId('live-preview-title')).toHaveText(title)
    await expect(page.locator('.preview-content')).toContainText(longContent)
  })

  test('edit opens composer with review draft', async ({ page, baseURL }) => {
    test.setTimeout(60_000)
    const token = e2eBootstrapToken()
    if (!baseURL) {
      throw new Error('baseURL missing')
    }
    const teamId = await getFirstTeamId(baseURL, token)
    await seedAutomationReviewDraft(baseURL, token, teamId)

    await signIn(page)
    await openReviewQueue(page)

    const item = page.getByTestId('review-queue-item').filter({ hasText: E2E_REVIEW_POST_TITLE })
    await expect(item).toBeVisible({ timeout: 15_000 })
    await item.getByTestId('review-edit').click()
    await expect(page.getByTestId('composer-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('composer-title')).toHaveText('Edit post')
    await expect(page.getByLabel(/title/i)).toHaveValue(E2E_REVIEW_POST_TITLE)
  })
})
