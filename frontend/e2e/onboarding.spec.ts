import { spawn, type ChildProcess } from 'node:child_process'
import { mkdirSync, rmSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { test, expect } from '@playwright/test'

import { e2eBootstrapToken } from './constants'

// Onboarding needs a user WITHOUT any team. The shared e2e server is seeded
// with a team in global-setup, so this spec boots its own server instance on
// a fresh database and drives the first sign-in end to end.

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.join(__dirname, '..', '..')
const port = process.env.PLAYWRIGHT_ONBOARDING_PORT ?? '18081'
const baseURL = `http://127.0.0.1:${port}`
const dbPath = path.join(repoRoot, '.e2e', 'goloom-onboarding-e2e.db')

let server: ChildProcess

test.beforeAll(async () => {
  mkdirSync(path.dirname(dbPath), { recursive: true })
  rmSync(dbPath, { force: true })
  rmSync(`${dbPath}-wal`, { force: true })
  rmSync(`${dbPath}-shm`, { force: true })

  server = spawn(path.join(repoRoot, 'bin', 'goloom'), [], {
    env: {
      ...process.env,
      HTTP_ADDR: `127.0.0.1:${port}`,
      DATABASE_URL: `file:${dbPath}?_journal_mode=WAL&_busy_timeout=8000`,
      BOOTSTRAP_ADMIN_TOKEN: e2eBootstrapToken(),
      PUBLIC_BASE_URL: baseURL,
      APP_ENV: 'development',
      ALLOWED_ORIGINS: baseURL,
      RATE_LIMIT_PER_MINUTE: '5000',
      RATE_LIMIT_AUTHENTICATED_PER_MINUTE: '10000',
    },
    stdio: 'ignore',
  })

  await expect(async () => {
    const res = await fetch(`${baseURL}/healthz`)
    expect(res.ok).toBe(true)
  }).toPass({ timeout: 30_000 })
})

test.afterAll(() => {
  server?.kill()
})

test('first sign-in without a team runs the onboarding wizard', async ({ page }) => {
  test.setTimeout(90_000)

  await page.goto(baseURL)
  const tokenField = page.getByLabel(/access token|administrator token/i)
  await expect(tokenField).toBeVisible({ timeout: 30_000 })
  await tokenField.fill(e2eBootstrapToken())
  await page.getByRole('button', { name: 'Sign in with token' }).click()

  // No team yet → the wizard replaces the dashboard.
  const wizard = page.getByTestId('onboarding-wizard')
  await expect(wizard).toBeVisible({ timeout: 30_000 })

  // The team name is prefilled (display name or email local part).
  const nameField = wizard.getByTestId('onboarding-team-name')
  await expect(nameField).not.toHaveValue('')

  const teamName = `Onboarding Team ${Date.now().toString(36)}`
  await nameField.fill(teamName)
  await wizard.getByTestId('onboarding-create-team').click()

  // Creating the team completes onboarding and starts the guided tour.
  // The bootstrap admin gets the admin flow: providers first, then accounts.
  const tour = page.getByTestId('guided-tour')
  await expect(tour).toBeVisible({ timeout: 30_000 })
  await expect(tour).toHaveAttribute('data-tour-step', 'welcome')
  await tour.getByTestId('tour-next').click()

  // Interactive step: open the user menu and choose Admin.
  await expect(tour).toHaveAttribute('data-tour-step', 'open-admin')
  await page.getByTestId('user-menu-trigger').click()
  await page.getByRole('menuitem', { name: 'Admin' }).click()

  // Interactive step: the spotlighted providers tab must be clicked for real.
  await expect(tour).toHaveAttribute('data-tour-step', 'admin-providers', { timeout: 15_000 })
  await page.getByRole('tab', { name: 'Providers' }).click()

  await expect(tour).toHaveAttribute('data-tour-step', 'provider-form', { timeout: 15_000 })
  await tour.getByTestId('tour-next').click()

  // Interactive step: navigate to the accounts section via the sidebar.
  await expect(tour).toHaveAttribute('data-tour-step', 'nav-accounts')
  await page.getByRole('button', { name: 'Accounts', exact: true }).click()

  await expect(tour).toHaveAttribute('data-tour-step', 'accounts-connect', { timeout: 15_000 })
  await tour.getByTestId('tour-next').click()

  await expect(tour).toHaveAttribute('data-tour-step', 'new-post')
  await tour.getByTestId('tour-next').click()

  await expect(tour).toHaveAttribute('data-tour-step', 'finish')
  await tour.getByTestId('tour-done').click()
  await expect(tour).toBeHidden()

  // The tour walked us into the accounts section; the team is set up.
  await expect(page.locator('.sidebar-team-selector')).toContainText(teamName, { timeout: 15_000 })

  // Reloading brings back neither the wizard nor the tour — both are done.
  // (The app restores the last visited section, so don't assert a heading.)
  await page.reload()
  await expect(page.locator('.sidebar-team-selector')).toContainText(teamName, { timeout: 30_000 })
  await expect(page.getByTestId('onboarding-wizard')).toHaveCount(0)
  await expect(page.getByTestId('guided-tour')).toHaveCount(0)

  // The tour can be replayed from the settings.
  await page.getByTestId('user-menu-trigger').click()
  await page.getByRole('menuitem', { name: 'Settings' }).click()
  await page.getByTestId('restart-tour').click()
  await expect(page.getByTestId('guided-tour')).toBeVisible()
  await page.getByTestId('tour-skip').click()
  await expect(page.getByTestId('guided-tour')).toBeHidden()
})
