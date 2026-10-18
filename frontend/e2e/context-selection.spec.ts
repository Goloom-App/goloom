import { expect, test } from '@playwright/test'
import { signIn } from './helpers'

test('campaign editor saves specific, none and inherited knowledge, retaining missing IDs visibly', async ({ page }) => {
  await page.route('**/v1/teams', async route => {
    const data = await (await route.fetch()).json()
    await route.fulfill({ json: { items: data.items.map((team: object) => ({ ...team, is_ai_enabled: true })) } })
  })
  let format = { id: 'selected-format', name: 'Selected format', weekday: 2, structure: { topic: 'Topic' }, required_hashtags: [], is_active: true, knowledge_source_ids: ['deleted'] as string[] | null }
  await page.route('**/v1/teams/*/knowledge-sources', route => route.fulfill({ json: { items: [{ id: 'source', name: 'Brand facts' }] } }))
  await page.route('**/v1/teams/*/campaign-formats', route => route.fulfill({ json: { items: [format] } }))
  await page.route('**/v1/teams/*/campaign-formats/selected-format', async route => {
    format = { ...format, ...route.request().postDataJSON() }
    await route.fulfill({ json: format })
  })
  await signIn(page)
  await page.getByRole('button', { name: 'Campaign Formats', exact: true }).click()
  const open = () => page.getByTestId('campaign-format-selected-format').locator('button').first().click()
  await open()
  await expect(page.getByTestId('knowledge-missing')).toBeVisible()
  // Removing a missing source unmounts its checkbox instead of leaving it unchecked.
  await page.getByLabel('deleted (unavailable)').click()
  await expect(page.getByLabel('deleted (unavailable)')).toHaveCount(0)
  await expect(page.getByTestId('knowledge-missing')).toHaveCount(0)
  await page.getByLabel('Brand facts', { exact: true }).check()
  await page.getByTestId('campaign-dialog-save').click()
  await expect.poll(() => format.knowledge_source_ids).toEqual(['source'])
  await open()
  await expect(page.getByLabel('Brand facts', { exact: true })).toBeChecked()
  await page.getByTestId('knowledge-mode').selectOption('none')
  await page.getByTestId('campaign-dialog-save').click()
  await expect.poll(() => format.knowledge_source_ids).toEqual([])
  await open()
  await page.getByTestId('knowledge-mode').selectOption('inherit')
  await page.getByTestId('campaign-dialog-save').click()
  await expect.poll(() => format.knowledge_source_ids).toBeNull()
})
