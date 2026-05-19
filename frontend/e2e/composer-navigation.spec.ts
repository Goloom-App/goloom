import { test, expect } from '@playwright/test'
import { E2E_SEEDED_POST_TITLE } from './constants'
import { openContentCalendar, openSeededPostPreview, signIn } from './helpers'

test.describe('composer navigation', () => {
  test('desktop: new post opens dedicated composer and cancel returns to calendar', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })

    await signIn(page)
    await openContentCalendar(page)

    await page.getByRole('button', { name: 'New Post' }).click()

    await expect(page.getByTestId('composer-view')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('composer-title')).toHaveText('Create post')
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toHaveCount(0)

    await page.getByRole('button', { name: 'Cancel' }).click()

    await expect(page.getByTestId('composer-view')).toHaveCount(0)
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()
  })

  test('mobile: new post opens fullscreen composer overlay and cancel returns to calendar', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 390, height: 844 })

    await signIn(page)
    await openContentCalendar(page)

    await page.getByRole('button', { name: 'Post', exact: true }).click()

    const overlay = page.getByTestId('composer-overlay')
    await expect(overlay).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('composer-title')).toHaveText('Create post')
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toHaveCount(0)

    const viewport = page.viewportSize()!
    const box = await overlay.boundingBox()
    expect(box).not.toBeNull()
    // inset:0 overlay; CI/subpixel/safe-area can nudge y slightly below zero threshold
    expect(box!.y).toBeLessThanOrEqual(32)
    expect(box!.height).toBeGreaterThan(viewport.height * 0.85)

    await page.getByRole('button', { name: 'Cancel' }).click()

    await expect(overlay).toHaveCount(0)
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()
  })

  test('desktop: edit from preview opens composer and cancel restores calendar', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })

    await signIn(page)
    await openContentCalendar(page)
    await openSeededPostPreview(page)

    await expect(page.getByTestId('live-preview-title')).toHaveText(E2E_SEEDED_POST_TITLE)
    await page.getByTestId('preview-edit-button').click()

    await expect(page.getByTestId('composer-title')).toHaveText('Edit post', { timeout: 15_000 })
    await expect(page.getByTestId('composer-view')).toBeVisible()
    await expect(page.getByTestId('live-preview-title')).toHaveCount(0)

    await page.getByRole('button', { name: 'Cancel' }).click()

    await expect(page.getByTestId('composer-view')).toHaveCount(0)
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()
    await expect(page.getByTestId('live-preview-title')).toHaveText(E2E_SEEDED_POST_TITLE)
  })

  test('mobile: edit from preview opens composer overlay and closes preview sheet', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 390, height: 844 })

    await signIn(page)
    await openContentCalendar(page)
    await openSeededPostPreview(page)

    await expect(page.getByTestId('mobile-preview-overlay')).toBeVisible({ timeout: 10_000 })
    await page.getByTestId('preview-edit-button').click()

    await expect(page.getByTestId('composer-title')).toHaveText('Edit post', { timeout: 15_000 })
    await expect(page.getByTestId('composer-overlay')).toBeVisible()
    await expect(page.getByTestId('mobile-preview-overlay')).toHaveCount(0)

    await page.getByRole('button', { name: 'Cancel' }).click()

    await expect(page.getByTestId('composer-overlay')).toHaveCount(0)
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()
  })
})
