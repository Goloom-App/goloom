import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam } from './helpers'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'

// Encode one SSE event the way the chat backend streams it.
const sse = (event: unknown) => `data: ${JSON.stringify(event)}\n\n`

async function signInAndOpenChat(page: import('@playwright/test').Page, token: string, teamId: string) {
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')
  await page.getByLabel(/access token|administrator token/i).fill(token)
  await page.getByRole('button', { name: 'Sign in with token' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })
  await page.goto(`/?team=${teamId}`)
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 15_000 })
  await page.getByTestId('ai-chat-fab').click()
  const drawer = page.locator('.ai-chat-drawer')
  await expect(drawer).toBeVisible()
  return drawer
}

// A tool call fails (content overran a platform limit) but the server-side agent
// loop recovers in the same stream. The chat must NOT alarm the user with a red
// error bubble — it shows a muted "Adjusting…" step and the final answer lands.
test('AI chat shows a transient tool failure as a muted step, not an error', async ({ page }) => {
  test.setTimeout(90_000)
  const token = e2eBootstrapToken()
  const teamId = await createAITeam(baseURL, token)

  await page.route('**/ai/chat', async (route) => {
    const body =
      sse({ type: 'tool_call', tool_name: 'modify_post', tool_args: { content: 'way too long for bluesky' } }) +
      sse({
        type: 'tool_result',
        tool_name: 'modify_post',
        message: 'Error: character limit exceeded: bluesky allows 300 characters but the text has 389.',
      }) +
      sse({ type: 'message', message: 'I added a shorter Bluesky version.' }) +
      sse({ type: 'done' })
    await route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' },
      body,
    })
  })

  const drawer = await signInAndOpenChat(page, token, teamId)
  const chatInput = drawer.locator('textarea')
  await chatInput.click()
  await chatInput.fill('add a bluesky version to the post')
  await chatInput.press('Enter')

  // The recovered answer renders, the muted step is shown, and no error bubble.
  await expect(drawer.getByText('I added a shorter Bluesky version.')).toBeVisible({ timeout: 15_000 })
  await expect(drawer.locator('.ai-chat-bubble--tool', { hasText: 'Adjusting…' })).toBeVisible()
  await expect(drawer.locator('.ai-chat-bubble--error')).toHaveCount(0)
})

// The /clear command starts a fresh agent session: the conversation is wiped and
// the empty state returns.
test('AI chat /clear resets the conversation', async ({ page }) => {
  test.setTimeout(90_000)
  const token = e2eBootstrapToken()
  const teamId = await createAITeam(baseURL, token)

  await page.route('**/ai/chat', async (route) => {
    const body = sse({ type: 'message', message: 'Here is a quick idea for your feed.' }) + sse({ type: 'done' })
    await route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' },
      body,
    })
  })

  const drawer = await signInAndOpenChat(page, token, teamId)
  const chatInput = drawer.locator('textarea')
  await chatInput.click()
  await chatInput.fill('give me a post idea')
  await chatInput.press('Enter')

  await expect(drawer.getByText('Here is a quick idea for your feed.')).toBeVisible({ timeout: 15_000 })
  await expect(drawer.getByText('give me a post idea')).toBeVisible()

  // /clear wipes everything and brings back the empty state.
  await chatInput.fill('/clear')
  await chatInput.press('Enter')

  await expect(drawer.getByText('How can I help with your social media today?')).toBeVisible()
  await expect(drawer.getByText('give me a post idea')).toHaveCount(0)
  await expect(drawer.getByText('Here is a quick idea for your feed.')).toHaveCount(0)
})
