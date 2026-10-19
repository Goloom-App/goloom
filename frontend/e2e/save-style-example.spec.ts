import { test, expect } from '@playwright/test'
import { createAITeam, signIn } from './helpers'
import { e2eBootstrapToken } from './constants'

for (const [entry, source] of [['composer', 'shared'], ['composer', 'variant'], ['review', 'shared'], ['review', 'variant']] as const) {
  test(`${entry} saves corrected ${source} text without changing post`, async ({ page, baseURL }) => {
    test.setTimeout(90_000)
    const teamId = await createAITeam(baseURL!, e2eBootstrapToken())
    const accountId = 'style-account'
    const corrected = 'Final manually corrected shared text'
    const variant = 'Final corrected Mastodon variant'
    const post = { id: 'style-post', team_id: teamId, title: 'Style source', content: corrected,
      scheduled_at: '2030-01-01T12:00:00Z', status: 'draft', source: 'automation', target_accounts: [accountId] }
    await page.route(`**/v1/teams/${teamId}/accounts`, route => route.fulfill({ json: { items: [
      { id: accountId, team_id: teamId, provider: 'mastodon', username: 'style', instance_url: 'https://example.com' },
    ] } }))
    await page.route(`**/v1/teams/${teamId}/review-queue`, route => route.fulfill({ json: { items: [post] } }))
    await page.route(`**/v1/teams/${teamId}/posts/style-post/versions`, route => route.fulfill({ json: { items: [
      { post_id: post.id, account_id: accountId, content: variant },
    ] } }))
    const postMutations: string[] = []
    let exampleCreates = 0
    page.on('request', request => {
      if (request.method() === 'POST' && request.url().endsWith('/style-examples')) exampleCreates++
      if (request.method() !== 'GET' && /\/posts(?:\/|$)/.test(new URL(request.url()).pathname)) postMutations.push(request.url())
    })
    await signIn(page)
    await page.goto(`/?team=${teamId}`)
    if (entry === 'composer') {
      await page.getByRole('button', { name: 'Calendar', exact: true }).click()
      await page.getByRole('button', { name: 'New Post' }).click()
      const composer = page.getByTestId('composer-view')
      await composer.getByLabel(/message.*all destinations/i).fill(corrected)
      const destination = composer.getByTestId('composer-destination-toggle')
      if (await destination.getAttribute('aria-pressed') !== 'true') await destination.click()
      if (source === 'variant') {
        await composer.getByRole('tab', { name: '@style', exact: true }).click()
        await composer.locator('textarea').first().fill(variant)
      }
    } else {
      await page.getByTestId('nav-review-queue').click()
    }
    expect(exampleCreates).toBe(0)
    await page.getByRole('button', { name: 'Save as style example', exact: true }).click()
    const dialog = page.getByRole('dialog')
    if (entry === 'composer' && source === 'variant') {
      await expect(dialog.getByLabel('Example text')).toHaveValue(variant)
      // Shared text remains selectable even when opening from an account tab.
      await dialog.getByLabel('Text source').selectOption('')
    }
    await expect(dialog.getByLabel('Example text')).toHaveValue(corrected)
    if (source === 'variant') {
      await dialog.getByLabel('Text source').selectOption(accountId)
      await expect(dialog.getByLabel('Example text')).toHaveValue(variant)
    }
    await dialog.getByLabel('Platform').selectOption('mastodon')
    await dialog.getByLabel('Note (optional)').fill('Editorial correction')
    // Existing API authorization failures remain visible; retry preserves input.
    await page.route(`**/v1/teams/${teamId}/style-examples`, async route => {
      if (route.request().method() === 'POST') await route.fulfill({ status: 403, json: { error: 'forbidden' } })
      else await route.continue()
    })
    await dialog.getByRole('button', { name: 'Save example', exact: true }).click()
    await expect(dialog.getByRole('alert')).toBeVisible()
    await page.unroute(`**/v1/teams/${teamId}/style-examples`)
    const saved = page.waitForResponse(response => response.url().endsWith(`/teams/${teamId}/style-examples`) && response.request().method() === 'POST')
    await dialog.getByRole('button', { name: 'Save example', exact: true }).click()
    const response = await saved
    expect(response.status()).toBe(201)
    const example = await response.json()
    expect(example).toMatchObject({ content: source === 'variant' ? variant : corrected, platform: 'mastodon', notes: 'Editorial correction' })
    await expect(page.getByRole('status').filter({ hasText: 'Style example saved' })).toBeVisible()
    expect(postMutations).toEqual([])
    expect(exampleCreates).toBe(2) // Explicit failed attempt and explicit successful retry only.
    const list = await page.request.get(`${baseURL}/v1/teams/${teamId}/style-examples`, { headers: { Authorization: `Bearer ${e2eBootstrapToken()}` } })
    expect((await list.json()).items).toEqual(expect.arrayContaining([expect.objectContaining({ content: source === 'variant' ? variant : corrected })]))
    await page.getByRole('button', { name: 'AI Profile', exact: true }).click()
    await page.getByTestId('brand-profile-tab-examples').click()
    await expect(page.getByTestId(`brand-example-${example.id}`)).toContainText(source === 'variant' ? variant : corrected)
  })
}
