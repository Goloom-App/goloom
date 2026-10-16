import { test, expect, type APIRequestContext } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { ensureE2EAccount, getFirstTeamId, signIn } from './helpers'

async function deleteTemplatesByTitle(request: APIRequestContext, baseURL: string, teamId: string, title: string) {
  const headers = { Authorization: `Bearer ${e2eBootstrapToken()}` }
  const res = await request.get(`${baseURL}/v1/teams/${teamId}/post-templates`, { headers })
  expect(res.ok()).toBeTruthy()
  const { items = [] } = (await res.json()) as { items?: { id: string; title: string }[] }
  for (const item of items.filter((i) => i.title === title)) {
    const del = await request.delete(`${baseURL}/v1/teams/${teamId}/post-templates/${item.id}`, { headers })
    expect(del.ok()).toBeTruthy()
  }
}

test.describe('recurring posts form', () => {
  let teamId = ''

  test.beforeEach(async ({ page, baseURL }) => {
    if (!baseURL) throw new Error('baseURL missing')
    // Other specs share the database and may already have seeded an account;
    // ensure one exists so saving is deterministic regardless of spec order.
    teamId = await getFirstTeamId(baseURL, e2eBootstrapToken())
    await ensureE2EAccount(baseURL, e2eBootstrapToken(), teamId)
    await signIn(page)
    await page.getByRole('button', { name: /^Automation$/i }).click()
    await expect(page.getByTestId('automation-view')).toBeVisible()
  })

  test('renders kind selector with weekly/monthly options', async ({ page }) => {
    await page.getByTestId('recurring-add-template').click()
    await expect(page.getByRole('radio', { name: /weekly|wöchentlich/i })).toBeVisible()
    await expect(page.getByRole('radio', { name: /monthly.*day of month|monatlich.*tag des monats/i })).toBeVisible()
    await expect(page.getByRole('radio', { name: /monthly.*anchor|monatlich.*anker/i })).toBeVisible()
  })

  test('weekly recurrence saves correctly', async ({ page, request, baseURL }) => {
    await page.getByTestId('recurring-add-template').click()

    await page.getByLabel(/title|titel/i).fill('E2E Weekly Test')
    await page.getByLabel(/content|inhalt/i).fill('Test content #{counter}')

    // select weekly kind
    await page.getByRole('radio', { name: /weekly|wöchentlich/i }).click()

    // toggle weekdays: Monday, Wednesday, Friday
    await page.getByRole('button', { name: /mon|mo/i }).click()
    await page.getByRole('button', { name: /^(we|mi)$/i }).click()
    await page.getByRole('button', { name: /fri|fr/i }).click()

    // set time
    await page.locator('.recurrence-form__field').filter({ hasText: /hour|stunde/i }).locator('input').fill('10')
    await page.locator('.recurrence-form__field').filter({ hasText: /minute/i }).locator('input').fill('30')

    // select timezone
    await page.locator('.recurrence-form__field').filter({ hasText: /timezone|zeitzone/i }).locator('input').fill('Europe/Berlin')

    await page.locator('.composer-destination-toggle').first().click()

    await page.getByRole('button', { name: /create|erstellen/i }).evaluate(el => (el as HTMLButtonElement).click())
    await expect(page.getByText(/template created|vorlage erstellt/i)).toBeVisible()
    await deleteTemplatesByTitle(request, baseURL!, teamId, 'E2E Weekly Test')
  })

  test('monthly_dom recurrence saves correctly', async ({ page, request, baseURL }) => {
    await page.getByTestId('recurring-add-template').click()

    await page.getByLabel(/title|titel/i).fill('E2E Monthly Test')
    await page.getByLabel(/content|inhalt/i).fill('Monthly #{counter}')

    await page.getByRole('radio', { name: /monthly.*day of month|monatlich.*tag des monats/i }).click()

    // set day of month
    await page.locator('.recurrence-form__field').filter({ hasText: /day of month|tag des monats/i }).locator('input').fill('15')

    await page.locator('.recurrence-form__field').filter({ hasText: /hour|stunde/i }).locator('input').fill('14')
    await page.locator('.recurrence-form__field').filter({ hasText: /minute/i }).locator('input').fill('0')
    await page.locator('.recurrence-form__field').filter({ hasText: /timezone|zeitzone/i }).locator('input').fill('UTC')

    await page.locator('.composer-destination-toggle').first().click()

    await page.getByRole('button', { name: /create|erstellen/i }).evaluate(el => (el as HTMLButtonElement).click())
    await expect(page.getByText(/template created|vorlage erstellt/i)).toBeVisible()
    await deleteTemplatesByTitle(request, baseURL!, teamId, 'E2E Monthly Test')
  })

  test('validates required fields', async ({ page }) => {
    await page.getByTestId('recurring-add-template').click()
    await page.getByRole('button', { name: /create|erstellen/i }).evaluate(el => (el as HTMLButtonElement).click())
    await expect(page.getByText(/required|erforderlich/i)).toBeVisible()
  })

  test('monthly ordinal weekday supports multiple occurrences', async ({ page }) => {
    await page.getByTestId('recurring-add-template').click()
    await page.getByRole('radio', { name: /monthly.*ordinal|monatlich.*wochentag/i }).click()
    await page.getByTestId('recurring-ordinal-0-1').click()
    await page.getByTestId('recurring-ordinal-weekday-0-fri').click()
    await page.getByTestId('recurring-add-occurrence').click()
    await page.getByTestId('recurring-ordinal-1-3').click()
    await page.getByTestId('recurring-ordinal-weekday-1-mon').click()
    await expect(page.locator('.occurrence-preview__item')).toHaveCount(5)
  })

  test('preview shows upcoming occurrences', async ({ page }) => {
    await page.getByTestId('recurring-add-template').click()

    await page.getByLabel(/title|titel/i).fill('Preview Test')
    await page.getByLabel(/content|inhalt/i).fill('Preview #{counter}')

    await page.getByRole('radio', { name: /weekly|wöchentlich/i }).click()
    // Monday is already selected by default (weekdays: [1]), skip toggling it
    await page.locator('.recurrence-form__field').filter({ hasText: /hour|stunde/i }).locator('input').fill('9')
    await page.locator('.recurrence-form__field').filter({ hasText: /minute/i }).locator('input').fill('0')
    await page.locator('.recurrence-form__field').filter({ hasText: /timezone|zeitzone/i }).locator('input').fill('UTC')

    // preview should show 5 upcoming dates
    const previewItems = page.locator('.occurrence-preview__item')
    await expect(previewItems).toHaveCount(5)
  })
})
