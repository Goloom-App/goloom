import { expect, type Page } from '@playwright/test'
import { e2eBootstrapToken } from './constants'

export async function signIn(page: Page) {
  await page.goto('/')
  await page.getByLabel(/access token|administrator token/i).fill(e2eBootstrapToken())
  await page.getByRole('button', { name: 'Sign in with token' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })
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
