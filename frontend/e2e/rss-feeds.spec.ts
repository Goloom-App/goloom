import { test, expect } from '@playwright/test'

import { signIn } from './helpers'

test.describe('rss feeds', () => {
  test('navigation opens RSS feeds view', async ({ page }) => {
    test.setTimeout(60_000)
    await signIn(page)
    await page.getByRole('button', { name: /^Automation$/i }).click()
    await page.getByTestId('automation-tab-rss').click()
    await expect(page.getByText(/automatically create posts/i)).toBeVisible()
    await expect(page.getByRole('button', { name: /add feed/i })).toBeVisible()
  })
})
