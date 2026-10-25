import { test, expect, type Page } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { getFirstTeamId, signIn } from './helpers'

const POST_ID = 'thread-post'
const ACCOUNT_ID = 'thread-acct'

async function openPostedPreview(page: Page, baseURL: string, opts: {
  provider?: string
  secondPostCalls?: string[]
  thread: (route: import('@playwright/test').Route) => Promise<void> | void
}) {
  const teamId = await getFirstTeamId(baseURL, e2eBootstrapToken())
  await page.route(`**/v1/teams/${teamId}/accounts`, (route) => route.fulfill({ json: { items: [{
    id: ACCOUNT_ID, team_id: teamId, provider: opts.provider ?? 'mastodon', auth_type: 'oauth_token',
    instance_url: 'https://social.example', username: 'me@social.example', remote_account_id: 'me',
    created_at: '2026-01-01T00:00:00Z',
  }] } }))
  const mkPost = (id: string, title: string, n: number) => ({
    id, team_id: teamId, author_user_id: 'u1', title, content: `${title} body`,
    scheduled_at: '2026-01-02T09:00:00Z', status: 'posted', attempt_count: 1,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-02T09:00:00Z',
    target_accounts: [ACCOUNT_ID], published_links: { [ACCOUNT_ID]: `https://social.example/@me/${n}` },
  })
  const posts = [{ ...mkPost(POST_ID, 'Thread post', 1), content: 'Hello world' }]
  if (opts.secondPostCalls) posts.push(mkPost('thread-post-b', 'Other post', 2))
  await page.route(`**/v1/teams/${teamId}/posts`, (route) => route.fulfill({ json: { items: posts } }))
  if (opts.secondPostCalls) {
    const bCalls = opts.secondPostCalls
    await page.route(`**/v1/teams/${teamId}/posts/thread-post-b/targets/${ACCOUNT_ID}/thread`, (route) => {
      bCalls.push(route.request().url())
      return route.fulfill({ json: { post_id: 'thread-post-b', account_id: ACCOUNT_ID, provider: 'mastodon', fetched_at: '2026-01-02T12:30:00Z', replies: [] } })
    })
  }
  const threadCalls: string[] = []
  await page.route(`**/v1/teams/${teamId}/posts/${POST_ID}/targets/${ACCOUNT_ID}/thread`, async (route) => {
    threadCalls.push(route.request().url())
    await opts.thread(route)
  })
  await signIn(page)
  await page.goto(`/?team=${teamId}`)
  await page.getByRole('button', { name: 'Archive', exact: true }).click()
  await page.getByRole('article').filter({ hasText: 'Thread post' }).click()
  return threadCalls
}

const replies = [
  { remote_id: 'r1', parent_id: 'root', url: 'https://social.example/@alice/10', content: 'First reply\nsecond line <b>not bold</b>',
    published_at: '2026-01-02T10:00:00Z', author: { handle: 'alice@social.example', display_name: 'Alice', avatar_url: 'http://127.0.0.1:1/broken.png' } },
  { remote_id: 'r2', parent_id: 'r1', url: 'https://social.example/@bob/11', content: 'Nested reply',
    published_at: '2026-01-02T11:00:00Z', author: { handle: 'bob@social.example', display_name: 'Bob' } },
  { remote_id: 'r3', parent_id: 'unknown', url: 'https://social.example/@carol/12', content: 'Orphan reply',
    published_at: '', author: { handle: 'carol@social.example' } },
]

test('replies load lazily and render nested', async ({ page, baseURL }) => {
  const calls = await openPostedPreview(page, baseURL!, {
    thread: (route) => route.fulfill({ json: { post_id: POST_ID, account_id: ACCOUNT_ID, provider: 'mastodon',
      published_url: 'https://social.example/@me/1', fetched_at: '2026-01-02T12:30:00Z', replies } }),
  })
  const toggle = page.getByRole('button', { name: 'Show replies' })
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await page.waitForTimeout(500)
  expect(calls).toHaveLength(0)

  await toggle.click()
  await expect(page.getByRole('button', { name: 'Hide replies' })).toHaveAttribute('aria-expanded', 'true')
  const items = page.getByTestId('thread-reply')
  await expect(items).toHaveCount(3)
  await expect(items.first()).toContainText('Alice')
  await expect(items.first()).toContainText('alice@social.example')
  await expect(items.first().locator('.post-thread__content').first()).toHaveText(/First reply\nsecond line <b>not bold<\/b>/)
  // Nested: Bob lives inside Alice's item
  await expect(items.first().locator('.post-thread__list--nested').getByTestId('thread-reply')).toContainText('Bob')
  // Broken avatar falls back to initials
  await expect(items.first().locator('.post-thread__avatar').first()).toHaveText('AL')
  const link = items.first().getByRole('link', { name: /Open reply/ }).first()
  await expect(link).toHaveAttribute('href', 'https://social.example/@alice/10')
  await expect(link).toHaveAttribute('target', '_blank')
  await expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  expect(calls).toHaveLength(1)

  // collapse + re-expand within staleTime does not refetch; manual refresh does
  await page.getByRole('button', { name: 'Hide replies' }).click()
  await page.getByRole('button', { name: 'Show replies' }).click()
  await expect(items).toHaveCount(3)
  expect(calls).toHaveLength(1)
  await page.getByRole('button', { name: 'Refresh replies' }).click()
  await expect.poll(() => calls.length).toBe(2)
})

test('empty thread shows empty state', async ({ page, baseURL }) => {
  await openPostedPreview(page, baseURL!, {
    thread: (route) => route.fulfill({ json: { post_id: POST_ID, account_id: ACCOUNT_ID, provider: 'mastodon', fetched_at: '2026-01-02T12:30:00Z', replies: [] } }),
  })
  await page.getByRole('button', { name: 'Show replies' }).click()
  await expect(page.getByTestId('thread-empty')).toHaveText('No replies yet.')
})

test('provider failure (502) shows error', async ({ page, baseURL }) => {
  await openPostedPreview(page, baseURL!, { thread: (route) => route.fulfill({ status: 502, body: 'bad gateway' }) })
  await page.getByRole('button', { name: 'Show replies' }).click()
  await expect(page.getByTestId('thread-error')).toBeVisible()
})

test('unsupported provider (422) shows hint without retrying', async ({ page, baseURL }) => {
  const calls = await openPostedPreview(page, baseURL!, {
    thread: (route) => route.fulfill({ status: 422, headers: { 'X-Error-Code': 'thread_unsupported' }, body: 'thread_unsupported' }),
  })
  await page.getByRole('button', { name: 'Show replies' }).click()
  await expect(page.getByTestId('thread-unsupported')).toHaveText('Replies are not supported for this network.')
  await page.waitForTimeout(1500)
  expect(calls).toHaveLength(1)
})

test('no toggle for providers without thread support', async ({ page, baseURL }) => {
  await openPostedPreview(page, baseURL!, { provider: 'bluesky', thread: (route) => route.fulfill({ status: 422, body: '' }) })
  await expect(page.getByTestId('live-preview-title')).toHaveText('Thread post')
  await expect(page.getByRole('button', { name: 'Show replies' })).toHaveCount(0)
})

test('truncated thread shows hint with link to the original post', async ({ page, baseURL }) => {
  await openPostedPreview(page, baseURL!, {
    thread: (route) => route.fulfill({ json: { post_id: POST_ID, account_id: ACCOUNT_ID, provider: 'mastodon',
      published_url: 'https://social.example/@me/1', fetched_at: '2026-01-02T12:30:00Z', truncated: true, replies: replies.slice(0, 1) } }),
  })
  await page.getByRole('button', { name: 'Show replies' }).click()
  const hint = page.getByTestId('thread-truncated')
  await expect(hint).toContainText('Only the first 200 replies are shown.')
  await expect(hint.getByRole('link')).toHaveAttribute('href', 'https://social.example/@me/1')
})

test('not truncated shows no hint', async ({ page, baseURL }) => {
  await openPostedPreview(page, baseURL!, {
    thread: (route) => route.fulfill({ json: { post_id: POST_ID, account_id: ACCOUNT_ID, provider: 'mastodon',
      fetched_at: '2026-01-02T12:30:00Z', truncated: false, replies: replies.slice(0, 1) } }),
  })
  await page.getByRole('button', { name: 'Show replies' }).click()
  await expect(page.getByTestId('thread-reply')).toHaveCount(1)
  await expect(page.getByTestId('thread-truncated')).toHaveCount(0)
})

test('content warnings collapse replies until revealed', async ({ page, baseURL }) => {
  const author = { handle: 'dan@social.example', display_name: 'Dan' }
  await openPostedPreview(page, baseURL!, {
    thread: (route) => route.fulfill({ json: { post_id: POST_ID, account_id: ACCOUNT_ID, provider: 'mastodon', fetched_at: '2026-01-02T12:30:00Z',
      replies: [
        { remote_id: 'c1', parent_id: 'root', url: '', content: 'Hidden spoiler text', content_warning: 'Movie spoilers', sensitive: true, published_at: '2026-01-02T10:00:00Z', author },
        { remote_id: 'c2', parent_id: 'root', url: '', content: 'Sensitive body', sensitive: true, published_at: '2026-01-02T10:05:00Z', author },
        { remote_id: 'c3', parent_id: 'root', url: '', content: 'Plain body', published_at: '2026-01-02T10:10:00Z', author },
      ] } }),
  })
  await page.getByRole('button', { name: 'Show replies' }).click()
  const items = page.getByTestId('thread-reply')
  await expect(items).toHaveCount(3)

  const first = items.nth(0)
  await expect(first.getByTestId('thread-cw')).toHaveText('Movie spoilers')
  await expect(first).not.toContainText('Hidden spoiler text')
  const toggle = first.getByRole('button', { name: 'Show', exact: true })
  await expect(toggle).toHaveAttribute('aria-expanded', 'false')
  await toggle.click()
  await expect(first).toContainText('Hidden spoiler text')
  const hide = first.getByRole('button', { name: 'Hide', exact: true })
  await expect(hide).toHaveAttribute('aria-expanded', 'true')
  await hide.click()
  await expect(first).not.toContainText('Hidden spoiler text')

  // sensitive without CW text uses the generic label
  const second = items.nth(1)
  await expect(second.getByTestId('thread-cw')).toHaveText('Sensitive content')
  await expect(second).not.toContainText('Sensitive body')
  // plain reply unaffected
  await expect(items.nth(2)).toContainText('Plain body')
  await expect(items.nth(2).getByTestId('thread-cw')).toHaveCount(0)
})

test('switching posts resets the thread toggle and does not fetch', async ({ page, baseURL }) => {
  const bCalls: string[] = []
  const calls = await openPostedPreview(page, baseURL!, {
    secondPostCalls: bCalls,
    thread: (route) => route.fulfill({ json: { post_id: POST_ID, account_id: ACCOUNT_ID, provider: 'mastodon', fetched_at: '2026-01-02T12:30:00Z', replies: replies.slice(0, 1) } }),
  })
  await page.getByRole('button', { name: 'Show replies' }).click()
  await expect(page.getByTestId('thread-reply')).toHaveCount(1)
  expect(calls).toHaveLength(1)

  await page.getByRole('article').filter({ hasText: 'Other post' }).click()
  await expect(page.getByTestId('live-preview-title')).toHaveText('Other post')
  await expect(page.getByRole('button', { name: 'Show replies' })).toHaveAttribute('aria-expanded', 'false')
  await page.waitForTimeout(500)
  expect(bCalls).toHaveLength(0)
})
