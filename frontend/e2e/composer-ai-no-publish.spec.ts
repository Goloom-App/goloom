import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam, signIn } from './helpers'

test('Composer optimization requests text without publishing', async ({ page, baseURL }) => {
  test.setTimeout(90_000)
  const teamId = await createAITeam(baseURL!, e2eBootstrapToken())
  const accountId = 'composer-ai-account'
  await page.route(`**/v1/teams/${teamId}/profile`, (route) => route.fulfill({ json: {
    id: 'composer-ai-profile', team_id: teamId, style_metadata: {}, auto_publish_enabled: true,
    created_at: '2026-10-17T10:00:00Z', updated_at: '2026-10-17T10:00:00Z',
  } }))
  const postCreateRequests: string[] = []
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname
    if (request.method() === 'POST' && [
      `/v1/teams/${teamId}/posts`,
      `/v1/teams/${teamId}/posts/draft`,
    ].includes(path)) postCreateRequests.push(path)
  })
  await page.route(`**/v1/teams/${teamId}/accounts`, (route) => route.fulfill({ json: {
    items: [{ id: accountId, team_id: teamId, provider: 'mastodon', username: 'composer', instance_url: 'https://example.com' }],
  } }))
  let params: Record<string, unknown> | undefined
  await page.route(`**/v1/teams/${teamId}/ai-trigger`, (route) => {
    params = route.request().postDataJSON().params
    return route.fulfill({ status: 202, json: { job_id: 'editor-job', status: 'pending' } })
  })
  await page.route(`**/v1/teams/${teamId}/ai-jobs/editor-job`, (route) => route.fulfill({ json: {
    id: 'editor-job', team_id: teamId, type: 'voice_engine', status: 'completed', result: { content: 'Optimized editor text' },
  } }))

  await signIn(page)
  await page.goto(`/?team=${teamId}`)
  await page.getByRole('button', { name: 'Calendar', exact: true }).click()
  await page.getByRole('button', { name: 'New Post' }).click()
  const composer = page.getByTestId('composer-view')
  const content = composer.getByLabel(/message.*all destinations|nachricht.*alle ziele/i)
  await content.fill('Original editor text')
  const destination = composer.getByTestId('composer-destination-toggle')
  if (await destination.getAttribute('aria-pressed') !== 'true') await destination.click()
  await page.getByTestId('composer-ai-optimize').click()
  await page.getByTestId('composer-ai-optimize').click()
  await expect(content).toHaveValue('Optimized editor text')
  expect(postCreateRequests).toEqual([])
  expect(params).toMatchObject({
    refine_content: true,
    no_publish: true,
    source_content: 'Original editor text',
    target_account_ids: [accountId],
  })
})
