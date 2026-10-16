import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam } from './helpers'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'

const sse = (event: unknown) => `data: ${JSON.stringify(event)}\n\n`

type ChatBody = {
  messages: { role: string; content: string; tool_results?: { tool: string; summary: string }[] }[]
}

// Tool outcomes (e.g. the post_id from draft_post) must survive into the next
// request so the model does not create a duplicate. The LLM is mocked at the SSE
// boundary; the test inspects the second request body.
test('AI chat sends tool result summaries back with the next request', async ({ page }) => {
  test.setTimeout(90_000)
  const token = e2eBootstrapToken()
  const teamId = await createAITeam(baseURL, token)

  const bodies: ChatBody[] = []
  await page.route('**/ai/chat', async (route) => {
    bodies.push(route.request().postDataJSON() as ChatBody)
    const first = bodies.length === 1
    const body = first
      ? sse({ type: 'message', message: 'Drafting it now.' }) +
        sse({ type: 'tool_call', tool_name: 'draft_post', tool_args: { title: 'Hello' } }) +
        sse({
          type: 'tool_result',
          tool_name: 'draft_post',
          message: 'Created draft.',
          payload: { post_id: 'post-123', content: 'Hello world' },
        }) +
        sse({ type: 'tool_call', tool_name: 'fetch_url', tool_args: { url: 'https://example.com' } }) +
        sse({ type: 'tool_result', tool_name: 'fetch_url', message: 'INJECTED-PAGE-TEXT' }) +
        sse({ type: 'done' })
      : sse({ type: 'message', message: 'Done.' }) + sse({ type: 'done' })
    await route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' },
      body,
    })
  })

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
  const chatInput = drawer.locator('textarea')

  await chatInput.click()
  await chatInput.fill('draft a hello post')
  await chatInput.press('Enter')
  await expect(drawer.getByText('Drafting it now.')).toBeVisible({ timeout: 15_000 })
  await expect.poll(() => bodies.length).toBe(1)

  await chatInput.click()
  await chatInput.fill('make it shorter')
  await chatInput.press('Enter')
  await expect.poll(() => bodies.length, { timeout: 15_000 }).toBe(2)

  const assistant = bodies[1].messages.find((m) => m.role === 'assistant' && m.tool_results?.length)
  expect(assistant).toBeTruthy()
  expect(assistant?.tool_results?.[0].tool).toBe('draft_post')
  expect(assistant?.tool_results?.[0].summary).toContain('post-123')
  expect(JSON.stringify(bodies[1])).not.toContain('INJECTED-PAGE-TEXT')
  expect(bodies[1].messages.at(-1)?.content).toBe('make it shorter')
})
