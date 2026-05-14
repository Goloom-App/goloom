import { test, expect } from '@playwright/test'
import { e2eBootstrapToken, E2E_SEEDED_POST_TITLE } from './constants'

test.describe('post preview', () => {
  test('desktop: calendar post opens sidebar preview, not mobile modal', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })

    await page.goto('/')
    await page.getByLabel(/access token|administrator token/i).fill(e2eBootstrapToken())
    await page.getByRole('button', { name: 'Sign in with token' }).click()

    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })

    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()

    await page.getByRole('button', { name: 'List', exact: true }).click()
    const seededCard = page.getByRole('article').filter({ hasText: E2E_SEEDED_POST_TITLE })
    await expect(seededCard).toBeVisible({ timeout: 15_000 })
    await seededCard.click()

    await expect(page.getByTestId('live-preview-title')).toHaveText(E2E_SEEDED_POST_TITLE, { timeout: 10_000 })
    await expect(page.getByTestId('mobile-preview-overlay')).toHaveCount(0)
  })

  test('mobile width: post opens preview sheet', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 390, height: 844 })

    await page.goto('/')
    await page.getByLabel(/access token|administrator token/i).fill(e2eBootstrapToken())
    await page.getByRole('button', { name: 'Sign in with token' }).click()

    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toBeVisible({ timeout: 30_000 })

    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()

    await page.getByRole('button', { name: 'List', exact: true }).click()
    const seededCardMobile = page.getByRole('article').filter({ hasText: E2E_SEEDED_POST_TITLE })
    await expect(seededCardMobile).toBeVisible({ timeout: 15_000 })
    await seededCardMobile.click()

    await expect(page.getByTestId('mobile-preview-overlay')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('live-preview-title')).toHaveCount(0)
  })
})
