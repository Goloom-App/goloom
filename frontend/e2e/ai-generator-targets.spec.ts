import { expect, test, type Page } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam, signIn } from './helpers'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'
const scheduledAt = '2030-01-15T12:00:00Z'
const originalTargets = ['original', 'bluesky']
const accountContentOverride = {
  original: 'Mastodon account version',
  'bluesky': 'Bluesky account version',
}

async function openGenerator(page: Page, payload: Record<string, unknown>) {
  const teamId = await createAITeam(baseURL, e2eBootstrapToken())
  await page.route(`**/v1/teams/${teamId}/accounts`, (route) => route.fulfill({ json: {
    items: [
      { id: 'original', provider: 'mastodon' },
      { id: 'bluesky', provider: 'bluesky' },
      { id: 'changed', provider: 'mastodon' },
    ].map(({ id, provider }) => ({
      id, team_id: teamId, provider, auth_type: 'oauth_token',
      instance_url: 'https://social.example', username: id, remote_account_id: id,
      created_at: '2026-01-01T00:00:00Z',
    })),
  } }))
  await page.route(`**/v1/teams/${teamId}/ai-jobs`, (route) => route.fulfill({ json: {
    items: [{
      id: 'completed-job', team_id: teamId, type: 'voice_engine', status: 'completed', payload,
      result: { title: 'Generated title', content: 'Generated content',
        account_content_override: accountContentOverride, scheduled_at: scheduledAt },
      created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    }],
  } }))
  await signIn(page)
  await page.goto(`/?team=${teamId}`)
  await page.getByRole('button', { name: 'AI Generator', exact: true }).click()
  await expect(page.getByTestId('gen-editor')).toBeVisible()
  // Change selection after completion. Actions must retain both original targets.
  for (const id of originalTargets) {
    await page.getByTestId(`gen-dest-${id}`).click()
    await page.getByTestId(`gen-dest-${id}`).click()
    await expect(page.getByTestId(`gen-dest-${id}`)).toHaveAttribute('aria-pressed', 'false')
  }
  await page.getByTestId('gen-dest-changed').click()
  await expect(page.getByTestId('gen-dest-changed')).toHaveAttribute('aria-pressed', 'true')
  return teamId
}

test('generator draft keeps mixed-provider job targets after selection changes', async ({ page }) => {
  const teamId = await openGenerator(page, { params: { target_account_ids: originalTargets } })
  let draft: Record<string, unknown> | undefined
  await page.route(`**/v1/teams/${teamId}/posts/draft`, async (route) => {
    draft = route.request().postDataJSON()
    await route.fulfill({ json: { id: 'saved-draft' } })
  })
  await page.getByRole('button', { name: 'Als Entwurf speichern' }).click()
  await expect(page.getByTestId('gen-status-success')).toHaveText('Entwurf gespeichert')
  expect(draft?.account_ids).toEqual(originalTargets)
  expect(draft?.account_content_override).toEqual(accountContentOverride)
  expect(draft?.scheduled_at).toBe(scheduledAt)
})

test('generator composer keeps mixed-provider job targets, variants and schedule', async ({ page }) => {
  await openGenerator(page, { params: { target_account_ids: originalTargets } })
  await page.getByRole('button', { name: 'Im Composer öffnen' }).click()
  const composer = page.getByTestId('composer-view')
  await expect(composer).toBeVisible()
  await expect(composer.getByTestId('composer-destination-toggle').and(composer.locator('[title="original · mastodon"]'))).toHaveAttribute('aria-pressed', 'true')
  await expect(composer.getByTestId('composer-destination-toggle').and(composer.locator('[title="bluesky · bluesky"]'))).toHaveAttribute('aria-pressed', 'true')
  await expect(composer.getByTestId('composer-destination-toggle').and(composer.locator('[aria-pressed="true"]'))).toHaveCount(2)
  await expect(composer.getByTestId('composer-destination-toggle').and(composer.locator('[title="changed · mastodon"]'))).toHaveAttribute('aria-pressed', 'false')
  const localSchedule = await page.evaluate((value) => {
    const date = new Date(value)
    return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
  }, scheduledAt)
  await expect(composer.locator('input[type="datetime-local"]')).toHaveValue(localSchedule)
  for (const [id, content] of Object.entries(accountContentOverride)) {
    await composer.getByRole('tab', { name: `@${id}`, exact: true }).click()
    await expect(composer.locator('textarea').first()).toHaveValue(content)
  }
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
