import { test, expect } from '@playwright/test'

import { signIn } from './helpers'

// Owners can invite people by email from team settings: creating an invitation
// shows the one-time invite link, the invitation appears as pending, and it
// can be revoked again.
test('invite a member by email and revoke the invitation', async ({ page }) => {
  test.setTimeout(90_000)
  await signIn(page)

  const teamName = `E2E Invite Team ${Date.now().toString(36)}`

  // A dedicated team keeps this spec independent of other specs' state.
  await page.locator('.sidebar-team-selector').click()
  await page.getByTestId('sidebar-create-team').click()
  const modal = page.getByTestId('create-team-modal')
  await expect(modal).toBeVisible()
  await modal.getByTestId('create-team-name').fill(teamName)
  await modal.getByTestId('create-team-submit').click()
  await expect(modal).toBeHidden({ timeout: 15_000 })
  await expect(page.locator('.sidebar-team-selector')).toContainText(teamName, { timeout: 15_000 })

  // Team settings → invite by email.
  await page.getByRole('button', { name: 'Team', exact: true }).click()
  const email = `invitee-${Date.now().toString(36)}@example.test`
  await page.getByTestId('invite-email').fill(email)
  await page.getByTestId('invite-role').selectOption('viewer')
  await page.getByTestId('invite-submit').click()

  // One-time invite link with the ?invite= token is displayed.
  const linkPanel = page.getByTestId('invite-link-panel')
  await expect(linkPanel).toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('invite-link')).toContainText('?invite=')

  // The invitation is listed as pending and can be revoked.
  const pendingRow = page.getByTestId('invite-pending-row').filter({ hasText: email })
  await expect(pendingRow).toBeVisible()
  await pendingRow.getByTestId('invite-revoke').click()
  await expect(pendingRow).toHaveCount(0, { timeout: 15_000 })
})
