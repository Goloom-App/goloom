import { test, expect } from '@playwright/test'

import { signIn } from './helpers'

// Regression: clicking "+ Create Team" in the sidebar team selector must actually
// open the create-team flow and create the team (the dropdown item previously had
// no handler, so nothing happened).
test('create a team from the sidebar team selector', async ({ page }) => {
  test.setTimeout(60_000)
  await signIn(page)

  const teamName = `E2E Team ${Date.now().toString(36)}`

  // Open the team selector and start team creation.
  await page.locator('.sidebar-team-selector').click()
  await page.getByTestId('sidebar-create-team').click()

  // Fill in the modal and submit.
  const modal = page.getByTestId('create-team-modal')
  await expect(modal).toBeVisible()
  await modal.getByTestId('create-team-name').fill(teamName)
  await modal.getByTestId('create-team-submit').click()

  // The modal closes and the freshly created team becomes the selected team.
  await expect(modal).toBeHidden({ timeout: 15_000 })
  await expect(page.locator('.sidebar-team-selector')).toContainText(teamName, { timeout: 15_000 })
})
