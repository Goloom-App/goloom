import { expect, test } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { createAITeam, signIn } from './helpers'

test('switching teams clears brand profile and unsaved knowledge drafts before saving', async ({ page }) => {
  test.setTimeout(90_000)
  const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'http://127.0.0.1:18080'
  const token = e2eBootstrapToken()
  const teamA = await createAITeam(baseURL, token)
  const teamB = await createAITeam(baseURL, token)
  const teamsResponse = await fetch(`${baseURL}/v1/teams`, { headers: { Authorization: `Bearer ${token}` } })
  expect(teamsResponse.ok).toBeTruthy()
  const { items: teams } = await teamsResponse.json() as { items: { id: string; name: string }[] }
  const teamBName = teams.find((team) => team.id === teamB)!.name
  const profile = {
    id: 'team-a-profile', team_id: teamA,
    style_metadata: {
      identity: { archetype: 'Team A brand', persona: 'Team A persona', industry: 'Team A industry', main_value: 'Team A value', target_audience: 'Team A audience' },
      language_dna: { sentence_style: 'Team A sentences', humor_style: 'Team A humor', preferred_words: ['TeamAWord'], signature_phrases: [], anti_ai_override: false },
      reach_strategy: { hook_style: 'Team A hook', cta_focus: 'Team A CTA' },
      formatting_rules: [], banned_words: [], max_hashtags: 8, preferred_language: 'en',
    },
    auto_publish_enabled: true,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
  }
  await page.route(`**/v1/teams/${teamA}/profile`, (route) => route.fulfill({ json: profile }))
  await page.route(`**/v1/teams/${teamB}/profile`, (route) => route.request().method() === 'GET'
    ? route.fulfill({ status: 404, json: { error: 'not_found' } })
    : route.fulfill({ json: { ...profile, ...route.request().postDataJSON(), team_id: teamB } }))

  await signIn(page)
  await page.goto(`/?team=${teamA}`)
  await page.getByRole('button', { name: 'AI Profile', exact: true }).click()
  await expect(page.getByTestId('brand-archetype')).toHaveValue('Team A brand')
  const knowledge = page.getByTestId('brand-knowledge-section')
  await knowledge.getByPlaceholder('z. B. Produkt-FAQ').fill('Team A unsaved source')
  await knowledge.getByPlaceholder('Fakten, Zitate, Produktinfos…').fill('Team A secret facts')

  const emptyProfile = page.waitForResponse((response) => response.url().endsWith(`/v1/teams/${teamB}/profile`) && response.request().method() === 'GET')
  await page.locator('.sidebar-team-selector').click()
  await page.getByRole('menuitem', { name: teamBName, exact: true }).click()
  expect((await emptyProfile).status()).toBe(404)
  for (const field of ['archetype', 'persona', 'industry', 'main-value', 'audience', 'sentence-style', 'humor', 'hook', 'cta']) {
    await expect(page.getByTestId(`brand-${field}`)).toHaveValue('')
  }
  await expect(knowledge.getByPlaceholder('z. B. Produkt-FAQ')).toHaveValue('')
  await expect(knowledge.getByPlaceholder('Fakten, Zitate, Produktinfos…')).toHaveValue('')
  await expect(page.locator('label[title="Auto-Publish"] input[type="checkbox"]')).not.toBeChecked()

  const saved = page.waitForRequest((request) => request.url().endsWith(`/v1/teams/${teamB}/profile`) && request.method() === 'PUT')
  await page.getByTestId('brand-save-setup').click()
  expect((await saved).postDataJSON()).toEqual({
    style_metadata: {
      identity: { archetype: '', persona: '', industry: '', main_value: '', target_audience: '' },
      language_dna: { sentence_style: '', humor_style: '', preferred_words: [], signature_phrases: [], anti_ai_override: false },
      reach_strategy: { hook_style: '', cta_focus: '' },
      formatting_rules: [], banned_words: [], max_hashtags: 3, preferred_language: 'de',
    },
    auto_publish_enabled: false,
  })
  await expect(page.getByTestId('brand-status-success')).toBeVisible()
})
