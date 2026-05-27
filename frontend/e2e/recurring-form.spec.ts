import { test, expect } from '@playwright/test'
import { signIn } from './helpers'

test.describe('recurring posts form', () => {
  test.beforeEach(async ({ page }) => {
    await signIn(page)
    await page.getByRole('button', { name: /recurring posts|wiederkehrend/i }).click()
    await expect(page.getByRole('heading', { name: /recurring posts|wiederkehrend/i })).toBeVisible()
  })

  test('renders kind selector with weekly/monthly options', async ({ page }) => {
    await page.getByRole('button', { name: /new template|neue vorlage/i }).click()
    await expect(page.getByText(/weekly|wöchentlich/i)).toBeVisible()
    await expect(page.getByText(/monthly.*dom|monatlich.*tag/i)).toBeVisible()
    await expect(page.getByText(/monthly.*anchor|monatlich.*anker/i)).toBeVisible()
  })

  test('weekly recurrence saves correctly', async ({ page }) => {
    await page.getByRole('button', { name: /new template|neue vorlage/i }).click()

    await page.getByLabel(/title|titel/i).fill('E2E Weekly Test')
    await page.getByLabel(/content|inhalt/i).fill('Test content #{counter}')

    // select weekly kind
    await page.getByRole('radio', { name: /weekly|wöchentlich/i }).click()

    // toggle weekdays: Monday, Wednesday, Friday
    await page.getByRole('button', { name: /mon|mo/i }).click()
    await page.getByRole('button', { name: /wed|mi/i }).click()
    await page.getByRole('button', { name: /fri|fr/i }).click()

    // set time
    await page.getByLabel(/hour|stunde/i).fill('10')
    await page.getByLabel(/minute/i).fill('30')

    // select timezone
    await page.getByLabel(/timezone|zeitzone/i).fill('Europe/Berlin')

    // pick at least one account target
    const targetBtns = page.locator('.composer-destination-toggle')
    const count = await targetBtns.count()
    if (count > 0) {
      await targetBtns.first().click()
    }

    await page.getByRole('button', { name: /create|erstellen/i }).click()
    await expect(page.getByText('E2E Weekly Test')).toBeVisible({ timeout: 10_000 })
  })

  test('monthly_dom recurrence saves correctly', async ({ page }) => {
    await page.getByRole('button', { name: /new template|neue vorlage/i }).click()

    await page.getByLabel(/title|titel/i).fill('E2E Monthly Test')
    await page.getByLabel(/content|inhalt/i).fill('Monthly #{counter}')

    await page.getByRole('radio', { name: /monthly.*dom|monatlich.*tag/i }).click()

    // set day of month
    await page.getByLabel(/day of month|tag des monats/i).fill('15')

    await page.getByLabel(/hour|stunde/i).fill('14')
    await page.getByLabel(/minute/i).fill('0')
    await page.getByLabel(/timezone|zeitzone/i).fill('UTC')

    const targetBtns = page.locator('.composer-destination-toggle')
    const count = await targetBtns.count()
    if (count > 0) {
      await targetBtns.first().click()
    }

    await page.getByRole('button', { name: /create|erstellen/i }).click()
    await expect(page.getByText('E2E Monthly Test')).toBeVisible({ timeout: 10_000 })
  })

  test('validates required fields', async ({ page }) => {
    await page.getByRole('button', { name: /new template|neue vorlage/i }).click()
    await page.getByRole('button', { name: /create|erstellen/i }).click()
    await expect(page.getByText(/required fields|erforderlich/i)).toBeVisible()
  })

  test('preview shows upcoming occurrences', async ({ page }) => {
    await page.getByRole('button', { name: /new template|neue vorlage/i }).click()

    await page.getByLabel(/title|titel/i).fill('Preview Test')
    await page.getByLabel(/content|inhalt/i).fill('Preview #{counter}')

    await page.getByRole('radio', { name: /weekly|wöchentlich/i }).click()
    await page.getByRole('button', { name: /mon|mo/i }).click()
    await page.getByLabel(/hour|stunde/i).fill('9')
    await page.getByLabel(/minute/i).fill('0')
    await page.getByLabel(/timezone|zeitzone/i).fill('UTC')

    // preview should show 5 upcoming dates
    const previewItems = page.locator('.occurrence-preview__item')
    await expect(previewItems).toHaveCount(5)
  })
})
