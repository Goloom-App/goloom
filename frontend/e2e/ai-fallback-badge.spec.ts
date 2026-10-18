import { test, expect } from '@playwright/test'
import { e2eBootstrapToken } from './constants'
import { getFirstTeamId, openReviewQueue, signIn } from './helpers'

test('review queue shows template fallback reason from API', async ({ page, baseURL }) => {
  const teamId = await getFirstTeamId(baseURL!, e2eBootstrapToken())
  await page.route(`**/v1/teams/${teamId}/review-queue`, route => route.fulfill({ json: { items: [{
    id: 'fallback-draft', team_id: teamId, title: 'Fallback draft', content: 'Template content',
    scheduled_at: '2027-02-01T09:00:00Z', status: 'draft', source: 'automation',
    target_accounts: [], ai_fallback_reason: 'provider_failure', ai_job_id: 'failed-job-123',
  }, {
    id: 'normal-draft', team_id: teamId, title: 'Normal draft', content: 'AI content',
    scheduled_at: '2027-02-01T09:00:00Z', status: 'draft', source: 'automation', target_accounts: [],
  }] } }))
  await signIn(page)
  await openReviewQueue(page)
  const fallback = page.getByTestId('review-queue-item').filter({ hasText: 'Fallback draft' })
  await expect(fallback.getByTestId('ai-fallback-badge')).toContainText('Template fallback')
  await expect(fallback.getByTestId('ai-fallback-badge')).toHaveAttribute('title', /AI provider.*AI job: failed-job-123/)
  await expect(page.getByTestId('review-queue-item').filter({ hasText: 'Normal draft' }).getByTestId('ai-fallback-badge')).toHaveCount(0)
})

test('review queue shows article fetch notice', async ({ page, baseURL }) => {
  const teamId = await getFirstTeamId(baseURL!, e2eBootstrapToken())
  await page.route(`**/v1/teams/${teamId}/review-queue`, route => route.fulfill({ json: { items: [{
    id: 'article-note', team_id: teamId, title: 'Feed draft', content: 'AI used teaser',
    scheduled_at: '2027-02-01T09:00:00Z', status: 'draft', source: 'automation', target_accounts: [],
    ai_fallback_reason: 'article_fetch_failed', ai_job_id: 'article-job',
  }] } }))
  await signIn(page)
  await openReviewQueue(page)
  const badge = page.getByTestId('ai-fallback-badge')
  await expect(badge).toContainText('Article fetch failed')
  await expect(badge).toHaveAttribute('title', /Linked article could not be fetched/)
})
