import { test, expect } from '@playwright/test'
import { createAITeam, signIn } from './helpers'
import { e2eBootstrapToken } from './constants'

test('viewer cannot save review text as an example', async ({ page, baseURL }) => {
  test.setTimeout(90_000)
  const teamId = await createAITeam(baseURL!, e2eBootstrapToken())
  await page.route(`**/v1/teams/${teamId}/members`, async route => {
    const response = await route.fetch()
    const data = await response.json()
    await route.fulfill({ json: { items: data.items.map((member: Record<string, unknown>) => ({ ...member, role: 'viewer' })) } })
  })
  await page.route(`**/v1/teams/${teamId}/review-queue`, route => route.fulfill({ json: { items: [{
    id: 'viewer-style-post', team_id: teamId, title: 'Viewer style source', content: 'Reviewed final text',
    scheduled_at: '2030-01-01T12:00:00Z', status: 'draft', source: 'automation', target_accounts: [],
  }] } }))
  await signIn(page)
  await page.goto(`/?team=${teamId}`)
  await page.getByTestId('nav-review-queue').click()
  await expect(page.getByTestId('review-queue-item')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Save as style example', exact: true })).toHaveCount(0)
})
