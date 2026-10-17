import { test, expect, type Page } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam } from './helpers'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'
type ChatBody = {
  messages: { content: string }[]
  view_context?: { focus?: { type: string }; visible?: { composer?: { targets: { accountId: string }[] } } }
}

async function openComposerChat(page: Page) {
  const token = e2eBootstrapToken()
  const teamId = await createAITeam(baseURL, token)
  await page.route(`**/v1/teams/${teamId}/accounts`, route => route.fulfill({ json: { items: [
    { id: 'scope-a', team_id: teamId, provider: 'mastodon', username: 'Scope Alpha', instance_url: 'https://example.com', auth_type: 'oauth_token' },
    { id: 'scope-b', team_id: teamId, provider: 'bluesky', username: 'Scope Beta', instance_url: 'https://bsky.social', auth_type: 'app_password' },
  ] } }))
  const bodies: ChatBody[] = []
  await page.route('**/ai/chat', async route => {
    bodies.push(route.request().postDataJSON() as ChatBody)
    await route.fulfill({ contentType: 'text/event-stream', body: 'data: {"type":"done"}\n\n' })
  })
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.goto('/')
  await page.getByLabel(/access token|administrator token/i).fill(token)
  await page.getByRole('button', { name: 'Sign in with token' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible()
  await page.goto(`/?team=${teamId}`)
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible()
  await page.getByRole('button', { name: 'Calendar', exact: true }).click()
  await page.getByRole('button', { name: 'New Post' }).click()
  const composer = page.getByTestId('composer-view')
  const destinations = composer.getByTestId('composer-destination-toggle')
  await expect(destinations).toHaveCount(2)
  for (const destination of await destinations.all()) {
    if (await destination.getAttribute('aria-pressed') !== 'true') await destination.click()
  }
  const editor = composer.getByLabel(/message.*all destinations|nachricht.*alle ziele/i)
  await editor.fill('Composer secret draft')
  await page.getByTestId('ai-chat-fab').click()
  const drawer = page.locator('.ai-chat-drawer')
  await expect(drawer.locator('.ai-chat-chip--context')).toBeVisible()
  async function send(text: string) {
    const count = bodies.length
    await drawer.locator('textarea').fill(text)
    await drawer.getByRole('button', { name: 'Send', exact: true }).click()
    await expect.poll(() => bodies.length).toBe(count + 1)
    await expect(drawer.locator('textarea')).toBeEnabled()
    return bodies.at(-1)!
  }
  return { composer, drawer, editor, bodies, send }
}

for (const action of ['detach', '/clear'] as const) {
  test(`AI chat ${action} removes composer from outgoing requests and survives edits`, async ({ page }) => {
    const { drawer, editor, send } = await openComposerChat(page)
    if (action === 'detach') await drawer.locator('.ai-chat-chip--context').click()
    else {
      await drawer.locator('textarea').fill('/clear')
      await drawer.getByRole('button', { name: 'Send', exact: true }).click()
    }
    await expect(drawer.locator('.ai-chat-chip--context')).toHaveCount(0)
    const first = await send('Help without draft')
    expect(first.view_context?.visible?.composer).toBeUndefined()
    expect(first.view_context?.focus?.type).not.toBe('composerDraft')
    expect(first.messages.at(-1)?.content).toBe('Help without draft')
    await editor.fill('Composer secret draft changed')
    const afterEdit = await send('Still without draft')
    expect(afterEdit.view_context?.visible?.composer).toBeUndefined()
    expect(afterEdit.messages.at(-1)?.content).toBe('Still without draft')
    await expect(drawer.locator('.ai-chat-chip--context')).toHaveCount(0)

    // Existing explicit composer action re-attaches without closing the composer.
    await page.getByTestId('composer-ai-chat').click()
    await expect(drawer.locator('.ai-chat-chip--context')).toBeVisible()
    const reattached = await send('Use draft again')
    expect(reattached.view_context?.visible?.composer?.targets).toHaveLength(2)
  })
}

test('AI chat excludes deselected accounts from both view context and composer note', async ({ page }) => {
  const { drawer, send } = await openComposerChat(page)
  await drawer.locator('.ai-chat-chip--scope', { hasText: 'Scope Beta' }).click()
  const scoped = await send('Revise selected account')
  expect(scoped.view_context?.visible?.composer?.targets.map(target => target.accountId)).toEqual(['scope-a'])
  expect(scoped.messages.at(-1)?.content).toContain('Scope Alpha')
  expect(scoped.messages.at(-1)?.content).not.toContain('Scope Beta')
  await drawer.locator('.ai-chat-chip--scope', { hasText: 'Scope Alpha' }).click()
  const empty = await send('No accounts selected')
  expect(empty.view_context?.visible?.composer).toBeUndefined()
  expect(empty.messages.at(-1)?.content).toBe('No accounts selected')
})
