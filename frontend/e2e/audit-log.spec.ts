import { test, expect, type Page, type Route } from '@playwright/test'
import { createAITeam, signIn } from './helpers'
import { e2eBootstrapToken } from './constants'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || `http://127.0.0.1:${process.env.PLAYWRIGHT_SERVER_PORT || '18080'}`
const browserErrors = new Map<Page, string[]>()
test.beforeEach(({ page }) => {
  browserErrors.set(page, [])
  page.on('pageerror', (error) => browserErrors.get(page)!.push(error.message))
})
test.afterEach(({ page }) => {
  expect(browserErrors.get(page)).toEqual([])
  browserErrors.delete(page)
})

const event = (team: string, summary = 'Activity one', action = 'team.update') => ({
  id: `${team}-${summary}`, team_id: team, actor_user_id: 'system', actor_name: '', actor_email: '',
  actor_kind: 'system', action, target_type: 'post', target_id: 'post-123', summary,
  metadata: { channel: 'scheduler', job_id: 'job-123', outcome: 'success', api_key: 'never-show-secret' },
  created_at: '2026-06-01T12:00:00Z',
})
const reply = (route: Route, entries: ReturnType<typeof event>[], total = entries.length) =>
  route.fulfill({ json: { entries, total } })

async function openAudit(page: Page, team: string) {
  await signIn(page)
  await page.goto(`/?team=${team}`)
  await page.getByRole('button', { name: 'Team', exact: true }).click()
  return page.locator('section.brand-card').filter({ has: page.getByRole('heading', { name: 'Audit log', exact: true }) })
}

test('query failure stays distinct from empty activity and can be retried', async ({ page }) => {
  const team = await createAITeam(baseURL, e2eBootstrapToken())
  let failed = true
  await page.route(`**/v1/teams/${team}/audit-log*`, (route) => failed
    ? route.fulfill({ status: 500, json: { error: 'internal_error' } }) : reply(route, []))
  const card = await openAudit(page, team)
  await expect(card.getByRole('alert')).toHaveText('Could not load activity.')
  await expect(card.getByText('No activity recorded yet.')).toHaveCount(0)
  failed = false
  await card.getByRole('button', { name: 'Refresh activity' }).click()
  await expect(card.getByText('No activity recorded yet.')).toBeVisible()
})

test('filter resets page; empty out-of-range pages retain navigation', async ({ page }) => {
  const team = await createAITeam(baseURL, e2eBootstrapToken())
  const queries: URL[] = []
  await page.route(`**/v1/teams/${team}/audit-log*`, (route) => {
    const url = new URL(route.request().url())
    queries.push(url)
    return reply(route, url.searchParams.get('offset') === '25' ? [] : [event(team)], 26)
  })
  const card = await openAudit(page, team)
  await expect(card.getByText('Activity one')).toBeVisible()
  await card.getByRole('button', { name: 'Next', exact: true }).click()
  await expect(card.getByRole('button', { name: 'Previous', exact: true })).toBeEnabled()
  await expect(card.getByText('No activity on this page.')).toBeVisible()
  await card.getByLabel('Filter by action').fill('knowledge_source.create')
  await expect.poll(() => queries.at(-1)?.searchParams.get('action')).toBe('knowledge_source.create')
  expect(queries.at(-1)?.searchParams.get('offset')).toBe('0')
})

test('team switch resets filter and page and ignores delayed previous-team response', async ({ page }) => {
  const team = await createAITeam(baseURL, e2eBootstrapToken())
  const second = await createAITeam(baseURL, e2eBootstrapToken())
  let release: (() => void) | undefined
  await page.route(`**/v1/teams/${team}/audit-log*`, async (route) => {
    const delayed = new URL(route.request().url()).searchParams.get('offset') === '25'
    if (delayed) await new Promise<void>((resolve) => { release = resolve })
    await reply(route, [event(team, delayed ? 'STALE TEAM DATA' : 'First team')], 26)
  })
  const secondQueries: URL[] = []
  await page.route(`**/v1/teams/${second}/audit-log*`, (route) => {
    secondQueries.push(new URL(route.request().url()))
    return reply(route, [event(second, 'Second team')])
  })
  const card = await openAudit(page, team)
  await expect(card.getByText('First team')).toBeVisible()
  await card.getByLabel('Filter by action').fill('team.update')
  await expect(card.getByText('First team')).toBeVisible()
  await card.getByRole('button', { name: 'Next', exact: true }).click()
  await expect.poll(() => Boolean(release)).toBe(true)
  const teamsResponse = await page.request.get('/v1/teams', { headers: { Authorization: `Bearer ${e2eBootstrapToken()}` } })
  const teams = await teamsResponse.json() as { items: { id: string; name: string }[] }
  await page.locator('.sidebar-team-selector').click()
  await page.getByRole('menuitem', { name: teams.items.find((t) => t.id === second)!.name, exact: true }).click()
  await expect(card.getByText('Second team')).toBeVisible()
  const oldResponse = page.waitForResponse((response) => response.url().includes(`/teams/${team}/audit-log`) && new URL(response.url()).searchParams.get('offset') === '25')
  release!()
  await (await oldResponse).finished()
  await expect(card.getByText('Second team')).toBeVisible()
  await expect(card.getByText('STALE TEAM DATA')).toHaveCount(0)
  await expect(card.getByLabel('Filter by action')).toHaveValue('')
  expect(secondQueries.at(-1)?.searchParams.get('offset')).toBe('0')
  expect(secondQueries.at(-1)?.searchParams.has('action')).toBe(false)
})

test('successful mutation immediately refreshes activity', async ({ page }) => {
  const team = await createAITeam(baseURL, e2eBootstrapToken())
  let calls = 0
  await page.route(`**/v1/teams/${team}/audit-log*`, (route) => reply(route, [event(team, `Revision ${++calls}`)]))
  const card = await openAudit(page, team)
  await expect(card.getByText('Revision 1')).toBeVisible()
  const before = calls
  await page.getByLabel('Team name', { exact: true }).fill(`Audit updated ${Date.now()}`)
  await page.locator('section.brand-card').filter({ has: page.getByLabel('Team name', { exact: true }) }).getByRole('button', { name: 'Save Changes', exact: true }).click()
  await expect.poll(() => calls).toBeGreaterThan(before)
  await expect(card.getByText(`Revision ${calls}`)).toBeVisible()
})

test('polls new activity, shows safe automation details and filters historical actions', async ({ page }) => {
  const team = await createAITeam(baseURL, e2eBootstrapToken())
  let calls = 0
  let action = ''
  await page.route(`**/v1/teams/${team}/audit-log*`, (route) => {
    calls++
    action = new URL(route.request().url()).searchParams.get('action') || ''
    return reply(route, [event(team, `Background ${calls}`, 'historical.custom_action')])
  })
  await page.clock.install()
  const card = await openAudit(page, team)
  await expect(card.getByText('Background 1')).toBeVisible()
  await expect(card.getByText('Automation', { exact: true }).first()).toBeVisible()
  await expect(card.getByText('post: post-123')).toBeVisible()
  await expect(card.getByText('job_id: job-123')).toBeVisible()
  await expect(card.getByText('never-show-secret')).toHaveCount(0)
  await expect(card.getByText('historical.custom_action', { exact: true })).toBeVisible()
  await card.getByLabel('Filter by action').fill('historical.custom_action')
  await expect.poll(() => action).toBe('historical.custom_action')
  const before = calls
  await page.clock.fastForward(30_001)
  await expect.poll(() => calls).toBeGreaterThan(before)
})


test('supported action labels render in English and German', async ({ page }) => {
  const team = await createAITeam(baseURL, e2eBootstrapToken())
  await page.route(`**/v1/teams/${team}/audit-log*`, (route) => reply(route, [event(team, 'Knowledge added', 'knowledge_source.create')]))
  const card = await openAudit(page, team)
  await expect(card.getByRole('cell', { name: 'Knowledge source created', exact: true })).toBeVisible()
  expect(await page.locator('#audit-actions option').evaluateAll((options) => options.filter((option) => option.textContent === option.getAttribute('value')).map((option) => option.textContent))).toEqual([])
  await page.evaluate(() => {
    const settings = JSON.parse(localStorage.getItem('goloom-ui-settings') || '{}')
    settings.ui = { ...settings.ui, language: 'de' }
    localStorage.setItem('goloom-ui-settings', JSON.stringify(settings))
  })
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Auditlog', exact: true })).toBeVisible()
  await expect(page.getByRole('cell', { name: 'Wissensquelle erstellt', exact: true })).toBeVisible()
  expect(await page.locator('#audit-actions option').evaluateAll((options) => options.filter((option) => option.textContent === option.getAttribute('value')).map((option) => option.textContent))).toEqual([])
})
