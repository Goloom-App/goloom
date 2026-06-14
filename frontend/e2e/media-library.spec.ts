import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

// 1x1 transparent PNG.
const PNG_BASE64 =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=='

// Issue #84: the media library supports search, rename (via a lightbox), and a
// lightbox preview when clicking an item.
test('upload, preview in lightbox, rename, and search media', async ({ page }) => {
  test.setTimeout(90_000)
  await page.setViewportSize({ width: 1280, height: 900 })
  await signIn(page)

  await page.getByRole('button', { name: 'Media', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Media library' })).toBeVisible({ timeout: 15_000 })

  await page.locator('input.media-library__file-input').setInputFiles({
    name: 'cat.png',
    mimeType: 'image/png',
    buffer: Buffer.from(PNG_BASE64, 'base64'),
  })

  const tile = page.locator('.media-library__tile')
  await expect(tile.filter({ hasText: 'cat.png' })).toBeVisible({ timeout: 15_000 })

  // Open the lightbox and rename the file.
  await tile.filter({ hasText: 'cat.png' }).locator('.media-library__open-btn').click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()
  const renameInput = dialog.getByRole('textbox')
  await renameInput.fill('kitten.png')
  await dialog.getByRole('button', { name: 'Rename' }).click()
  await page.keyboard.press('Escape')

  await expect(tile.filter({ hasText: 'kitten.png' })).toBeVisible({ timeout: 15_000 })
  await expect(page.getByText('cat.png')).toHaveCount(0)

  // Search filters by file name.
  const search = page.getByPlaceholder('Search by file name…')
  await search.fill('kitten')
  await expect(tile.filter({ hasText: 'kitten.png' })).toBeVisible()
  await search.fill('zzz-nope')
  await expect(page.getByText(/No media matches/)).toBeVisible()
})
