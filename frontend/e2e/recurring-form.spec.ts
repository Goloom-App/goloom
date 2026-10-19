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

  test('AI preview shows occurrence output without creating posts or importing', async ({ page, baseURL }) => {
    // Keep Monday first and Tuesday second, regardless of the CI run date.
    // Removing Tuesday below must invalidate the selected occurrence.
    await page.clock.setFixedTime(new Date('2026-10-18T12:00:00Z'))
    const accountId = await ensureE2EAccount(baseURL!, e2eBootstrapToken(), teamId)
    const previewRequests: Record<string, unknown>[] = []
    const sideEffectRequests: string[] = []
    await page.route('**/v1/teams', async route => {
      const response = await route.fetch()
      const data = await response.json() as { items: Record<string, unknown>[] }
      await route.fulfill({ response, json: { ...data, items: data.items.map(item => ({ ...item, is_ai_enabled: true })) } })
    })
    await page.route(`**/v1/teams/${teamId}/post-templates`, route => route.fulfill({ json: { items: [{
      id: 'preview-template', team_id: teamId, author_user_id: 'user', title: 'Event recap',
      content: 'Join us on {day}/{month}: {counter}', recurrence_json: JSON.stringify({ kind: 'weekly', weekdays: [1], hour: 9, minute: 0, timezone: 'UTC' }),
      visibility: 'public', target_account_ids: [accountId], enabled: true, ai_enhance_enabled: true,
      output_mode: 'draft', prompt_hint: 'Keep it warm.', counter_next: 3,
      announcement_enabled: true, announcement_content: 'See you before the event.', announcement_counter_next: 4,
      created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    }] } }))
    await page.route(`**/v1/teams/${teamId}/post-templates/preview-template/ai-preview`, async route => {
      previewRequests.push(route.request().postDataJSON() as Record<string, unknown>)
      await route.fulfill({ status: 202, json: { job_id: 'recurring-preview-job', status: 'pending' } })
    })
    await page.route(`**/v1/teams/${teamId}/ai-jobs/recurring-preview-job`, route => route.fulfill({ json: {
      id: 'recurring-preview-job', team_id: teamId, author_user_id: 'user', type: 'voice_engine', status: 'completed',
      payload: {
        preview: {
          source: { occurrence_at: '2026-10-19T09:00:00Z', post_kind: 'main' },
          template: 'Join us on 19/10: 3', generation_inputs: { prompt_hint: 'Keep it warm.' },
        },
        params: { target_account_ids: [accountId] },
      },
      result: { content: 'Join our event next Monday!', account_content_override: {} },
      error_message: null, created_at: '2026-10-18T00:00:00Z', updated_at: '2026-10-18T00:00:00Z', completed_at: '2026-10-18T00:00:01Z',
    } }))
    await page.route(`**/v1/teams/${teamId}/posts**`, route => {
      if (route.request().method() !== 'POST') return route.continue()
      sideEffectRequests.push(`${route.request().method()} ${route.request().url()}`)
      return route.fulfill({ status: 500, json: { error: 'preview must not create posts' } })
    })
    await page.route(`**/v1/teams/${teamId}/**/import**`, route => {
      if (route.request().method() !== 'POST') return route.continue()
      sideEffectRequests.push(`${route.request().method()} ${route.request().url()}`)
      return route.fulfill({ status: 500, json: { error: 'preview must not import' } })
    })

    await page.reload()
    await page.getByRole('button', { name: /^edit$|bearbeiten/i }).click()
    await page.getByTestId('recurring-content').fill('Edited event {counter}')
    await page.getByTestId('recurring-counter-start').fill('8')
    await page.getByTestId('recurring-weekday-tue').click()
    await page.getByTestId('recurring-announcement-counter-start').fill('11')
    await page.getByTestId('recurring-ai-prompt').fill('Changed preview prompt.')
    await page.getByTestId('recurring-ai-preview').getByRole('button', { name: /generate preview|vorschau generieren/i }).click()
    await expect(page.getByTestId(`recurring-ai-preview-account-${accountId}`)).toContainText('Join our event next Monday!')
    await expect(page.getByTestId(`recurring-ai-preview-account-${accountId}`)).toContainText(/\d+ characters|\d+ Zeichen/)
    expect(previewRequests[0]).toMatchObject({
      post_kind: 'main',
      config: {
        content: 'Edited event {counter}',
        prompt_hint: 'Changed preview prompt.',
        target_account_ids: [accountId],
        counter_next: 8,
        announcement_counter_next: 11,
      },
    })
    const previewConfig = previewRequests[0]?.config as Record<string, unknown>
    expect(JSON.parse(previewConfig.recurrence_json as string)).toMatchObject({ weekdays: [1, 2] })
    expect(previewRequests[0]?.occurrence_at).toBeTruthy()
    expect(sideEffectRequests).toEqual([])

    await page.getByTestId('recurring-preview-kind-announcement').click()
    await page.getByTestId('recurring-ai-preview').getByRole('button', { name: /generate preview|vorschau generieren/i }).click()
    expect(previewRequests[1]).toMatchObject({ post_kind: 'announcement' })

    const occurrence = page.getByTestId('recurring-preview-occurrence')
    const firstOccurrence = await occurrence.inputValue()
    await occurrence.selectOption({ index: 1 })
    await page.getByTestId('recurring-weekday-tue').click()
    await expect(occurrence).toHaveValue(firstOccurrence)
    await page.getByTestId('recurring-announcement-enabled').click()
    await expect(page.getByTestId('recurring-preview-kind-main')).toHaveAttribute('aria-selected', 'true')
    await page.getByRole('dialog').getByRole('button', { name: /cancel|abbrechen/i }).last().click()
    await page.getByRole('button', { name: /^edit$|bearbeiten/i }).click()
    await expect(occurrence).toHaveValue(firstOccurrence)
    await expect(page.getByTestId('recurring-preview-kind-main')).toHaveAttribute('aria-selected', 'true')
  })
})
