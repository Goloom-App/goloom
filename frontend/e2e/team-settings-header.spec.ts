import { expect, test } from '@playwright/test'

import { signIn } from './helpers'

const viewports = [
  { name: 'desktop', width: 1280, height: 800 },
  { name: 'mobile', width: 390, height: 844 },
]

for (const viewport of viewports) {
  for (const role of ['owner', 'non-owner'] as const) {
    test(`${role} team settings omit duplicate header card on ${viewport.name}`, async ({ page }) => {
      await page.setViewportSize({ width: viewport.width, height: viewport.height })

      if (role === 'non-owner') {
        await page.route('**/v1/teams/*/members', async (route) => {
          const response = await route.fetch()
          if (route.request().method() !== 'GET' || !response.ok()) {
            await route.fulfill({ response })
            return
          }

          const body = await response.json() as {
            items?: Array<Record<string, unknown> & { role?: string }>
          }
          await route.fulfill({
            response,
            json: {
              ...body,
              items: body.items?.map((member) => ({ ...member, role: 'viewer' })),
            },
          })
        })
      }

      await signIn(page)
      if (viewport.name === 'mobile') {
        await page.getByRole('button', { name: 'More', exact: true }).click()
        await page.getByRole('button', { name: 'Team', exact: true }).click()
      } else {
        await page.getByRole('button', { name: 'Team', exact: true }).click()
      }

      await expect(page.getByRole('heading', { level: 1, name: 'Team settings', exact: true })).toBeVisible()
      await expect(page.getByRole('heading', { level: 2, name: 'Team settings', exact: true })).toHaveCount(0)

      if (role === 'owner') {
        await expect(page.locator('.brand-card__title').first()).toHaveText('General')
        await expect(page.getByText('Manage members, update access, and transfer ownership', { exact: false })).toHaveCount(0)
        await expect(page.getByLabel('Team name')).toBeVisible()
        await expect(page.getByText('Only team owners can change settings here.')).toHaveCount(0)
      } else {
        await expect(page.locator('.brand-card__title').first()).toHaveText('Members')
        await expect(page.getByText('You are a member of this team. Only team owners can change settings here.')).toBeVisible()
        await expect(page.getByLabel('Team name')).toHaveCount(0)
      }
    })
  }
}
