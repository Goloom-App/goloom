import { expect, test } from '@playwright/test'
import { createAITeam, signIn } from './helpers'
import { e2eBootstrapToken } from './constants'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || `http://127.0.0.1:${process.env.PLAYWRIGHT_SERVER_PORT || '18080'}`

for (const section of ['Admin', 'Analytics', 'Team', 'AI Profile', 'Automation', 'Calendar']) {
  test(`${section}: shared page tabs support click and keyboard navigation`, async ({ page }) => {
    const team = await createAITeam(baseURL, e2eBootstrapToken())
    await signIn(page)
    await page.goto(`/?team=${team}`)
    if (section === 'Admin') {
      await page.getByTestId('user-menu-trigger').click()
      await page.getByRole('menuitem', { name: 'Admin', exact: true }).click()
    } else {
      await page.getByRole('button', { name: section, exact: true }).click()
    }
    const tabs = page.getByRole('tablist').getByRole('tab')
    await expect(tabs.first()).toHaveAttribute('aria-selected', 'true')
    await tabs.nth(1).click()
    await expect(tabs.nth(1)).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('tabpanel')).toHaveAttribute('aria-labelledby', await tabs.nth(1).getAttribute('id') as string)
    await expect(tabs.nth(1)).toHaveAttribute('tabindex', '0')
    await expect(tabs.first()).toHaveAttribute('tabindex', '-1')
    await tabs.nth(1).press('Home')
    await expect(tabs.first()).toBeFocused()
    await tabs.first().press('ArrowLeft')
    await expect(tabs.last()).toBeFocused()
    await expect(tabs.last()).toHaveAttribute('aria-selected', 'true')
    await tabs.last().press('ArrowRight')
    await expect(tabs.first()).toBeFocused()
    await tabs.first().press('End')
    await expect(tabs.last()).toBeFocused()
    if (section === 'Admin' || section === 'Analytics') {
      await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1)
    }
    await page.setViewportSize({ width: 375, height: 812 })
    const list = page.getByRole('tablist')
    for (const theme of ['light', 'dark']) {
      await page.evaluate((value) => document.documentElement.setAttribute('data-theme', value), theme)
      await expect(list).toHaveCSS('overflow-x', 'auto')
      expect(await list.evaluate((el) => el.getBoundingClientRect().right)).toBeLessThanOrEqual(375)
      await tabs.first().focus()
      await tabs.first().press('End')
      await expect(tabs.last()).toBeInViewport()
      await expect(tabs.last()).toHaveAttribute('aria-selected', 'true')
    }
    // Guided tours keep their stable anchors.
    if (section === 'Admin') await expect(page.locator('[data-tour="admin-tab-status"]')).toHaveRole('tab')
    if (section === 'Team') await expect(page.locator('[data-tour="team-tab-general"]')).toHaveRole('tab')
  })
}
