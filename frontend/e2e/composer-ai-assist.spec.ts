import { test, expect, type Page } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { getFirstTeamId, openContentCalendar, openSeededPostPreview, signIn } from './helpers'

const DEFAULT_PLACEHOLDER = "What's on your mind?"
const OVERRIDE_PLACEHOLDER = 'Leave aligned with default, or type a custom version for this account.'

const MASTODON_ACCOUNT_ID = 'acc-mastodon'
const BLUESKY_ACCOUNT_ID = 'acc-bluesky'
const MASTODON_USERNAME = 'masto-user'
const BLUESKY_USERNAME = 'bsky-user'

type ComposerAIResult = {
  content: string
  /** null marks an explicitly proposed override removal. */
  accountContentOverride: Record<string, string | null>
}

const TEAM_ID = 'mock-ai-team'
const OTHER_TEAM_ID = 'mock-other-team'

type TeamItem = Record<string, unknown> & { id: string }

/**
 * Serves two extra AI-enabled teams from the browser-side team list only, so the
 * shared E2E database (and every other spec's sidebar) stays untouched. A
 * POST /v1/teams creates a third mocked team and adds it to the list.
 */
async function mockTeams(page: Page) {
  const extra: TeamItem[] = []
  const mockTeam = (id: string, name: string, template: TeamItem): TeamItem => ({
    ...template,
    id,
    name,
    is_ai_enabled: true,
  })
  await page.route('**/v1/teams', async (route) => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as { name: string }
      const real = (await (await route.fetch({ method: 'GET' })).json()) as { items: TeamItem[] }
      const created = mockTeam('mock-created-team', body.name, real.items[0])
      extra.push(created)
      await route.fulfill({ status: 201, json: created })
      return
    }
    const response = await route.fetch()
    const data = (await response.json()) as { items: TeamItem[] }
    const template = data.items[0]
    await route.fulfill({
      response,
      json: {
        ...data,
        items: [
          ...data.items.map((item) => ({ ...item, is_ai_enabled: true })),
          mockTeam(TEAM_ID, 'Mock AI team', template),
          mockTeam(OTHER_TEAM_ID, 'Mock other team', template),
          ...extra,
        ],
      },
    })
  })
}

/** Two destinations with different limits so per-account overrides are exercised. */
async function mockTeamAccounts(page: Page, teamId: string) {
  await page.route(`**/v1/teams/${teamId}/accounts`, (route) =>
    route.fulfill({
      json: {
        items: [
          {
            id: MASTODON_ACCOUNT_ID,
            team_id: teamId,
            provider: 'mastodon',
            auth_type: 'oauth_token',
            instance_url: 'https://mastodon.example',
            username: MASTODON_USERNAME,
            remote_account_id: 'remote-mastodon',
            max_chars_override: 5000,
            created_at: '2026-01-01T00:00:00Z',
          },
          {
            id: BLUESKY_ACCOUNT_ID,
            team_id: teamId,
            provider: 'bluesky',
            auth_type: 'app_password',
            instance_url: 'https://bsky.social',
            username: BLUESKY_USERNAME,
            remote_account_id: 'remote-bluesky',
            max_chars_override: 300,
            created_at: '2026-01-01T00:00:00Z',
          },
        ],
      },
    }),
  )
}

/**
 * Mocks the optimize round trip with a controllable job so a spec can decide
 * when the result lands (and assert what happens when it lands too late).
 */
async function mockComposerAI(page: Page, teamId: string, result: ComposerAIResult) {
  let completed = false
  let triggers = 0

  await page.route(`**/v1/teams/${teamId}/ai-trigger`, (route) => {
    triggers += 1
    return route.fulfill({ status: 202, json: { job_id: 'job-1', status: 'pending' } })
  })

  await page.route(`**/v1/teams/${teamId}/ai-jobs/job-1`, (route) =>
    route.fulfill({
      json: {
        id: 'job-1',
        team_id: teamId,
        author_user_id: 'user-1',
        type: 'voice_engine',
        status: completed ? 'completed' : 'pending',
        payload: {},
        result: completed
          ? { content: result.content, account_content_override: result.accountContentOverride }
          : null,
        error_message: null,
        created_at: '2026-10-17T10:00:00Z',
        updated_at: '2026-10-17T10:00:00Z',
        completed_at: completed ? '2026-10-17T10:00:05Z' : null,
      },
    }),
  )

  return {
    complete: () => {
      completed = true
    },
    triggerCount: () => triggers,
  }
}

async function openComposer(page: Page) {
  await page.locator('.sidebar-composer-cta').click()
  await expect(page.getByTestId('composer-view')).toBeVisible({ timeout: 15_000 })
}

/** Optimize is a two-step action: reveal the instruction field, then run. */
async function startOptimize(page: Page) {
  const optimize = page.getByTestId('composer-ai-optimize')
  await expect(optimize).toBeEnabled()
  await optimize.click()
  await expect(page.getByTestId('composer-ai-instruction')).toBeVisible()
  await optimize.click()
}

test.describe('composer AI assist', () => {
  test('discards a stale AI result when the composer draft changes', async ({ page }) => {
    test.setTimeout(120_000)
    const teamId = TEAM_ID
    await mockTeams(page)
    await mockTeamAccounts(page, teamId)
    const job = await mockComposerAI(page, teamId, {
      content: 'AI refined draft body.',
      accountContentOverride: {},
    })

    await signIn(page)
    await page.goto(`/?team=${teamId}`)
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await page.getByRole('button', { name: 'New Post' }).click()

    const defaultBody = page.getByPlaceholder(DEFAULT_PLACEHOLDER)
    await defaultBody.fill('Original draft while the job runs.')
    await startOptimize(page)
    await expect.poll(job.triggerCount).toBe(1)

    // Start a different draft while the first optimize job is still running.
    await page.locator('.sidebar-composer-cta').click()
    await expect(defaultBody).toHaveValue('')
    await defaultBody.fill('Second draft that must survive.')

    job.complete()

    // The optimize button re-enables once the (stale) job has been processed.
    await expect(page.getByTestId('composer-ai-optimize')).toBeEnabled({ timeout: 20_000 })
    await expect(defaultBody).toHaveValue('Second draft that must survive.')
    await expect(page.getByTestId('composer-ai-notice')).toBeVisible()
  })

  test('discards the AI result when the draft is edited while the job runs', async ({ page }) => {
    test.setTimeout(120_000)
    const teamId = TEAM_ID
    await mockTeams(page)
    await mockTeamAccounts(page, teamId)
    const job = await mockComposerAI(page, teamId, {
      content: 'AI refined draft body.',
      accountContentOverride: {},
    })

    await signIn(page)
    await page.goto(`/?team=${teamId}`)
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await page.getByRole('button', { name: 'New Post' }).click()

    const defaultBody = page.getByPlaceholder(DEFAULT_PLACEHOLDER)
    await defaultBody.fill('Original draft while the job runs.')
    await startOptimize(page)
    await expect.poll(job.triggerCount).toBe(1)

    await defaultBody.fill('Edited while the job runs.')

    job.complete()

    await expect(page.getByTestId('composer-ai-optimize')).toBeEnabled({ timeout: 20_000 })
    await expect(defaultBody).toHaveValue('Edited while the job runs.')
    await expect(page.getByTestId('composer-ai-notice')).toBeVisible()
  })

  test('discards the AI result when another team is selected while the job runs', async ({ page }) => {
    test.setTimeout(120_000)
    await mockTeams(page)
    await mockTeamAccounts(page, TEAM_ID)
    await mockTeamAccounts(page, OTHER_TEAM_ID)
    const job = await mockComposerAI(page, TEAM_ID, {
      content: 'AI refined draft body.',
      accountContentOverride: {},
    })

    await signIn(page)
    await page.goto(`/?team=${TEAM_ID}`)
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await page.getByRole('button', { name: 'New Post' }).click()

    const defaultBody = page.getByPlaceholder(DEFAULT_PLACEHOLDER)
    await defaultBody.fill('Draft of the first team.')
    await startOptimize(page)
    await expect.poll(job.triggerCount).toBe(1)

    await page.locator('.sidebar-team-selector').click()
    await page.getByRole('menuitem', { name: 'Mock other team', exact: true }).click()

    await expect(defaultBody).toHaveValue('')
    // Retype identical text so the content-change guard cannot mask a broken team guard.
    await defaultBody.fill('Draft of the first team.')

    job.complete()

    await expect(page.getByTestId('composer-ai-notice')).toBeVisible({ timeout: 20_000 })
    await expect(defaultBody).toHaveValue('Draft of the first team.')
  })

  test('discards the AI result when a newly created team is selected while the job runs', async ({ page }) => {
    test.setTimeout(120_000)
    await mockTeams(page)
    await mockTeamAccounts(page, TEAM_ID)
    await mockTeamAccounts(page, 'mock-created-team')
    const job = await mockComposerAI(page, TEAM_ID, {
      content: 'AI refined draft body.',
      accountContentOverride: {},
    })

    await signIn(page)
    await page.goto(`/?team=${TEAM_ID}`)
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await page.getByRole('button', { name: 'New Post' }).click()

    const defaultBody = page.getByPlaceholder(DEFAULT_PLACEHOLDER)
    await defaultBody.fill('Draft of the first team.')
    await startOptimize(page)
    await expect.poll(job.triggerCount).toBe(1)

    // Creating a team switches to it without going through the team menu.
    await page.locator('.sidebar-team-selector').click()
    await page.getByTestId('sidebar-create-team').click()
    await page.getByTestId('create-team-name').fill('Brand new team')
    await page.getByTestId('create-team-submit').click()
    await expect(page.getByTestId('create-team-modal')).toBeHidden()

    await expect(defaultBody).toHaveValue('')
    // Retype identical text so the content-change guard cannot mask a broken team guard.
    await defaultBody.fill('Draft of the first team.')

    job.complete()

    await expect(page.getByTestId('composer-ai-notice')).toBeVisible({ timeout: 20_000 })
    await expect(defaultBody).toHaveValue('Draft of the first team.')
  })

  test('default selection keeps user-written overrides and fills the remaining accounts', async ({ page }) => {
    test.setTimeout(120_000)
    const teamId = TEAM_ID
    await mockTeams(page)
    await mockTeamAccounts(page, teamId)
    const job = await mockComposerAI(page, teamId, {
      content: 'AI refined draft body.',
      accountContentOverride: {
        [MASTODON_ACCOUNT_ID]: 'AI mastodon variant',
        [BLUESKY_ACCOUNT_ID]: 'AI bluesky variant',
      },
    })

    await signIn(page)
    await page.goto(`/?team=${teamId}`)
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await page.getByRole('button', { name: 'New Post' }).click()

    const defaultBody = page.getByPlaceholder(DEFAULT_PLACEHOLDER)
    const overrideBody = page.getByPlaceholder(OVERRIDE_PLACEHOLDER)

    // The first team account (bluesky, sorted first) is targeted by default;
    // add the mastodon destination and write a user override on it.
    await page.getByTestId('composer-destination-toggle').filter({ hasText: MASTODON_USERNAME }).click()
    await defaultBody.fill('Original shared body.')
    await page.getByRole('tab', { name: new RegExp(MASTODON_USERNAME) }).click()
    await overrideBody.fill('User-written mastodon override.')
    await page.getByRole('tab', { name: 'Default', exact: true }).click()
    await expect(defaultBody).toHaveValue('Original shared body.')

    job.complete()
    await startOptimize(page)
    // Nothing changes until the proposal is applied.
    await expect(page.getByTestId('composer-ai-proposal')).toBeVisible({ timeout: 20_000 })
    await expect(defaultBody).toHaveValue('Original shared body.')
    await page.getByTestId('composer-ai-proposal-apply').click()
    await expect(page.getByTestId('composer-ai-proposal')).toHaveCount(0)
    await expect(defaultBody).toHaveValue('AI refined draft body.')

    // The account the user edited keeps their text …
    await page.getByRole('tab', { name: new RegExp(MASTODON_USERNAME) }).click()
    await expect(overrideBody).toHaveValue('User-written mastodon override.')
    // … while the account without a user override receives the AI variant.
    await page.getByRole('tab', { name: new RegExp(BLUESKY_USERNAME) }).click()
    await expect(overrideBody).toHaveValue('AI bluesky variant')
  })

  test.describe('proposal review', () => {
    const SHARED = 'Original shared body.'
    const MASTO_OVERRIDE = 'User-written mastodon override.'

    /** New post targeting both accounts, a user override on mastodon, optimize finished. */
    async function openProposal(page: Page, result: ComposerAIResult) {
      await mockTeams(page)
      await mockTeamAccounts(page, TEAM_ID)
      const job = await mockComposerAI(page, TEAM_ID, result)
      await signIn(page)
      await page.goto(`/?team=${TEAM_ID}`)
      await page.getByRole('button', { name: 'Calendar', exact: true }).click()
      await page.getByRole('button', { name: 'New Post' }).click()

      const defaultBody = page.getByPlaceholder(DEFAULT_PLACEHOLDER)
      const overrideBody = page.getByPlaceholder(OVERRIDE_PLACEHOLDER)
      await page.getByTestId('composer-destination-toggle').filter({ hasText: MASTODON_USERNAME }).click()
      await defaultBody.fill(SHARED)
      await page.getByRole('tab', { name: new RegExp(MASTODON_USERNAME) }).click()
      await overrideBody.fill(MASTO_OVERRIDE)
      await page.getByRole('tab', { name: 'Default', exact: true }).click()

      job.complete()
      await startOptimize(page)
      const proposal = page.getByTestId('composer-ai-proposal')
      await expect(proposal).toBeVisible({ timeout: 20_000 })
      return { defaultBody, overrideBody, proposal }
    }

    async function expectOverride(page: Page, username: string, value: string) {
      await page.getByRole('tab', { name: new RegExp(username) }).click()
      await expect(page.getByPlaceholder(OVERRIDE_PLACEHOLDER)).toHaveValue(value)
      await page.getByRole('tab', { name: 'Default', exact: true }).click()
    }

    test('shows original and proposed text side by side before changing the draft', async ({ page }) => {
      test.setTimeout(120_000)
      const { defaultBody, proposal } = await openProposal(page, {
        content: 'AI refined shared body.',
        accountContentOverride: { [MASTODON_ACCOUNT_ID]: 'AI mastodon variant' },
      })
      const shared = proposal.getByTestId('composer-ai-proposal-shared-diff')
      await expect(shared).toContainText(SHARED)
      await expect(shared).toContainText('AI refined shared body.')
      const masto = proposal.getByTestId(`composer-ai-proposal-item-${MASTODON_ACCOUNT_ID}`)
      await expect(masto).toContainText(MASTO_OVERRIDE)
      await expect(masto).toContainText('AI mastodon variant')
      // Bluesky got no variant: shown as unchanged, not as a removal.
      await expect(proposal.getByTestId('composer-ai-proposal-unchanged')).toContainText(BLUESKY_USERNAME)
      await expect(defaultBody).toHaveValue(SHARED)
    })

    test('partial acceptance applies only the selected account change', async ({ page }) => {
      test.setTimeout(120_000)
      const { defaultBody, proposal } = await openProposal(page, {
        content: 'AI refined shared body.',
        accountContentOverride: {
          [MASTODON_ACCOUNT_ID]: 'AI mastodon variant',
          [BLUESKY_ACCOUNT_ID]: 'AI bluesky variant',
        },
      })
      await proposal.getByTestId('composer-ai-proposal-shared').uncheck()
      await proposal.getByTestId(`composer-ai-proposal-account-${MASTODON_ACCOUNT_ID}`).check()
      await proposal.getByTestId(`composer-ai-proposal-account-${BLUESKY_ACCOUNT_ID}`).uncheck()
      await proposal.getByTestId('composer-ai-proposal-apply').click()

      await expect(proposal).toHaveCount(0)
      await expect(defaultBody).toHaveValue(SHARED)
      await expectOverride(page, MASTODON_USERNAME, 'AI mastodon variant')
      await expectOverride(page, BLUESKY_USERNAME, SHARED)
    })

    test('full acceptance applies the shared text and every account change', async ({ page }) => {
      test.setTimeout(120_000)
      const { defaultBody, proposal } = await openProposal(page, {
        content: 'AI refined shared body.',
        accountContentOverride: {
          [MASTODON_ACCOUNT_ID]: 'AI mastodon variant',
          [BLUESKY_ACCOUNT_ID]: 'AI bluesky variant',
        },
      })
      await proposal.getByTestId('composer-ai-proposal-apply-all').click()

      await expect(proposal).toHaveCount(0)
      await expect(defaultBody).toHaveValue('AI refined shared body.')
      await expectOverride(page, MASTODON_USERNAME, 'AI mastodon variant')
      await expectOverride(page, BLUESKY_USERNAME, 'AI bluesky variant')
    })

    test('an explicit override removal is labelled and makes the account inherit the shared text', async ({ page }) => {
      test.setTimeout(120_000)
      const { defaultBody, proposal } = await openProposal(page, {
        content: 'AI refined shared body.',
        accountContentOverride: { [MASTODON_ACCOUNT_ID]: null },
      })
      const masto = proposal.getByTestId(`composer-ai-proposal-item-${MASTODON_ACCOUNT_ID}`)
      await expect(masto.getByTestId('composer-ai-proposal-removal')).toBeVisible()
      await proposal.getByTestId('composer-ai-proposal-apply-all').click()

      await expect(defaultBody).toHaveValue('AI refined shared body.')
      await expectOverride(page, MASTODON_USERNAME, 'AI refined shared body.')
    })

    test('discarding the proposal leaves the draft intact', async ({ page }) => {
      test.setTimeout(120_000)
      const { defaultBody, proposal } = await openProposal(page, {
        content: 'AI refined shared body.',
        accountContentOverride: { [BLUESKY_ACCOUNT_ID]: 'AI bluesky variant' },
      })
      await proposal.getByTestId('composer-ai-proposal-discard').click()

      await expect(proposal).toHaveCount(0)
      await expect(defaultBody).toHaveValue(SHARED)
      await expectOverride(page, MASTODON_USERNAME, MASTO_OVERRIDE)
      await expectOverride(page, BLUESKY_USERNAME, SHARED)
    })

    test('editing the draft while the proposal is open drops the stale proposal', async ({ page }) => {
      test.setTimeout(120_000)
      const { defaultBody, proposal } = await openProposal(page, {
        content: 'AI refined shared body.',
        accountContentOverride: {},
      })
      await defaultBody.fill('Edited after the proposal arrived.')

      await expect(proposal).toHaveCount(0)
      await expect(page.getByTestId('composer-ai-notice')).toBeVisible()
      await expect(defaultBody).toHaveValue('Edited after the proposal arrived.')
    })

    test('explains an invalid limit combination and blocks applying it', async ({ page }) => {
      test.setTimeout(120_000)
      const longShared = `${'Long shared text that no longer fits Bluesky. '.repeat(8)}End.`
      const { defaultBody, proposal } = await openProposal(page, {
        content: longShared,
        accountContentOverride: { [BLUESKY_ACCOUNT_ID]: 'Short bluesky variant.' },
      })
      await proposal.getByTestId(`composer-ai-proposal-account-${BLUESKY_ACCOUNT_ID}`).uncheck()

      const limitError = proposal.getByTestId('composer-ai-proposal-limit-error')
      await expect(limitError).toBeVisible()
      await expect(limitError).toContainText(BLUESKY_USERNAME)
      await expect(proposal.getByTestId('composer-ai-proposal-apply')).toBeDisabled()

      await proposal.getByTestId(`composer-ai-proposal-account-${BLUESKY_ACCOUNT_ID}`).check()
      await expect(limitError).toHaveCount(0)
      await proposal.getByTestId('composer-ai-proposal-apply').click()
      await expect(defaultBody).toHaveValue(longShared)
      await expectOverride(page, BLUESKY_USERNAME, 'Short bluesky variant.')
    })

    test('reports a proposal without changes', async ({ page }) => {
      test.setTimeout(120_000)
      const { defaultBody, proposal } = await openProposal(page, { content: SHARED, accountContentOverride: {} })
      await expect(proposal.getByTestId('composer-ai-proposal-empty')).toBeVisible()
      await expect(proposal.getByTestId('composer-ai-proposal-apply')).toHaveCount(0)
      await proposal.getByTestId('composer-ai-proposal-discard').click()
      await expect(defaultBody).toHaveValue(SHARED)
    })
  })

  test.describe('optimize errors', () => {
    async function openNewPost(page: Page) {
      await mockTeams(page)
      await mockTeamAccounts(page, TEAM_ID)
      await signIn(page)
      await page.goto(`/?team=${TEAM_ID}`)
      await page.getByRole('button', { name: 'Calendar', exact: true }).click()
      await page.getByRole('button', { name: 'New Post' }).click()
      const defaultBody = page.getByPlaceholder(DEFAULT_PLACEHOLDER)
      await defaultBody.fill('Draft to optimize.')
      return defaultBody
    }

    async function mockJob(page: Page, job: Record<string, unknown>) {
      await page.route(`**/v1/teams/${TEAM_ID}/ai-trigger`, (route) =>
        route.fulfill({ status: 202, json: { job_id: 'job-err', status: 'pending' } }),
      )
      await page.route(`**/v1/teams/${TEAM_ID}/ai-jobs/job-err`, (route) =>
        route.fulfill({
          json: { id: 'job-err', team_id: TEAM_ID, type: 'voice_engine', payload: {}, ...job },
        }),
      )
    }

    test('shows a failed generation and keeps the draft', async ({ page }) => {
      test.setTimeout(120_000)
      const defaultBody = await openNewPost(page)
      await mockJob(page, { status: 'failed', error_message: 'provider exploded' })
      await startOptimize(page)
      await expect(page.getByTestId('composer-ai-error')).toContainText('provider exploded', { timeout: 20_000 })
      await expect(page.getByTestId('composer-ai-proposal')).toHaveCount(0)
      await expect(defaultBody).toHaveValue('Draft to optimize.')
    })

    test('shows an empty proposal as an error', async ({ page }) => {
      test.setTimeout(120_000)
      const defaultBody = await openNewPost(page)
      await mockJob(page, { status: 'completed', result: { content: '  ' } })
      await startOptimize(page)
      await expect(page.getByTestId('composer-ai-error')).toBeVisible({ timeout: 20_000 })
      await expect(page.getByTestId('composer-ai-proposal')).toHaveCount(0)
      await expect(defaultBody).toHaveValue('Draft to optimize.')
    })

    test('cancelling a running optimization stops it without a proposal', async ({ page }) => {
      test.setTimeout(120_000)
      const defaultBody = await openNewPost(page)
      let cancelled = false
      await page.route(`**/v1/teams/${TEAM_ID}/ai-jobs/job-err/cancel`, (route) => {
        cancelled = true
        return route.fulfill({ json: { id: 'job-err', team_id: TEAM_ID, status: 'failed' } })
      })
      await mockJob(page, { status: 'pending' })
      await startOptimize(page)
      await page.getByTestId('composer-ai-cancel').click()
      await expect.poll(() => cancelled).toBe(true)
      await expect(page.getByTestId('composer-ai-optimize')).toBeEnabled()
      await expect(page.getByTestId('composer-ai-proposal')).toHaveCount(0)
      await expect(defaultBody).toHaveValue('Draft to optimize.')
    })
  })

  test.describe('async editor loads', () => {
    const VERSIONS = '**/v1/teams/*/posts/*/versions'

    /** Holds the next post-versions response until release() is called. */
    async function holdNextVersionsRequest(page: Page) {
      let release!: () => void
      const gate = new Promise<void>((resolve) => {
        release = resolve
      })
      let held = false
      await page.route(VERSIONS, async (route) => {
        if (held || route.request().method() !== 'GET') {
          await route.fallback()
          return
        }
        held = true
        await gate
        await route.fallback()
      })
      return { release: () => release(), isHeld: () => held }
    }

    test('an editor load finishing after another draft was opened is discarded', async ({ page, baseURL }) => {
      test.setTimeout(120_000)
      const teamId = await getFirstTeamId(baseURL!, e2eBootstrapToken())
      await mockTeams(page)
      await mockTeamAccounts(page, teamId)
      const job = await mockComposerAI(page, teamId, {
        content: 'AI refined draft body.',
        accountContentOverride: {},
      })
      await signIn(page)
      await openContentCalendar(page)

      // Learn the seeded post's text so draft B can share it exactly.
      await openSeededPostPreview(page)
      await page.getByTestId('preview-edit-button').click()
      const defaultBody = page.getByPlaceholder(DEFAULT_PLACEHOLDER)
      await expect(page.getByTestId('composer-title')).toHaveText('Edit post', { timeout: 15_000 })
      const seededText = await defaultBody.inputValue()
      await page.getByRole('button', { name: 'Cancel' }).click()

      // Draft A: the post editor whose versions request is held back.
      const hold = await holdNextVersionsRequest(page)
      // Cancel restores the calendar with the preview still open.
      await page.getByTestId('preview-edit-button').click()
      await expect.poll(hold.isHeld).toBe(true)

      // Draft B: a new post with identical shared text, optimization running.
      await page.getByRole('button', { name: 'New Post' }).click()
      await expect(page.getByTestId('composer-title')).toHaveText('Create post')
      await defaultBody.fill(seededText)
      await startOptimize(page)
      await expect.poll(job.triggerCount).toBe(1)

      hold.release()
      await page.waitForTimeout(500)
      job.complete()

      await expect(page.getByTestId('composer-ai-proposal')).toBeVisible({ timeout: 20_000 })
      await expect(page.getByTestId('composer-title')).toHaveText('Create post')
      await page.getByTestId('composer-ai-proposal-apply').click()
      await expect(defaultBody).toHaveValue('AI refined draft body.')
    })

    test('an editor load finishing after a team switch does not restore the old draft', async ({ page }) => {
      test.setTimeout(120_000)
      await mockTeams(page)
      await signIn(page)
      await openContentCalendar(page)
      await openSeededPostPreview(page)

      const hold = await holdNextVersionsRequest(page)
      await page.getByTestId('preview-edit-button').click()
      await expect.poll(hold.isHeld).toBe(true)

      await page.locator('.sidebar-team-selector').click()
      await page.getByRole('menuitem', { name: 'Mock other team', exact: true }).click()

      hold.release()
      await page.waitForTimeout(1000)
      await expect(page.getByTestId('composer-view')).toHaveCount(0)
    })
  })
})
