import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'
import { e2eBootstrapToken } from './e2e/constants'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.join(__dirname, '..')

const port = process.env.PLAYWRIGHT_SERVER_PORT ?? '18080'
const baseURL = process.env.PLAYWRIGHT_BASE_URL ?? `http://127.0.0.1:${port}`

const token = e2eBootstrapToken()
const serverBin = path.join(repoRoot, 'bin', 'goloom')

const webServerCommand = `sh -c 'mkdir -p .e2e && rm -f .e2e/goloom-e2e.db && exec env HTTP_ADDR="127.0.0.1:${port}" DATABASE_URL="file:.e2e/goloom-e2e.db?_journal_mode=WAL&_busy_timeout=8000" BOOTSTRAP_ADMIN_TOKEN="${token.replace(/"/g, '\\"')}" PUBLIC_BASE_URL="${baseURL}" APP_ENV=development ALLOWED_ORIGINS="${baseURL},http://localhost:${port}" RATE_LIMIT_PER_MINUTE=5000 RATE_LIMIT_AUTHENTICATED_PER_MINUTE=10000 "${serverBin}"'`

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? 'line' : 'list',
  globalSetup: './e2e/global-setup.ts',
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: process.env.PLAYWRIGHT_SKIP_WEB_SERVER
    ? undefined
    : {
        command: webServerCommand,
        cwd: repoRoot,
        url: `${baseURL}/healthz`,
        reuseExistingServer: !process.env.CI,
        timeout: 120_000,
      },
})
