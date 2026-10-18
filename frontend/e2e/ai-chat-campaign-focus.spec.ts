import { expect, test } from '@playwright/test'
import { signIn } from './helpers'

test('campaign editor sends saved identity and current unsaved fields, then clears on close', async ({ page }) => {
  const bodies: { view_context?: { focus?: unknown; visible?: { campaign_editor?: { name: string; dirty: boolean; truncated?: boolean; structure?: { topic?: string } } } } }[] = []
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.route('**/v1/teams', async route => {
    const response = await route.fetch()
    const data = await response.json()
    await route.fulfill({ json: { items: data.items.map((team: object) => ({ ...team, is_ai_enabled: true })) } })
  })
  await page.route('**/v1/teams/*/campaign-formats', route => route.fulfill({ json: { items: [{
    id: 'focused-format', name: 'Saved format', weekday: 2, structure: { topic: 'Saved topic' }, required_hashtags: ['launch'], is_active: true,
  }] } }))
  await page.route('**/v1/teams/*/ai/chat', async route => {
    bodies.push(route.request().postDataJSON())
    await route.fulfill({ contentType: 'text/event-stream', body: 'data: {"type":"done"}\n\n' })
  })
  await signIn(page)
  await page.getByRole('button', { name: 'Campaign Formats', exact: true }).click()
  await page.getByTestId('campaign-format-focused-format').locator('button').first().click()
  await expect(page.getByTestId('campaign-dialog')).toBeVisible()
  await page.getByTestId('ai-chat-fab').click()
  const drawer = page.locator('.ai-chat-drawer')
  const send = async () => {
    const count = bodies.length
    await drawer.locator('textarea').fill('Improve this format')
    await drawer.getByRole('button', { name: 'Send', exact: true }).click()
    await expect.poll(() => bodies.length).toBe(count + 1)
    await expect(drawer.locator('textarea')).toBeEnabled()
    return bodies.at(-1)!.view_context
  }
  expect((await send())?.focus).toEqual({ type: 'campaign_format', id: 'focused-format' })
  expect(bodies.at(-1)?.view_context?.visible?.campaign_editor).toMatchObject({ name: 'Saved format', dirty: false })
  await drawer.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByTestId('campaign-dialog-name').fill('Unsaved format')
  await page.getByTestId('campaign-dialog-topic').fill('T'.repeat(5000))
  await page.getByTestId('ai-chat-fab').click()
  const unsaved = (await send())?.visible?.campaign_editor
  expect(unsaved).toMatchObject({ name: 'Unsaved format', dirty: true, truncated: true })
  expect(unsaved?.structure?.topic).toBe('T'.repeat(1000))
  await drawer.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByRole('button', { name: 'Cancel', exact: true }).click()
  await page.getByTestId('ai-chat-fab').click()
  const closed = await send()
  expect(closed?.focus).toBeUndefined()
  expect(closed?.visible?.campaign_editor).toBeUndefined()
  await drawer.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByTestId('campaign-create-btn').click()
  await page.getByTestId('campaign-dialog-name').fill('New unsaved format')
  await page.getByTestId('ai-chat-fab').click()
  const created = await send()
  expect(created?.focus).toEqual({ type: 'campaign_format' })
  expect(created?.visible?.campaign_editor).toMatchObject({ name: 'New unsaved format', dirty: true })
  await drawer.getByRole('button', { name: 'Close', exact: true }).click()
  await page.getByRole('button', { name: 'Home', exact: true }).click()
  await page.getByTestId('ai-chat-fab').click()
  const navigated = await send()
  expect(navigated?.focus).toBeUndefined()
  expect(navigated?.visible?.campaign_editor).toBeUndefined()
})
