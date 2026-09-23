import { test, expect } from '@playwright/test'

import { e2eBootstrapToken, E2E_REVIEW_POST_TITLE } from './constants'
import { ensureE2EAccount, getFirstTeamId, getPost, getReviewQueueItemId, listReviewQueue, openReviewQueue, patchPostContent, seedAutomationReviewDraft, signIn } from './helpers'

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

  test('publish-now does not overwrite content edited after the queue was loaded', async ({ page, baseURL }) => {
    // Regression: the queue action must not resend the stale item payload
    // (title/content/targets), otherwise it bulldozes edits persisted meanwhile.
    test.setTimeout(60_000)
    const token = e2eBootstrapToken()
    if (!baseURL) {
      throw new Error('baseURL missing')
    }
    const teamId = await getFirstTeamId(baseURL, token)
    const title = `E2E stale payload ${Date.now()}`
    const edited = `Edited after queue load ${Date.now()}`
    await seedAutomationReviewDraft(baseURL, token, teamId, title, 'Original automation content.')

    await signIn(page)
    await openReviewQueue(page)
    const item = page.getByTestId('review-queue-item').filter({ hasText: title })
    await expect(item).toBeVisible({ timeout: 15_000 })

    // Persist newer content behind the queue page's back: the page's cached
    // item object still carries the original content.
    const postId = await getReviewQueueItemId(baseURL, token, teamId, title)
    await patchPostContent(baseURL, token, teamId, postId, edited)

    const updateReq = page.waitForRequest((req) => req.method() === 'PATCH' && req.url().includes(`/posts/${postId}`))
    await item.getByTestId('review-publish-now').click()

    const bodyText = (await updateReq).postData() ?? ''
    expect(bodyText).toContain('"publish_now":true')
    expect(bodyText).not.toContain('"content"')
    expect(bodyText).not.toContain('"title"')
    expect(bodyText).not.toContain('"target_accounts"')

    const post = await getPost(baseURL, token, teamId, postId)
    expect(post.content).toBe(edited)
  })

  test('composer edit scheduled to the future removes the item from the queue', async ({ page, baseURL }) => {
    // Required flow: a review draft edited in the composer and given a future
    // time completes the review in one save — the post leaves the queue and
    // the edited content is persisted, not discarded.
    test.setTimeout(90_000)
    const token = e2eBootstrapToken()
    if (!baseURL) {
      throw new Error('baseURL missing')
    }
    const teamId = await getFirstTeamId(baseURL, token)
    const accountId = await ensureE2EAccount(baseURL, token, teamId)
    const title = `E2E schedule flow ${Date.now()}`
    const editedTitle = `E2E schedule flow edited ${Date.now()}`
    await seedAutomationReviewDraft(baseURL, token, teamId, title, 'Draft body for the schedule flow.', [accountId])

    await signIn(page)
    await openReviewQueue(page)
    const item = page.getByTestId('review-queue-item').filter({ hasText: title })
    await expect(item).toBeVisible({ timeout: 15_000 })

    await item.getByTestId('review-edit').click()
    await expect(page.getByTestId('composer-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByLabel(/title/i)).toHaveValue(title)

    const editedAt = new Date(Date.now() + 3 * 24 * 60 * 60 * 1000)
    const pad = (n: number) => String(n).padStart(2, '0')
    const futureInput = `${editedAt.getFullYear()}-${pad(editedAt.getMonth() + 1)}-${pad(editedAt.getDate())}T${pad(editedAt.getHours())}:${pad(editedAt.getMinutes())}`

    await page.getByLabel(/title/i).fill(editedTitle)
    await page.getByTestId('composer-view').locator('input[type="datetime-local"]').fill(futureInput)

    const updateRes = page.waitForResponse((res) => res.request().method() === 'PATCH' && res.url().includes('/posts/'))
    await page.getByRole('button', { name: /save changes/i }).click()
    const update = await updateRes
    const postId = update.url().split('/').pop() ?? ''

    const post = await getPost(baseURL, token, teamId, postId)
    expect(post.status).toBe('pending')
    expect(post.title).toBe(editedTitle)
    expect(post.content).toBe('Draft body for the schedule flow.')

    const queue = await listReviewQueue(baseURL, token, teamId)
    expect(queue.some((it) => it.id === postId)).toBe(false)
    await expect(item).toHaveCount(0, { timeout: 20_000 })
  })

  test('past-time schedule shows offer and publish-now sends a minimal patch', async ({ page, baseURL }) => {
    test.setTimeout(60_000)
    const token = e2eBootstrapToken()
    if (!baseURL) {
      throw new Error('baseURL missing')
    }
    const teamId = await getFirstTeamId(baseURL, token)
    const title = `E2E past queue ${Date.now()}`
    await seedAutomationReviewDraft(baseURL, token, teamId, title, 'Draft awaiting a valid schedule.')

    await signIn(page)
    await openReviewQueue(page)
    const item = page.getByTestId('review-queue-item').filter({ hasText: title })
    await expect(item).toBeVisible({ timeout: 15_000 })

    await item.getByTestId('review-schedule-at').fill('2020-01-01T10:00')
    await expect(item.getByTestId('review-past-warning')).toBeVisible()
    await expect(item.getByTestId('review-schedule')).toBeDisabled()

    const postId = await getReviewQueueItemId(baseURL, token, teamId, title)
    const updateReq = page.waitForRequest((req) => req.method() === 'PATCH' && req.url().includes(`/posts/${postId}`))
    await item.getByTestId('review-past-publish-now').click()

    const bodyText = (await updateReq).postData() ?? ''
    expect(bodyText).toContain('"publish_now":true')
    expect(bodyText).not.toContain('"content"')

    const post = await getPost(baseURL, token, teamId, postId)
    expect(post.content).toBe('Draft awaiting a valid schedule.')
  })
})
