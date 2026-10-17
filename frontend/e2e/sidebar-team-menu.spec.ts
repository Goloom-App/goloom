import { expect, test } from '@playwright/test'
import { signIn } from './helpers'

for (const width of [1280, 1024]) {
  test(`many-team menu stays bounded and Create team reachable at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 500 })
    // Inflate only this browser's team response, without polluting the shared DB.
    await page.route('**/v1/teams', async (route) => {
      const response = await route.fetch()
      const data = await response.json()
      await route.fulfill({ response, json: {
        ...data,
        items: [...data.items, ...Array.from({ length: 40 }, (_, i) => ({
          ...data.items[0], id: `overflow-${i}`, name: `Overflow team ${i}`,
        }))],
      } })
    })
    await signIn(page)
    const trigger = page.locator('.sidebar-team-selector')
    await trigger.click()
    const menu = page.getByRole('menu')
    const create = page.getByTestId('sidebar-create-team')
    await expect(menu).toBeInViewport({ ratio: 1 })
    await expect(create).toBeInViewport({ ratio: 1 })
    const list = menu.locator('.sidebar-team-menu__list')
    await expect.poll(() => list.evaluate(el => el.scrollHeight > el.clientHeight)).toBe(true)
    // Last team must remain reachable by pointer scrolling and keyboard.
    const lastTeam = page.getByRole('menuitem', { name: 'Overflow team 39', exact: true })
    await lastTeam.scrollIntoViewIfNeeded()
    await expect.poll(() => list.evaluate(el => el.scrollTop)).toBeGreaterThan(0)
    await expect(lastTeam).toBeInViewport({ ratio: 1 })
    await expect(create).toBeInViewport({ ratio: 1 })
    await create.click()
    const modal = page.getByTestId('create-team-modal')
    await expect(modal).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(modal).toBeHidden()
    await trigger.focus()
    await page.keyboard.press('ArrowDown')
    await page.keyboard.press('End')
    await expect(create).toBeFocused()
    await page.keyboard.press('ArrowUp')
    await expect(lastTeam).toBeFocused()
    await expect(lastTeam).toBeInViewport({ ratio: 1 })
    await page.keyboard.press('End')
    await expect(create).toBeFocused()
    await page.keyboard.press('Enter')
    await expect(modal).toBeVisible()
  })
}
