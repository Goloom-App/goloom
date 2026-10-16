import { expect, test, type Page } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam, signIn } from './helpers'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'
const scheduledAt = '2030-01-15T12:00:00Z'

async function openGenerator(page: Page, payload: Record<string, unknown>) {
  const teamId = await createAITeam(baseURL, e2eBootstrapToken())
  await page.route(`**/v1/teams/${teamId}/accounts`, (route) => route.fulfill({ json: {
    items: ['original', 'changed'].map((id) => ({
      id, team_id: teamId, provider: 'mastodon', auth_type: 'oauth_token',
      instance_url: 'https://social.example', username: id, remote_account_id: id,
      created_at: '2026-01-01T00:00:00Z',
    })),
  } }))
  await page.route(`**/v1/teams/${teamId}/ai-jobs`, (route) => route.fulfill({ json: {
    items: [{
      id: 'completed-job', team_id: teamId, type: 'voice_engine', status: 'completed', payload,
      result: { title: 'Generated title', content: 'Generated content',
        account_content_override: { original: 'Original account version' }, scheduled_at: scheduledAt },
      created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    }],
  } }))
  await signIn(page)
  await page.goto(`/?team=${teamId}`)
  await page.getByRole('button', { name: 'AI Generator', exact: true }).click()
  await expect(page.getByTestId('gen-editor')).toBeVisible()
  // Change selection after the completed job appears. Actions must retain its original target.
  await page.getByTestId('gen-dest-original').click()
  await page.getByTestId('gen-dest-original').click()
  await page.getByTestId('gen-dest-changed').click()
  return teamId
}

test('generator draft keeps completed job targets after selection changes', async ({ page }) => {
  const teamId = await openGenerator(page, { params: { target_account_ids: ['original'] } })
  let draft: Record<string, unknown> | undefined
  await page.route(`**/v1/teams/${teamId}/posts/draft`, async (route) => {
    draft = route.request().postDataJSON()
    await route.fulfill({ json: { id: 'saved-draft' } })
  })
  await page.getByRole('button', { name: 'Als Entwurf speichern' }).click()
  await expect(page.getByTestId('gen-status-success')).toHaveText('Entwurf gespeichert')
  expect(draft).toMatchObject({ account_ids: ['original'],
    account_content_override: { original: 'Original account version' }, scheduled_at: scheduledAt })
})

test('generator composer keeps job targets, account override and schedule', async ({ page }) => {
  await openGenerator(page, { params: { target_account_ids: ['original'] } })
  await page.getByRole('button', { name: 'Im Composer öffnen' }).click()
  const composer = page.getByTestId('composer-view')
  await expect(composer).toBeVisible()
  await expect(composer.getByTestId('composer-destination-toggle').and(composer.locator('[title="original · mastodon"]'))).toHaveAttribute('aria-pressed', 'true')
  await expect(composer.getByTestId('composer-destination-toggle').and(composer.locator('[title="changed · mastodon"]'))).toHaveAttribute('aria-pressed', 'false')
  const localSchedule = await page.evaluate((value) => {
    const date = new Date(value)
    return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
  }, scheduledAt)
  await expect(composer.locator('input[type="datetime-local"]')).toHaveValue(localSchedule)
  await composer.getByRole('tab', { name: '@original' }).click()
  await expect(composer.locator('textarea').first()).toHaveValue('Original account version')
})

for (const [name, payload] of Object.entries({ missing: {}, empty: { params: { target_account_ids: [] } },
  invalid: { params: { target_account_ids: [42] } }, legacy: { target_account_ids: ['original'] } })) {
  test(`generator rejects ${name} job targets for draft and composer`, async ({ page }) => {
    const teamId = await openGenerator(page, payload)
    const drafts: unknown[] = []
    await page.route(`**/v1/teams/${teamId}/posts/draft`, async (route) => {
      drafts.push(route.request().postDataJSON())
      await route.fulfill({ json: { id: 'unexpected-draft' } })
    })
    await page.getByRole('button', { name: 'Als Entwurf speichern' }).click()
    await expect(page.getByTestId('gen-status-error')).toHaveText('Zielkonten für den Job nicht gefunden')
    expect(drafts).toEqual([])
    await page.getByRole('button', { name: 'Im Composer öffnen' }).click()
    await expect(page.getByTestId('gen-status-error')).toHaveText('Zielkonten für den Job nicht gefunden')
    await expect(page.getByTestId('composer-view')).toHaveCount(0)
  })
}
