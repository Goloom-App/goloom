import { test, expect, type Page } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam, signIn } from './helpers'

const DEFAULT_PLACEHOLDER = "What's on your mind?"
const OVERRIDE_PLACEHOLDER = 'Leave aligned with default, or type a custom version for this account.'

const MASTODON_ACCOUNT_ID = 'acc-mastodon'
const BLUESKY_ACCOUNT_ID = 'acc-bluesky'
const MASTODON_USERNAME = 'masto-user'
const BLUESKY_USERNAME = 'bsky-user'

type ComposerAIResult = {
  content: string
  accountContentOverride: Record<string, string>
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
  test('discards a stale AI result when the composer draft changes', async ({ page, baseURL }) => {
    test.setTimeout(120_000)
    const teamId = await createAITeam(baseURL!, e2eBootstrapToken())
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

  test('discards the AI result when the draft is edited while the job runs', async ({ page, baseURL }) => {
    test.setTimeout(120_000)
    const teamId = await createAITeam(baseURL!, e2eBootstrapToken())
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

  test('keeps user-written per-account overrides and fills the remaining accounts', async ({ page, baseURL }) => {
    test.setTimeout(120_000)
    const teamId = await createAITeam(baseURL!, e2eBootstrapToken())
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
    await expect(defaultBody).toHaveValue('AI refined draft body.')

    // The account the user edited keeps their text …
    await page.getByRole('tab', { name: new RegExp(MASTODON_USERNAME) }).click()
    await expect(overrideBody).toHaveValue('User-written mastodon override.')
    // … while the account without a user override receives the AI variant.
    await page.getByRole('tab', { name: new RegExp(BLUESKY_USERNAME) }).click()
    await expect(overrideBody).toHaveValue('AI bluesky variant')
  })
})
