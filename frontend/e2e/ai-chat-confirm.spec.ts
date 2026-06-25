import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam } from './helpers'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'

// Encode one SSE event the way the chat backend streams it.
const sse = (event: unknown) => `data: ${JSON.stringify(event)}\n\n`

// The agent proposes a write that needs confirmation (schedule_post). The chat
// must show a confirm card instead of acting, and only run the action — via the
// confirm-action endpoint — when the user approves. The LLM is mocked at the SSE
// boundary so the test is deterministic and needs no provider.
test('AI chat shows a confirm card for a proposed write and runs it on approve', async ({ page }) => {
  test.setTimeout(90_000)
  const token = e2eBootstrapToken()
  const teamId = await createAITeam(baseURL, token)

  // Mock the chat stream: a tool_call then a tool_result carrying a confirmation.
  await page.route('**/ai/chat', async (route) => {
    const body =
      sse({ type: 'tool_call', tool_name: 'schedule_post', tool_args: { title: 'Launch', content: 'Launch day is here!' } }) +
      sse({
        type: 'tool_result',
        tool_name: 'schedule_post',
        message: 'Proposed "schedule_post"; waiting for the user to confirm before it runs.',
        payload: {
          confirmation: {
            tool: 'schedule_post',
            args: {
              title: 'Launch',
              content: 'Launch day is here!',
              scheduled_at: '2099-01-01T10:00:00Z',
              target_accounts: [],
            },
            summary: 'Confirm before running "schedule_post".',
          },
        },
      }) +
      sse({ type: 'message', message: 'I prepared a scheduled post for your review.' }) +
      sse({ type: 'done' })
    await route.fulfill({
      status: 200,
      headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' },
      body,
    })
  })

  // Mock the confirm-action endpoint that runs the approved write.
  let confirmCalls = 0
  await page.route('**/ai/confirm-action', async (route) => {
    confirmCalls += 1
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        summary: '{"post_id":"p1","status":"pending"}',
        payload: { post_id: 'p1', status: 'pending', content: 'Launch day is here!' },
      }),
    })
  })

  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/')
  await page.getByLabel(/access token|administrator token/i).fill(token)
  await page.getByRole('button', { name: 'Sign in with token' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })
  await page.goto(`/?team=${teamId}`)
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 15_000 })

  // Open the chat and ask for something that the mock answers with a proposal.
  await page.getByTestId('ai-chat-fab').click()
  const drawer = page.locator('.ai-chat-drawer')
  await expect(drawer).toBeVisible()

  const chatInput = drawer.locator('textarea')
  await chatInput.click()
  await chatInput.fill('schedule the launch post')
  await chatInput.press('Enter')

  // The proposal renders as a confirm card with approve/dismiss — nothing ran yet.
  const card = drawer.locator('.ai-chat-preview--confirm')
  await expect(card).toBeVisible({ timeout: 15_000 })
  await expect(card.getByRole('button', { name: 'Dismiss', exact: true })).toBeVisible()
  expect(confirmCalls).toBe(0)

  // Approve → the confirm-action endpoint runs and the card reports it confirmed.
  await card.getByRole('button', { name: 'Confirm', exact: true }).click()
  await expect(card).toContainText('Confirmed.', { timeout: 15_000 })
  expect(confirmCalls).toBe(1)
})
