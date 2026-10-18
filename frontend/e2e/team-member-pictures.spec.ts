import { expect, test } from '@playwright/test'

import { signIn } from './helpers'

for (const viewport of [{ name: 'desktop', width: 1280, height: 800 }, { name: 'mobile', width: 390, height: 844 }]) {
  for (const picture of ['https://example.test/member.png', ''] as const) {
    test(`${viewport.name} member picture ${picture ? 'loads or falls back on error' : 'falls back when absent'}`, async ({ page }) => {
      await page.setViewportSize({ width: viewport.width, height: viewport.height })
      await page.route('**/v1/users', async (route) => {
        const response = await route.fetch()
        const body = await response.json() as { items: Array<Record<string, unknown>> }
        await route.fulfill({ response, json: { ...body, items: body.items.map((user) => ({ ...user, picture_url: picture })) } })
      })
      let failImage = false
      if (picture) await page.route('**/member.png*', (route) => failImage
        ? route.abort()
        : route.fulfill({ status: 200, contentType: 'image/gif', body: Buffer.from('R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=', 'base64') }))

      await signIn(page)
      if (viewport.name === 'mobile') await page.getByRole('button', { name: 'More', exact: true }).click()
      await page.getByRole('button', { name: 'Team', exact: true }).click()
      const member = page.locator('.team-member-row').first()
      await expect(member).toBeVisible()
      await expect(member.locator('.member-identity__text')).toContainText('@')
      const role = member.getByRole('combobox')
      await expect(role).toBeVisible()
      await role.selectOption('viewer')
      await expect(member.getByRole('button', { name: 'Apply' })).toBeEnabled()
      const bounds = await member.boundingBox()
      expect(bounds && bounds.x + bounds.width).toBeLessThanOrEqual(viewport.width + 1)

      if (picture) {
        const image = member.locator('img.avatar__img')
        await expect(image).toHaveAttribute('src', picture)
        await expect(image).toBeVisible()
        failImage = true
        await image.evaluate((element: HTMLImageElement) => { element.src = `${element.src}?broken` })
        await expect(member.locator('.avatar__placeholder')).toBeVisible()
      } else {
        await expect(member.locator('.avatar__placeholder')).toBeVisible()
        await expect(member.locator('img.avatar__img')).toHaveCount(0)
      }
    })
  }
}
