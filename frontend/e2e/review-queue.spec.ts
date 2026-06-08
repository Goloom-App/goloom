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
    await expect(page.getByTestId('review-queue-empty')).toBeVisible({ timeout: 15_000 })
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
