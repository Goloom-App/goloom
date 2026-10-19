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

      const tabs = page.getByRole('tablist', { name: 'Team settings tabs' })
      await expect(tabs).toHaveClass(/view-toggle--scrollable/)
      if (role === 'owner') {
        await expect(tabs.getByRole('tab')).toHaveText(['General', 'Members', 'AI', 'Integrations', 'Audit log'])
        await expect(tabs.getByRole('tab', { name: 'General', exact: true })).toHaveAttribute('aria-selected', 'true')
        await expect(page.locator('.brand-card__title').first()).toHaveText('General')
        await expect(page.getByText('Manage members, update access, and transfer ownership', { exact: false })).toHaveCount(0)
        await expect(page.getByLabel('Team name')).toBeVisible()
        await expect(page.getByText('Only team owners can change settings here.')).toHaveCount(0)
        await tabs.getByRole('tab', { name: 'Members', exact: true }).click()
        await expect(page.getByRole('button', { name: 'Add Member' })).toBeVisible()
        await expect(page.getByLabel('Team name')).toHaveCount(0)
        await tabs.getByRole('tab', { name: 'AI', exact: true }).click()
        await expect(page.getByTestId('team-ai-toggle')).toBeVisible()
        await tabs.getByRole('tab', { name: 'Integrations', exact: true }).click()
        await expect(page.getByTestId('team-external-monitor-toggle')).toBeVisible()
        await tabs.getByRole('tab', { name: 'Audit log', exact: true }).click()
        await expect(page.getByRole('heading', { name: 'Audit log', exact: true })).toBeVisible()
        await tabs.getByRole('tab', { name: 'General', exact: true }).click()
        await expect(page.getByLabel('Team name')).toBeVisible()
      } else {
        await expect(tabs.getByRole('tab')).toHaveText(['Members'])
        await expect(tabs.getByRole('tab')).toHaveAttribute('aria-selected', 'true')
        await expect(page.locator('.brand-card__title').first()).toHaveText('Members')
        await expect(page.getByRole('button', { name: 'Add Member' })).toHaveCount(0)
        await expect(page.getByText('You are a member of this team. Only team owners can change settings here.')).toBeVisible()
        await expect(page.getByLabel('Team name')).toHaveCount(0)
      }
    })
  }
}
