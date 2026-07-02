import type { FullConfig } from '@playwright/test'
import { e2eBootstrapToken, E2E_SEEDED_POST_TITLE } from './constants'

async function listPosts(baseURL: string, token: string, teamId: string): Promise<{ items?: { title?: string }[] }> {
  const res = await fetch(`${baseURL}/v1/teams/${teamId}/posts`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    throw new Error(`list posts ${res.status}: ${await res.text()}`)
  }
  return (await res.json()) as { items?: { title?: string }[] }
}

export default async function globalSetup(config: FullConfig) {
  const baseURL = config.projects[0].use.baseURL
  if (!baseURL) {
    throw new Error('globalSetup: baseURL missing')
  }
  const token = e2eBootstrapToken()

  const teamsRes = await fetch(`${baseURL}/v1/teams`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!teamsRes.ok) {
    throw new Error(`globalSetup teams ${teamsRes.status}: ${await teamsRes.text()}`)
  }
  const teamsData = (await teamsRes.json()) as { items?: { id: string }[] }
  let teamId = teamsData.items?.[0]?.id
  if (!teamId) {
    // Fresh databases start without any team (users create their first team
    // through onboarding); seed one for the specs. Teams are listed by name and
    // the app defaults to the first one, so this name must sort before every
    // team the specs create ("E2E AI Test …", "E2E Invite Team …", …).
    const createRes = await fetch(`${baseURL}/v1/teams`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: 'E2E AA Seed Team', description: 'Seeded by Playwright global setup' }),
    })
    if (!createRes.ok) {
      throw new Error(`globalSetup create team ${createRes.status}: ${await createRes.text()}`)
    }
    teamId = ((await createRes.json()) as { id: string }).id
  }

  const existing = await listPosts(baseURL, token, teamId)
  if (existing.items?.some((p) => p.title === E2E_SEEDED_POST_TITLE)) {
    return
  }

  const scheduled = new Date()
  scheduled.setUTCDate(scheduled.getUTCDate() + 1)
  scheduled.setUTCHours(15, 30, 0, 0)

  const body = JSON.stringify({
    title: E2E_SEEDED_POST_TITLE,
    content: 'Seeded draft for Playwright E2E.',
    scheduled_at: scheduled.toISOString(),
    target_accounts: [] as string[],
    draft: true,
  })

  const postRes = await fetch(`${baseURL}/v1/teams/${teamId}/posts`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body,
  })
  if (!postRes.ok) {
    throw new Error(`globalSetup create post ${postRes.status}: ${await postRes.text()}`)
  }
}
