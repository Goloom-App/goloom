import { test, expect, type Page, type Route } from '@playwright/test'
import { createAITeam, getFirstTeamId, signIn } from './helpers'
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
  metadata: { channel: 'scheduler', job_id: 'job-123', outcome: 'success', removed_count: '2', status_before: 'failed', status_after: 'scheduled', author_user_id: 'author-123', attempt_id: 'attempt-123', api_key: 'never-show-secret' },
  created_at: '2026-06-01T12:00:00Z',
})
const reply = (route: Route, entries: ReturnType<typeof event>[], total = entries.length) =>
  route.fulfill({ json: { entries, total } })

async function openAudit(page: Page, team: string) {
  await signIn(page)
  await page.goto(`/?team=${team}`)
  await page.getByRole('button', { name: 'Team', exact: true }).click()
  await page.getByRole('tab', { name: 'Audit log', exact: true }).click()
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

test('returning to audit after a settings mutation loads fresh activity', async ({ page }) => {
  const defaultTeam = await getFirstTeamId(baseURL, e2eBootstrapToken())
  const team = await createAITeam(baseURL, e2eBootstrapToken())
  let calls = 0
  await page.route(`**/v1/teams/${team}/audit-log*`, (route) => reply(route, [event(team, `Revision ${++calls}`)]))
  const card = await openAudit(page, team)
  await expect(card.getByText('Revision 1')).toBeVisible()
  const before = calls
  await page.getByRole('tab', { name: 'General', exact: true }).click()
  await page.getByLabel('Team name', { exact: true }).fill(`E2E AI Updated ${Date.now()}`)
  await page.locator('section.brand-card').filter({ has: page.getByLabel('Team name', { exact: true }) }).getByRole('button', { name: 'Save Changes', exact: true }).click()
  await page.getByRole('tab', { name: 'Audit log', exact: true }).click()
  await expect.poll(() => calls).toBeGreaterThan(before)
  await expect(card.getByText(`Revision ${calls}`)).toBeVisible()
  // Shared specs sign into the first team; a renamed fixture must not displace it.
  expect(await getFirstTeamId(baseURL, e2eBootstrapToken())).toBe(defaultTeam)
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
  await card.getByRole('button', { name: 'Show details', exact: true }).click()
  await expect(card.getByText('post: post-123')).toBeVisible()
  await expect(card.getByText('job_id: job-123')).toBeVisible()
  await expect(card.getByText('removed_count: 2')).toBeVisible()
  await expect(card.getByText('status_before: failed')).toBeVisible()
  await expect(card.getByText('status_after: scheduled')).toBeVisible()
  await expect(card.getByText('author_user_id: author-123')).toBeVisible()
  await expect(card.getByText('attempt_id: attempt-123')).toBeVisible()
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
  await page.getByRole('tab', { name: 'Audit-Log', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Auditlog', exact: true })).toBeVisible()
  await expect(page.getByRole('cell', { name: 'Wissensquelle erstellt', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Details anzeigen', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Details ausblenden', exact: true })).toHaveAttribute('aria-expanded', 'true')
  expect(await page.locator('#audit-actions option').evaluateAll((options) => options.filter((option) => option.textContent === option.getAttribute('value')).map((option) => option.textContent))).toEqual([])
})

test('details collapse per entry, support keyboard and wrap long values on desktop and mobile', async ({ page }) => {
  const team = await createAITeam(baseURL, e2eBootstrapToken())
  const longValue = '01234567-89ab-cdef-0123-456789abcdef'.repeat(12)
  await page.route(`**/v1/teams/${team}/audit-log*`, (route) => route.fulfill({ json: {
    entries: [
      { ...event(team, 'Updated recurring template', 'recurring.update'), metadata: { changed_fields: 'ai_enhance_enabled,announcement_enabled,content', request_id: longValue, api_key: 'never-show-secret' } },
      { ...event(team, '', 'ai_job.complete'), target_id: longValue },
    ], total: 2,
  } }))
  await page.setViewportSize({ width: 1440, height: 1000 })
  const card = await openAudit(page, team)
  const rows = card.locator('tbody tr')
  const first = rows.nth(0)
  const second = rows.nth(1)
  await expect(first.getByText('Updated recurring template', { exact: true })).toBeVisible()
  await expect(first.getByRole('button', { name: 'Show details', exact: true })).toHaveAttribute('aria-expanded', 'false')
  await expect(second.getByRole('button', { name: 'Show details', exact: true })).toHaveAttribute('aria-expanded', 'false')
  await expect(card.getByText(`request_id: ${longValue}`, { exact: true })).toHaveCount(0)
  // A missing summary still has a short, single-line target teaser.
  await expect(second.locator('.audit-details__summary')).toBeVisible()
  for (const label of ['Recurring template updated', 'AI generation completed']) {
    const cell = card.getByRole('cell', { name: label, exact: true })
    expect(await cell.evaluate((el) => {
      const range = document.createRange()
      range.selectNodeContents(el)
      return range.getBoundingClientRect().height / parseFloat(getComputedStyle(el).lineHeight)
    })).toBeLessThanOrEqual(2.1)
  }
  await first.getByRole('button', { name: 'Show details', exact: true }).click()
  await expect(first.getByRole('button', { name: 'Hide details', exact: true })).toHaveAttribute('aria-expanded', 'true')
  await expect(first.getByText('changed_fields: ai_enhance_enabled, announcement_enabled, content', { exact: true })).toBeVisible()
  await expect(first.getByText(`request_id: ${longValue}`, { exact: true })).toBeVisible()
  await expect(second.getByRole('button', { name: 'Show details', exact: true })).toHaveAttribute('aria-expanded', 'false')
  await expect(card.getByText('never-show-secret')).toHaveCount(0)
  const toggle = second.getByRole('button', { name: 'Show details', exact: true })
  await toggle.focus()
  await page.keyboard.press('Enter')
  await expect(second.getByRole('button', { name: 'Hide details', exact: true })).toHaveAttribute('aria-expanded', 'true')
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await card.locator('table').evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
    const value = first.getByText(`request_id: ${longValue}`, { exact: true })
    expect(await value.evaluate((el) => el.scrollWidth <= el.clientWidth + 1)).toBe(true)
  }
  await second.getByRole('button', { name: 'Hide details', exact: true }).focus()
  await page.keyboard.press('Space')
  await expect(second.getByRole('button', { name: 'Show details', exact: true })).toHaveAttribute('aria-expanded', 'false')
  await expect(first.getByText(`request_id: ${longValue}`, { exact: true })).toBeVisible()
  await first.getByRole('button', { name: 'Hide details', exact: true }).click()
  await expect(first.getByText(`request_id: ${longValue}`, { exact: true })).toHaveCount(0)
})

test('OIDC actors retain human member attribution', async ({ page }) => {
  const team = await createAITeam(baseURL, e2eBootstrapToken())
  await page.route(`**/v1/teams/${team}/audit-log*`, (route) => route.fulfill({ json: {
    entries: [{ ...event(team), actor_kind: 'oidc', actor_user_id: 'human-123', actor_name: 'Test Member', actor_email: 'member@example.test' }], total: 1,
  } }))
  const card = await openAudit(page, team)
  await expect(card.getByText('Test Member', { exact: true })).toBeVisible()
  await expect(card.getByText('Member', { exact: true })).toBeVisible()
  await expect(card.getByText('member@example.test', { exact: true })).toBeVisible()
  await expect(card.getByText('Unknown', { exact: true })).toHaveCount(0)
})
