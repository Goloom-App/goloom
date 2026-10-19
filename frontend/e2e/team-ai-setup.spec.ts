import { test, expect } from '@playwright/test'

import { signIn } from './helpers'

test('new team can enable AI and save its first provider configuration', async ({ page }) => {
  test.setTimeout(90_000)
  await signIn(page)

  const teamName = `E2E AI Setup ${Date.now().toString(36)}`
  await page.locator('.sidebar-team-selector').click()
  await page.getByTestId('sidebar-create-team').click()
  const modal = page.getByTestId('create-team-modal')
  await modal.getByTestId('create-team-name').fill(teamName)
  await modal.getByTestId('create-team-submit').click()
  await expect(modal).toBeHidden()
  await expect(page.locator('.sidebar-team-selector')).toContainText(teamName)

  await page.getByRole('button', { name: 'Team', exact: true }).click()
  await page.getByRole('tab', { name: 'AI', exact: true }).click()
  const aiToggle = page.getByTestId('team-ai-toggle').locator('input')
  await expect(aiToggle).not.toBeChecked()
  await page.getByTestId('team-ai-toggle').click()
  await page.getByLabel('Model').fill('gpt-test')
  await page.getByLabel('API key').fill('sk-e2e-test')
  const enabledResponse = page.waitForResponse((response) =>
    response.request().method() === 'PATCH' && /\/v1\/teams\/[^/]+$/.test(response.url()),
  )
  await page.getByRole('button', { name: 'Save AI configuration' }).click()
  expect((await enabledResponse).status()).toBe(200)

  await expect(page.getByText('AI is not enabled for this team')).toHaveCount(0)
  await page.reload()
  await page.getByRole('button', { name: 'Team', exact: true }).click()
  await page.getByRole('tab', { name: 'AI', exact: true }).click()
  await expect(aiToggle).toBeChecked()
  await expect(page.getByLabel('API key')).toHaveAttribute('placeholder', 'Key stored — leave empty to keep it')
})
