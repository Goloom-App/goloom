import { expect, type Page } from '@playwright/test'
import { e2eBootstrapToken, E2E_REVIEW_POST_TITLE } from './constants'

export async function signIn(page: Page) {
  await page.goto('/')
  const tokenField = page.getByLabel(/access token|administrator token/i)
  await expect(tokenField).toBeVisible({ timeout: 30_000 })
  await tokenField.fill(e2eBootstrapToken())

  const signInBtn = page.getByRole('button', { name: 'Sign in with token' })
  const dashboard = page.getByRole('heading', { level: 1, name: 'Dashboard' })

  await signInBtn.click()
  try {
    await expect(dashboard).toBeVisible({ timeout: 20_000 })
  } catch {
    // On a loaded CI runner the first submit occasionally does not take; if the
    // sign-in form is still shown, submit once more before failing. Makes the
    // shared sign-in robust for every spec instead of relying on the job retry.
    if (await signInBtn.isVisible().catch(() => false)) {
      await tokenField.fill(e2eBootstrapToken())
      await signInBtn.click()
    }
    // A 401 race during session establishment can bounce the app through
    // clearAuthenticatedState, which parks the (re-)authenticated session on
    // the calendar section. If the shell is up but we are not on the
    // dashboard, navigate home instead of failing the whole spec.
    const homeButton = page.getByRole('button', { name: 'Home' })
    if (!(await dashboard.isVisible().catch(() => false)) && (await homeButton.isVisible().catch(() => false))) {
      await homeButton.click()
    }
    await expect(dashboard).toBeVisible({ timeout: 30_000 })
  }
  await expect(page.getByText(/too many requests|rate limit/i)).toHaveCount(0, { timeout: 30_000 })
  await expect(page.getByRole('button', { name: /select team/i })).toHaveCount(0, { timeout: 30_000 })
}

export async function openContentCalendar(page: Page) {
  await page.getByRole('button', { name: 'Calendar', exact: true }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()
  await page.getByRole('button', { name: 'List', exact: true }).click()
}

export async function openSeededPostPreview(page: Page) {
  const seededCard = page.getByRole('article').filter({ hasText: 'E2E Draft Post' })
  await expect(seededCard).toBeVisible({ timeout: 15_000 })
  await seededCard.click()
}

async function apiFetch(url: string, opts: RequestInit, retries = 3): Promise<Response> {
  for (let attempt = 1; attempt <= retries; attempt++) {
    const res = await fetch(url, opts)
    if (res.ok || res.status !== 500 || attempt === retries) return res
    // SQLITE_BUSY — wait and retry
    await new Promise((r) => setTimeout(r, 2000 * attempt))
  }
  throw new Error(`apiFetch exhausted ${retries} retries`)
}

let teamCounter = 0
const runId = Date.now().toString(36)

export async function getFirstTeamId(baseURL: string, token: string): Promise<string> {
  const res = await apiFetch(`${baseURL}/v1/teams`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    throw new Error(`list teams ${res.status}: ${await res.text()}`)
  }
  const data = (await res.json()) as { items?: { id: string }[] }
  const teamId = data.items?.[0]?.id
  if (!teamId) {
    throw new Error('no team returned')
  }
  return teamId
}

export async function seedAutomationReviewDraft(
  baseURL: string,
  token: string,
  teamId: string,
  title = E2E_REVIEW_POST_TITLE,
  content = 'Automation draft waiting for review.',
  targetAccounts: string[] = [],
) {
  const listRes = await apiFetch(`${baseURL}/v1/teams/${teamId}/review-queue`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!listRes.ok) {
    throw new Error(`list review queue ${listRes.status}: ${await listRes.text()}`)
  }
  const existing = (await listRes.json()) as { items?: { title?: string }[] }
  if (existing.items?.some((item) => item.title === title)) {
    return
  }

  const scheduled = new Date()
  scheduled.setUTCHours(scheduled.getUTCHours() - 4)

  const seedRes = await apiFetch(`${baseURL}/v1/admin/e2e/automation-draft`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({
      team_id: teamId,
      title,
      content,
      target_accounts: targetAccounts,
      scheduled_at: scheduled.toISOString(),
    }),
  })
  if (!seedRes.ok) {
    throw new Error(`seed automation draft ${seedRes.status}: ${await seedRes.text()}`)
  }
}

export async function openReviewQueue(page: import('@playwright/test').Page) {
  await page.getByTestId('nav-review-queue').click()
  await expect(page.getByTestId('review-queue')).toBeVisible()
}

export async function getReviewQueueItemId(baseURL: string, token: string, teamId: string, title: string): Promise<string> {
  const res = await apiFetch(`${baseURL}/v1/teams/${teamId}/review-queue`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    throw new Error(`list review queue ${res.status}: ${await res.text()}`)
  }
  const data = (await res.json()) as { items?: { id: string; title?: string }[] }
  const item = data.items?.find((it) => it.title === title)
  if (!item) {
    throw new Error(`review queue item not found: ${title}`)
  }
  return item.id
}

export async function getPost(baseURL: string, token: string, teamId: string, postId: string): Promise<{
  id: string
  title: string
  content: string
  status: string
}> {
  const res = await apiFetch(`${baseURL}/v1/teams/${teamId}/posts/${postId}`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    throw new Error(`get post ${postId} ${res.status}: ${await res.text()}`)
  }
  return (await res.json()) as { id: string; title: string; content: string; status: string }
}

export async function getFirstAccountId(baseURL: string, token: string, teamId: string): Promise<string> {
  const res = await apiFetch(`${baseURL}/v1/teams/${teamId}/accounts`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    throw new Error(`list accounts ${res.status}: ${await res.text()}`)
  }
  const data = (await res.json()) as { items?: { id: string }[] }
  const accountId = data.items?.[0]?.id
  if (!accountId) {
    throw new Error('no account returned')
  }
  return accountId
}

export async function ensureE2EAccount(baseURL: string, token: string, teamId: string): Promise<string> {
  const res = await apiFetch(`${baseURL}/v1/teams/${teamId}/accounts`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    throw new Error(`list accounts ${res.status}: ${await res.text()}`)
  }
  const data = (await res.json()) as { items?: { id: string }[] }
  const existing = data.items?.[0]?.id
  if (existing) {
    return existing
  }
  const seedRes = await apiFetch(`${baseURL}/v1/admin/e2e/account`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ team_id: teamId }),
  })
  if (!seedRes.ok) {
    throw new Error(`seed e2e account ${seedRes.status}: ${await seedRes.text()}`)
  }
  const account = (await seedRes.json()) as { id: string }
  return account.id
}

export async function listReviewQueue(baseURL: string, token: string, teamId: string): Promise<{ id: string; title?: string }[]> {
  const res = await apiFetch(`${baseURL}/v1/teams/${teamId}/review-queue`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    throw new Error(`list review queue ${res.status}: ${await res.text()}`)
  }
  const data = (await res.json()) as { items?: { id: string; title?: string }[] }
  return data.items ?? []
}

export async function patchPostContent(
  baseURL: string,
  token: string,
  teamId: string,
  postId: string,
  content: string,
): Promise<void> {
  const res = await apiFetch(`${baseURL}/v1/teams/${teamId}/posts/${postId}`, {
    method: 'PATCH',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
  if (!res.ok) {
    throw new Error(`patch post ${postId} ${res.status}: ${await res.text()}`)
  }
}

export async function createAITeam(baseURL: string, token: string): Promise<string> {
  teamCounter++
  const teamName = `E2E AI Test ${runId}-${teamCounter}`
  const createRes = await apiFetch(`${baseURL}/v1/teams`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: teamName, description: 'Created by E2E test for AI views' }),
  })
  if (!createRes.ok) {
    throw new Error(`create team ${createRes.status}: ${await createRes.text()}`)
  }
  const team = (await createRes.json()) as { id: string }
  const patchRes = await apiFetch(`${baseURL}/v1/teams/${team.id}`, {
    method: 'PATCH',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: teamName, is_ai_enabled: true }),
  })
  if (!patchRes.ok) {
    throw new Error(`enable AI ${patchRes.status}: ${await patchRes.text()}`)
  }
  return team.id
}
