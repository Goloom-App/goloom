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
