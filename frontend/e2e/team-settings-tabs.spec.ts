import { expect, test } from '@playwright/test'
import { createAITeam, signIn } from './helpers'
import { e2eBootstrapToken } from './constants'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || `http://127.0.0.1:${process.env.PLAYWRIGHT_SERVER_PORT || '18080'}`

for (const viewer of [false, true]) {
  test(`team switch ${viewer ? 'falls back from AI to read-only members' : 'keeps the AI tab'}`, async ({ page }) => {
    const first = await createAITeam(baseURL, e2eBootstrapToken())
    const second = await createAITeam(baseURL, e2eBootstrapToken())
    if (viewer) {
      await page.route(`**/v1/teams/${second}/members`, async (route) => {
        const response = await route.fetch()
        const body = await response.json() as { items: Array<Record<string, unknown>> }
        await route.fulfill({ response, json: { ...body, items: body.items.map((member) => ({ ...member, role: 'viewer' })) } })
      })
    }
    await signIn(page)
    await page.goto(`/?team=${first}`)
    await page.getByRole('button', { name: 'Team', exact: true }).click()
    await page.getByRole('tab', { name: 'AI', exact: true }).click()
    await expect(page.getByTestId('team-ai-toggle')).toBeVisible()

    const response = await page.request.get('/v1/teams', { headers: { Authorization: `Bearer ${e2eBootstrapToken()}` } })
    const teams = await response.json() as { items: { id: string; name: string }[] }
    await page.locator('.sidebar-team-selector').click()
    await page.getByRole('menuitem', { name: teams.items.find((team) => team.id === second)!.name, exact: true }).click()

    if (viewer) {
      await expect(page.getByRole('tab')).toHaveText(['Members'])
      await expect(page.getByRole('tab', { name: 'Members', exact: true })).toHaveAttribute('aria-selected', 'true')
      await expect(page.getByText('You are a member of this team. Only team owners can change settings here.')).toBeVisible()
      await expect(page.getByTestId('team-ai-toggle')).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Add Member' })).toHaveCount(0)
      await expect(page.getByRole('tabpanel').getByRole('combobox')).toHaveCount(0)
      // The fallback becomes the selected tab, not a temporarily hidden owner tab.
      await page.locator('.sidebar-team-selector').click()
      await page.getByRole('menuitem', { name: teams.items.find((team) => team.id === first)!.name, exact: true }).click()
      await expect(page.getByRole('tab', { name: 'Members', exact: true })).toHaveAttribute('aria-selected', 'true')
      await expect(page.getByRole('button', { name: 'Add Member' })).toBeVisible()
    } else {
      await expect(page.getByRole('tab', { name: 'AI', exact: true })).toHaveAttribute('aria-selected', 'true')
      await expect(page.getByTestId('team-ai-toggle')).toBeVisible()
    }
  })
}
