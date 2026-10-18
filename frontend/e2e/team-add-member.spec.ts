import { expect, test } from '@playwright/test'

import { signIn } from './helpers'

const newUser = {
  id: 'e2e-add-member-user',
  name: 'New Team Member',
  email: 'new-member@example.test',
  subject: 'e2e-add-member-user',
  is_admin: false,
  tour_done: true,
  created_at: new Date().toISOString(),
}

test('owner adds an existing user from the Members header', async ({ page }) => {
  let added = false
  let addRequests = 0

  await page.route('**/v1/users', async (route) => {
    const response = await route.fetch()
    const body = await response.json() as { items: Array<Record<string, unknown>> }
    await route.fulfill({ response, json: { ...body, items: [...body.items, newUser] } })
  })
  await page.route('**/v1/teams/*/members', async (route) => {
    if (route.request().method() === 'POST') {
      expect(route.request().postDataJSON()).toEqual({ user_id: newUser.id, role: 'editor' })
      addRequests += 1
      added = true
      await route.fulfill({ status: 201, json: { user_id: newUser.id, role: 'editor' } })
      return
    }
    const response = await route.fetch()
    const body = await response.json() as { items: Array<Record<string, unknown>> }
    await route.fulfill({ response, json: {
      ...body,
      items: added ? [...body.items, { user_id: newUser.id, role: 'editor' }] : body.items,
    } })
  })

  await signIn(page)
  await page.getByRole('button', { name: 'Team', exact: true }).click()
  const members = page.locator('section.brand-card').filter({ has: page.getByRole('heading', { name: 'Members', exact: true }) })
  const addButton = members.getByRole('button', { name: 'Add Member' })
  await expect(addButton).toBeVisible()
  await expect(members.getByRole('combobox', { name: 'Select user…' })).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Add Member' })).toHaveCount(0)

  await addButton.click()
  await members.getByRole('combobox', { name: 'Select user…' }).selectOption(newUser.id)
  await members.getByRole('button', { name: 'Add to Team' }).click()

  const newMember = members.locator('.team-member-row').filter({ hasText: newUser.email })
  await expect(newMember.getByText(newUser.name, { exact: true })).toBeVisible()
  await expect(newMember.getByText(newUser.email, { exact: true })).toBeVisible()
  await expect(members.getByRole('combobox', { name: 'Select user…' })).toHaveCount(0)
  expect(addRequests).toBe(1)
})

test('owner sees an empty state when the directory has no available users', async ({ page }) => {
  await page.route('**/v1/users', async (route) => {
    const response = await route.fetch()
    await route.fulfill({ response, json: { items: [] } })
  })
  await signIn(page)
  await page.getByRole('button', { name: 'Team', exact: true }).click()

  const members = page.locator('section.brand-card').filter({ has: page.getByRole('heading', { name: 'Members', exact: true }) })
  await members.getByRole('button', { name: 'Add Member' }).click()
  await expect(members.getByText('No users available to add.')).toBeVisible()
  await expect(members.getByRole('combobox', { name: 'Select user…' })).toBeDisabled()
  await expect(members.getByRole('button', { name: 'Add to Team' })).toBeDisabled()
})

test('failed addition keeps selection open and restores controls', async ({ page }) => {
  let releaseRequest: (() => void) | undefined
  const pendingRequest = new Promise<void>((resolve) => { releaseRequest = resolve })
  let requestStarted = false

  await page.route('**/v1/users', async (route) => {
    const response = await route.fetch()
    const body = await response.json() as { items: Array<Record<string, unknown>> }
    await route.fulfill({ response, json: { ...body, items: [...body.items, newUser] } })
  })
  await page.route('**/v1/teams/*/members', async (route) => {
    if (route.request().method() !== 'POST') {
      await route.continue()
      return
    }
    requestStarted = true
    await pendingRequest
    await route.fulfill({ status: 500, body: 'Member add failed' })
  })

  await signIn(page)
  await page.getByRole('button', { name: 'Team', exact: true }).click()
  const members = page.locator('section.brand-card').filter({ has: page.getByRole('heading', { name: 'Members', exact: true }) })
  await members.getByRole('button', { name: 'Add Member' }).click()
  const select = members.getByRole('combobox', { name: 'Select user…' })
  await select.selectOption(newUser.id)
  const submit = members.getByRole('button', { name: 'Add to Team' })
  await submit.click()
  await expect.poll(() => requestStarted).toBe(true)
  await expect(submit).toBeDisabled()
  releaseRequest?.()

  await expect(page.locator('.status-banner__error')).toHaveText('Member add failed')
  await expect(select).toHaveValue(newUser.id)
  await expect(submit).toBeEnabled()
})
