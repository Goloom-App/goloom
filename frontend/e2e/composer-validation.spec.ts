import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { ensureE2EAccount, getFirstTeamId, signIn } from './helpers'

test.describe('composer validation', () => {
  test('save button disabled when no destinations selected', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })

    await signIn(page)

    // Navigate to calendar to access new post button
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()

    // Open new post composer
    await page.getByRole('button', { name: 'New Post' }).click()
    await expect(page.getByTestId('composer-view')).toBeVisible({ timeout: 10_000 })

    // Type content (label is "Message (all destinations)", not "Content" — tablist uses contentScopeAria)
    await page
      .getByTestId('composer-view')
      .getByLabel(/message.*all destinations|nachricht.*alle ziele/i)
      .fill('Test content for validation')

    // Save button should be disabled because no destinations are selected
    const saveBtn = page.getByRole('button', { name: /schedule post|speichern/i })
    await expect(saveBtn).toBeDisabled()

    // Save draft should be enabled regardless
    await expect(page.getByRole('button', { name: /save draft/i })).toBeEnabled()
  })

  test('generates an internal title from content before scheduling', async ({ page, baseURL }) => {
    test.setTimeout(60_000)
    if (!baseURL) throw new Error('baseURL missing')
    const token = e2eBootstrapToken()
    const teamId = await getFirstTeamId(baseURL, token)
    await ensureE2EAccount(baseURL, token, teamId)

    await signIn(page)
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await page.getByRole('button', { name: 'New Post' }).click()

    const composer = page.getByTestId('composer-view')
    const content = `Title-free post ${Date.now()} with enough detail`
    await composer.getByLabel(/message.*all destinations|nachricht.*alle ziele/i).fill(content)

    const validation = page.waitForRequest((request) => request.url().endsWith(`/v1/teams/${teamId}/posts/validate`) && request.method() === 'POST')
    await page.getByRole('button', { name: /schedule post|beitrag planen/i }).click()
    expect(validation ? JSON.parse((await validation).postData() ?? '{}').title : '').toBe(content)
  })

  test('shows a visible scheduling error when the server rejects the post', async ({ page, baseURL }) => {
    test.setTimeout(60_000)
    if (!baseURL) throw new Error('baseURL missing')
    const token = e2eBootstrapToken()
    const teamId = await getFirstTeamId(baseURL, token)
    await ensureE2EAccount(baseURL, token, teamId)

    await signIn(page)
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await page.getByRole('button', { name: 'New Post' }).click()
    await page.getByRole('button', { name: /schedule post|beitrag planen/i }).click()

    await expect(page.getByRole('alert')).toContainText(/content is required|inhalt.*erforderlich/i)
  })
})
