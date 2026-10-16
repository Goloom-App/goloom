import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam, signIn } from './helpers'

test('prompt preview uses current refine settings and reports preview errors', async ({ page, baseURL }) => {
  test.setTimeout(90_000)
  const teamId = await createAITeam(baseURL!, e2eBootstrapToken())
  const content = 'Existing generated draft to refine.'
  const taskPrompt = 'Refine an existing draft for multi-account publishing.\n' +
    'Primary account: long (id=long-account, limit 5000 characters).\n' +
    'account_content_override must ONLY contain compressed variants for these lower-limit accounts: blue.\n' +
    'Accounts:\n- long (id=long-account, mastodon): max 5000 characters\n' +
    '- blue (id=blue-account, bluesky): max 300 characters'
  await page.route(`**/v1/teams/${teamId}/accounts`, (route) => route.fulfill({ json: {
    items: [
      { id: 'short-account', team_id: teamId, provider: 'mastodon', username: 'short', instance_url: 'https://example.com', max_chars_override: 500 },
      { id: 'long-account', team_id: teamId, provider: 'mastodon', username: 'long', instance_url: 'https://example.com', max_chars_override: 5000 },
      { id: 'blue-account', team_id: teamId, provider: 'bluesky', username: 'blue', instance_url: 'https://bsky.social', max_chars_override: 300 },
    ],
  } }))
  await page.route(`**/v1/teams/${teamId}/ai-jobs`, (route) => route.fulfill({ json: {
    items: [{ id: 'completed-job', team_id: teamId, type: 'voice_engine', status: 'completed', payload: {}, result: { content }, created_at: '2026-10-17T10:00:00Z' }],
  } }))

  let previewParams: Record<string, unknown> | undefined
  let refineParams: Record<string, unknown> | undefined
  let failPreview = false
  await page.route(`**/v1/teams/${teamId}/ai/prompt-preview`, (route) => {
    previewParams = route.request().postDataJSON().params
    return failPreview
      ? route.fulfill({ status: 400, json: { error: 'invalid_ai_prompt_params' } })
      : route.fulfill({ json: { system_prompt: 'Team voice', generation_prompt: taskPrompt } })
  })
  await page.route(`**/v1/teams/${teamId}/ai-trigger`, (route) => {
    refineParams = route.request().postDataJSON().params
    return route.fulfill({ status: 202, json: { status: 'pending' } })
  })

  await signIn(page)
  await page.goto(`/?team=${teamId}`)
  await page.getByRole('button', { name: 'AI Generator', exact: true }).click()
  await expect(page.getByText(content, { exact: true })).toBeVisible()
  await page.getByTestId('gen-dest-long-account').click()
  await page.getByTestId('gen-dest-blue-account').click()
  await page.getByTestId('gen-occasion').fill('Updated occasion')
  await page.getByTestId('gen-mood-shorter_punchier').click()
  await page.getByTestId('gen-prompt-preview').click()
  await expect(page.getByTestId('gen-prompt-system').locator('pre')).toHaveText('Team voice')
  await expect(page.getByTestId('gen-prompt-task').locator('pre')).toHaveText(taskPrompt)
  await expect(page.getByTestId('gen-dest-short-account')).toHaveAttribute('aria-pressed', 'false')
  expect(previewParams).toMatchObject({
    refine_content: true,
    source_content: content,
    occasion: 'Updated occasion',
    target_account_ids: ['long-account', 'blue-account'],
    mood_adjustments: ['shorter_punchier'],
  })
  await expect(page.getByTestId('gen-prompt-preview')).toContainText('aktuelle Einstellungen')
  await page.getByRole('button', { name: 'Neu generieren', exact: true }).click()
  await expect.poll(() => refineParams).toEqual(previewParams)

  failPreview = true
  await page.getByTestId('gen-prompt-preview').click()
  await expect(page.getByTestId('gen-status-error')).toBeVisible()
})
