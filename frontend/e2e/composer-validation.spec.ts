import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

test.describe('composer validation', () => {
  test('save button disabled when no destinations selected', async ({ page }) => {
    test.setTimeout(60_000)
    await page.setViewportSize({ width: 1280, height: 720 })

    await signIn(page)

    // Navigate to calendar to access new post button
    await page.getByRole('button', { name: 'Calendar', exact: true }).click()
    await expect(page.getByRole('heading', { level: 1, name: 'Content calendar' })).toBeVisible()

    // Open new post composer
    await page.getByRole('button', { name: 'New Post' }).click()
    await expect(page.getByTestId('composer-view')).toBeVisible({ timeout: 10_000 })

    // Type content (label is "Message (all destinations)", not "Content" — tablist uses contentScopeAria)
    await page
      .getByTestId('composer-view')
      .getByLabel(/message.*all destinations|nachricht.*alle ziele/i)
      .fill('Test content for validation')

    // Save button should be disabled because no destinations are selected
    const saveBtn = page.getByRole('button', { name: /schedule post|speichern/i })
    await expect(saveBtn).toBeDisabled()

    // Save draft should be enabled regardless
    await expect(page.getByRole('button', { name: /save draft/i })).toBeEnabled()
  })
})
