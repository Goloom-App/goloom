import { test, expect } from '@playwright/test'

import { signIn } from './helpers'

test('generate, share, and revoke an email-bound invite link', async ({ page }) => {
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

  // Team settings explain the email-bound link and its limits.
  await page.getByRole('button', { name: 'Team', exact: true }).click()
  await page.getByRole('tab', { name: 'Members', exact: true }).click()
  await expect(page.getByText('Invites', { exact: true })).toBeVisible()
  await expect(page.getByText(/bound to the entered email address/i)).toBeVisible()
  await expect(page.getByText(/seven days/i)).toBeVisible()
  const email = `invitee-${Date.now().toString(36)}@example.test`
  await page.getByTestId('invite-email').fill(email)
  await page.getByTestId('invite-role').selectOption('viewer')
  await expect(page.getByTestId('invite-submit')).toHaveText('Generate invite link')
  await page.getByTestId('invite-submit').click()

  // One-time invite link with the ?invite= token is displayed.
  const linkPanel = page.getByTestId('invite-link-panel')
  await expect(linkPanel).toBeVisible({ timeout: 15_000 })
  await expect(linkPanel).toContainText('shown only once')
  await expect(linkPanel).toContainText('goloom does not send the email')
  await expect(page.getByTestId('invite-link')).toContainText('?invite=')
  const inviteLink = (await page.getByTestId('invite-link').textContent())?.trim()
  expect(inviteLink).toBeTruthy()
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await linkPanel.getByRole('button', { name: 'Copy link' }).click()
  await expect(linkPanel.getByRole('button', { name: 'Copied!' })).toBeVisible()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(inviteLink)

  const mailto = await page.getByTestId('invite-send-email').getAttribute('href')
  expect(mailto).toBeTruthy()
  const draft = new URL(mailto!)
  expect(draft.protocol).toBe('mailto:')
  expect(draft.pathname).toBe(email)
  expect(draft.searchParams.get('subject')).toBeTruthy()
  expect(draft.searchParams.get('body')).toContain(inviteLink)

  // The invitation is listed as pending and can be revoked.
  const pendingRow = page.getByTestId('invite-pending-row').filter({ hasText: email })
  await expect(pendingRow).toBeVisible()
  await expect(pendingRow).toContainText('Viewer')
  await expect(pendingRow).toContainText('expires')
  await pendingRow.getByTestId('invite-revoke').click()
  await expect(pendingRow).toHaveCount(0, { timeout: 15_000 })
})
