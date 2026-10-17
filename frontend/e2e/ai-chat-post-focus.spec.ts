import { expect, test, type Page } from '@playwright/test'
import { signIn, openContentCalendar } from './helpers'

type ChatBody = { messages: { content: string }[]; view_context?: { section?: string; focus?: { type: string; id?: string }; visible?: { composer?: unknown } } }

async function setup(page: Page) {
  const bodies: ChatBody[] = []
  let firstTeamId = ''
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.route('**/v1/teams', async route => {
    const response = await route.fetch()
    const { items } = await response.json()
    const team = items[0]
    firstTeamId = team.id
    await route.fulfill({ json: { items: [
      { ...team, name: 'Focus Alpha', is_ai_enabled: true },
      { ...team, id: 'focus-beta', name: 'Focus Beta', is_ai_enabled: true },
    ] } })
  })
  await page.route('**/v1/teams/*/**', async route => {
    const url = new URL(route.request().url())
    const [, , , teamId, ...parts] = url.pathname.split('/')
    const resource = parts.join('/')
    if (resource === 'ai/chat') {
      bodies.push(route.request().postDataJSON() as ChatBody)
      await route.fulfill({ contentType: 'text/event-stream', body: 'data: {"type":"done"}\n\n' })
    } else if (resource === 'members') {
      await route.fulfill({ response: await route.fetch({ url: route.request().url().replace(teamId, firstTeamId) }) })
    } else if (resource === 'posts') {
      await route.fulfill({ json: { items: teamId === firstTeamId ? [{
        id: 'focused-post', team_id: teamId, title: 'Focus test post', content: 'Persisted post content',
        status: 'draft', scheduled_at: new Date().toISOString(), target_accounts: ['focus-account'],
      }] : [] } })
    } else if (resource === 'accounts') {
      await route.fulfill({ json: { items: [{ id: teamId === firstTeamId ? 'focus-account' : 'focus-beta-account', team_id: teamId, provider: 'mastodon', username: 'Focus account', instance_url: 'https://example.com', auth_type: 'oauth_token' }] } })
    } else {
      await route.fulfill({ json: { items: [] } })
    }
  })
  await signIn(page)
  await openContentCalendar(page)
  const card = page.getByRole('article').filter({ hasText: 'Focus test post' })
  await expect(card).toBeVisible()
  await card.click()
  await page.getByTestId('ai-chat-fab').click()
  const drawer = page.locator('.ai-chat-drawer')
  async function send() {
    const count = bodies.length
    await drawer.locator('textarea').fill('Improve this post')
    await drawer.getByRole('button', { name: 'Send', exact: true }).click()
    await expect.poll(() => bodies.length).toBe(count + 1)
    await expect(drawer.locator('textarea')).toBeEnabled()
    return bodies.at(-1)!
  }
  async function closeChat() {
    await drawer.getByRole('button', { name: 'Close', exact: true }).click()
  }
  async function openChat() {
    await page.getByTestId('ai-chat-fab').click()
  }
  return { card, send, closeChat, openChat }
}

test('expanded post focus clears on collapse and navigation without reviving on return', async ({ page }) => {
  const { card, send, closeChat, openChat } = await setup(page)
  expect((await send()).view_context?.focus).toEqual({ type: 'post', id: 'focused-post' })
  await closeChat()
  await card.click()
  await openChat()
  expect((await send()).view_context?.focus).toBeUndefined()
  await closeChat()
  await card.click()
  await openChat()
  expect((await send()).view_context?.focus).toEqual({ type: 'post', id: 'focused-post' })
  await closeChat()
  await page.getByRole('button', { name: 'Home', exact: true }).click()
  await openChat()
  expect((await send()).view_context?.focus).toBeUndefined()
  await closeChat()
  await openContentCalendar(page)
  await openChat()
  expect((await send()).view_context?.focus).toBeUndefined()
})

test('team switch clears focused post even when switching back', async ({ page }) => {
  const { send, closeChat, openChat } = await setup(page)
  expect((await send()).view_context?.focus).toEqual({ type: 'post', id: 'focused-post' })
  await closeChat()
  for (const name of ['Focus Beta', 'Focus Alpha']) {
    await page.locator('.sidebar-team-selector').click()
    await page.getByRole('menuitem', { name, exact: true }).click()
    await expect(page.locator('.sidebar-team-selector')).toContainText(name)
    await openChat()
    expect((await send()).view_context?.focus).toBeUndefined()
    await closeChat()
  }
})

test('editing persisted post keeps identity focus through composer edits', async ({ page }) => {
  const { send, closeChat, openChat } = await setup(page)
  await closeChat()
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await expect(page.getByTestId('composer-view')).toBeVisible()
  await openChat()
  const editing = await send()
  expect(editing.view_context?.focus).toEqual({ type: 'post', id: 'focused-post' })
  expect(editing.messages.at(-1)?.content).not.toContain('it has NO id')
  expect(editing.messages.at(-1)?.content).toContain('revise_composer_post')
  await closeChat()
  await page.getByTestId('composer-view').getByLabel(/message.*all destinations/i).fill('Unsaved edit')
  await openChat()
  expect((await send()).view_context?.focus).toEqual({ type: 'post', id: 'focused-post' })
})

test('new composer does not inherit expanded post identity', async ({ page }) => {
  const { send, closeChat, openChat } = await setup(page)
  expect((await send()).view_context?.focus).toEqual({ type: 'post', id: 'focused-post' })
  await closeChat()
  await page.getByRole('button', { name: 'New Post', exact: true }).click()
  await expect(page.getByTestId('composer-view')).toBeVisible()
  await openChat()
  expect((await send()).view_context?.focus?.type).not.toBe('post')
})

test('team switch clears editor draft and chat context before selecting a new destination', async ({ page }) => {
  const { send, closeChat, openChat } = await setup(page)
  await closeChat()
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  const composer = page.getByTestId('composer-view')
  const content = composer.getByLabel(/message.*all destinations/i)
  await expect(content).toHaveValue('Persisted post content')
  await content.fill('Alpha private unsaved draft')
  await openChat()
  expect(JSON.stringify(await send())).toContain('Alpha private unsaved draft')
  await closeChat()

  await page.locator('.sidebar-team-selector').click()
  await page.getByRole('menuitem', { name: 'Focus Beta', exact: true }).click()
  await expect(page.locator('.sidebar-team-selector')).toContainText('Focus Beta')
  await expect(composer).toBeVisible()
  const destination = composer.getByTestId('composer-destination-toggle')
  await expect(destination).toHaveCount(1)
  await expect(destination).toHaveAttribute('aria-pressed', 'false')
  await destination.click()
  await openChat()
  const betaRequest = await send()
  expect(JSON.stringify(betaRequest)).not.toContain('Alpha private unsaved draft')
  expect(JSON.stringify(betaRequest)).not.toContain('Focus test post')
  expect(JSON.stringify(betaRequest)).not.toContain('Persisted post content')
  expect(betaRequest.view_context?.focus?.type).not.toBe('post')
  expect(betaRequest.view_context?.visible?.composer).toBeUndefined()
  await closeChat()
  await expect(content).toHaveValue('')
  await expect(page.getByTestId('composer-title')).toHaveText('Create post')
})

test('mobile post preview reports focus and clears it on dismissal', async ({ page }) => {
  const { card, send, closeChat, openChat } = await setup(page)
  await closeChat()
  await page.setViewportSize({ width: 390, height: 844 })
  await card.click()
  const preview = page.getByTestId('mobile-preview-overlay')
  await expect(preview).toBeVisible()
  await openChat()
  expect((await send()).view_context?.focus).toEqual({ type: 'post', id: 'focused-post' })
  await closeChat()
  await preview.locator('.mobile-preview-header button').first().click()
  await expect(preview).not.toBeVisible()
  await openChat()
  expect((await send()).view_context?.focus).toBeUndefined()
})
