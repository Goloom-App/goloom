import { expect, test, type Page } from '@playwright/test'
import { signIn } from './helpers'

type ChatContext = {
  focus?: { type: string; id?: string }
  visible?: { automation?: { title?: string; has_unsaved_changes?: boolean } }
}

async function setup(page: Page) {
  const contexts: ChatContext[] = []
  let firstTeamId = ''
  await page.route('**/v1/teams', async route => {
    const response = await route.fetch()
    const data = await response.json()
    const team = data.items[0]
    firstTeamId = team.id
    await route.fulfill({ json: { items: [
      { ...team, name: 'Automation Alpha', is_ai_enabled: true },
      { ...team, id: 'automation-beta', name: 'Automation Beta', is_ai_enabled: true },
    ] } })
  })
  await page.route('**/v1/teams/automation-beta/**', async route => {
    const url = route.request().url()
    if (url.endsWith('/members')) {
      await route.fulfill({ response: await route.fetch({ url: url.replace('automation-beta', firstTeamId) }) })
    } else {
      await route.fulfill({ json: { items: [] } })
    }
  })
  await page.route('**/v1/teams/*/ai/chat', async route => {
    contexts.push(route.request().postDataJSON().view_context)
    await route.fulfill({ contentType: 'text/event-stream', body: 'data: {"type":"done"}\n\n' })
  })
  await page.route('**/v1/teams/*/post-templates', route => route.fulfill({ json: { items: [{
    id: 'focused-template', title: 'Saved recurring title', content: 'Saved recurring content',
    recurrence_json: '{"kind":"weekly","weekdays":[1],"hour":9,"minute":0,"timezone":"UTC"}',
    target_account_ids: [], enabled: true,
  }] } }))
  await page.route('**/v1/teams/*/rss-feeds', route => route.fulfill({ json: { items: [{
    id: 'focused-feed', name: 'Saved RSS title', feed_url: 'https://example.org/feed',
    content_template: '{title}', output_mode: 'draft', target_account_ids: [], is_active: true,
  }] } }))
  await page.setViewportSize({ width: 1920, height: 1080 })
  await signIn(page)
  await page.getByRole('button', { name: /^Automation$/i }).click()
  async function send() {
    await page.getByTestId('ai-chat-fab').click()
    const drawer = page.locator('.ai-chat-drawer')
    await drawer.locator('textarea').fill('Improve this automation')
    const count = contexts.length
    await drawer.getByRole('button', { name: 'Send', exact: true }).click()
    await expect.poll(() => contexts.length).toBe(count + 1)
    await expect(drawer.locator('textarea')).toBeEnabled()
    await drawer.getByRole('button', { name: 'Close', exact: true }).click()
    return contexts.at(-1)!
  }
  return { send }
}

for (const kind of ['recurring_template', 'rss_feed'] as const) {
  test(`${kind} editor supplies stable identity and current unsaved fields; clears on close and tab change`, async ({ page }) => {
    const { send } = await setup(page)
    const rss = kind === 'rss_feed'
    if (rss) await page.getByTestId('automation-tab-rss').click()
    await page.getByRole('button', { name: rss ? /^edit feed$/i : /^edit$/i }).first().click()
    const dialog = page.getByRole('dialog')
    const title = dialog.locator('input').first()
    const id = rss ? 'focused-feed' : 'focused-template'
    expect((await send()).focus).toEqual({ type: kind, id })
    expect((await send()).visible?.automation?.has_unsaved_changes).toBe(false)
    await title.fill('Unsaved automation title')
    const changed = await send()
    expect(changed.focus).toEqual({ type: kind, id })
    expect(changed.visible?.automation).toMatchObject({ title: 'Unsaved automation title', has_unsaved_changes: true })
    await title.fill(rss ? 'Saved RSS title' : 'Saved recurring title')
    expect((await send()).visible?.automation?.has_unsaved_changes).toBe(false)
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).last().click()
    expect((await send()).focus).toBeUndefined()
    await page.getByRole('button', { name: rss ? /add feed/i : /add template/i }).click()
    await page.getByRole('dialog').locator('input').first().fill('New unsaved automation')
    expect((await send()).focus).toEqual({ type: kind })
    await page.getByRole('dialog').getByRole('button', { name: 'Cancel', exact: true }).last().click()
    await page.getByRole('button', { name: rss ? /^edit feed$/i : /^edit$/i }).first().click()
    await page.getByTestId(rss ? 'automation-tab-recurring' : 'automation-tab-rss').click()
    expect((await send()).visible?.automation).toBeUndefined()
    await page.getByTestId(rss ? 'automation-tab-rss' : 'automation-tab-recurring').click()
    await page.getByRole('button', { name: rss ? /^edit feed$/i : /^edit$/i }).first().click()
    await page.locator('.sidebar-team-selector').click()
    await page.getByRole('menuitem', { name: 'Automation Beta', exact: true }).click()
    await expect(page.locator('.sidebar-team-selector')).toContainText('Automation Beta')
    const switched = await send()
    expect(switched.focus).toBeUndefined()
    expect(switched.visible?.automation).toBeUndefined()
  })
}
