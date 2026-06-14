import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

// Issue #81: creating an API token now happens in a modal (name, description,
// team, scopes, expiry) and the new token is revealed once for copying.
test('create an API token through the modal and reveal it', async ({ page }) => {
  test.setTimeout(60_000)
  await page.setViewportSize({ width: 1280, height: 900 })
  await signIn(page)

  await page.getByTestId('user-menu-trigger').click()
  await page.getByRole('menuitem', { name: 'Settings' }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'Settings' })).toBeVisible()

  await page.getByRole('button', { name: '+ New Token' }).click()

  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading', { name: 'Create API token' })).toBeVisible()
  await dialog.getByPlaceholder('e.g. CI, laptop').fill('e2e-modal-token')
  await dialog.getByPlaceholder('What is this token for?').fill('created by e2e')
  // Restrict to a single scope to exercise the scope checkboxes.
  await dialog.getByText('read', { exact: true }).click()
  await dialog.getByRole('button', { name: 'Create token' }).click()

  // The plaintext token is revealed exactly once.
  await expect(page.getByRole('heading', { name: 'Your new token' })).toBeVisible()
  await expect(page.getByText(/gl_/)).toBeVisible()

  await page.keyboard.press('Escape')

  // The new token shows up in the list with its description and scope.
  await expect(page.getByText('e2e-modal-token')).toBeVisible()
  await expect(page.getByText('created by e2e')).toBeVisible()
})
