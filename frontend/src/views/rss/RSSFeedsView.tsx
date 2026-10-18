import { useEffect, useRef, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { X, Plus, Trash2, Rss, Clock, Pencil, MoreVertical, Target } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { useTranslation } from 'react-i18next'

import { getViewContext, setViewFocus, setViewVisible } from '../../components/ai/viewContextBridge'
import { DestinationPicker } from '../../components/ai/DestinationPicker'
import { AutomationAIPreview } from '../../components/ai/AutomationAIPreview'
import { Segmented, ToggleSwitch } from '../../components/ui'
import { getApiClient, useCreateRSSFeed, useDeleteRSSFeed, useRSSFeeds, useUpdateRSSFeed } from '../../hooks/useAI'
import type { AccountRecord, AutomationOutputMode, RSSFeedConfig, RSSInitialSyncMode, TeamRecord } from '../../types'
import type { BackendRSSPreviewItem } from '../../api'

interface RSSFeedsViewProps {
  team: TeamRecord
  accounts: AccountRecord[]
  canEdit: boolean
}

type FeedFormState = {
  name: string
  feedUrl: string
  isActive: boolean
  aiEnhanceEnabled: boolean
  articleEnrichmentEnabled: boolean
  contentTemplate: string
  titleTemplate: string
  titleHint: string
  outputMode: AutomationOutputMode
  maxPostsPerDay: number
  promptHint: string
  targetAccountIds: string[]
  initialSyncMode: RSSInitialSyncMode
}

const defaultTemplate = '{title}\n\n{link}'
const defaultTitleTemplate = '{title}'

const emptyFeedForm = (): FeedFormState => ({
  name: '',
  feedUrl: '',
  isActive: true,
  aiEnhanceEnabled: false,
  articleEnrichmentEnabled: false,
  contentTemplate: defaultTemplate,
  titleTemplate: defaultTitleTemplate,
  titleHint: '',
  outputMode: 'draft',
  maxPostsPerDay: 10,
  promptHint: '',
  targetAccountIds: [],
  initialSyncMode: 'baseline',
})

function feedToForm(feed: RSSFeedConfig): FeedFormState {
  return {
    name: feed.name,
    feedUrl: feed.feedUrl,
    isActive: feed.isActive,
    aiEnhanceEnabled: feed.aiEnhanceEnabled,
    articleEnrichmentEnabled: feed.articleEnrichmentEnabled,
    contentTemplate: feed.contentTemplate || defaultTemplate,
    titleTemplate: feed.titleTemplate || defaultTitleTemplate,
    titleHint: feed.titleHint ?? '',
    outputMode: feed.outputMode,
    maxPostsPerDay: feed.maxPostsPerDay ?? 10,
    promptHint: feed.promptHint,
    targetAccountIds: feed.targetAccountIds,
    initialSyncMode: feed.initialSyncMode,
  }
}

const outputModeLabel: Record<AutomationOutputMode, string> = {
  draft: 'Draft (review)',
  scheduled: 'Scheduled',
  publish_now: 'Publish now',
}

export function RSSFeedsView({ team, accounts, canEdit }: RSSFeedsViewProps) {
  const { t } = useTranslation()
  const { data: rssFeeds, isLoading: feedsLoading } = useRSSFeeds(team.id)
  const createFeed = useCreateRSSFeed()
  const updateFeed = useUpdateRSSFeed()
  const deleteFeed = useDeleteRSSFeed()

  const editorBaselineRef = useRef('')
  const [isFeedDialogOpen, setIsFeedDialogOpen] = useState(false)
  const [editingFeedId, setEditingFeedId] = useState<string | null>(null)
  const [feedForm, setFeedForm] = useState<FeedFormState>(emptyFeedForm)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [previewFeedId, setPreviewFeedId] = useState<string | null>(null)
  const [previewItems, setPreviewItems] = useState<BackendRSSPreviewItem[]>([])
  const [previewItemKey, setPreviewItemKey] = useState('')
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState('')

  const openFeedPreview = async (feedId: string) => {
    setPreviewFeedId(feedId)
    setPreviewItems([])
    setPreviewItemKey('')
    setPreviewError('')
    setPreviewLoading(true)
    try {
      const response = await getApiClient().listRSSPreviewItems(team.id, feedId)
      setPreviewItems(response.items ?? [])
      setPreviewItemKey(response.items?.[0]?.item_key ?? '')
    } catch (cause) {
      setPreviewError(cause instanceof Error ? cause.message : t('automationPreview.failed'))
    } finally {
      setPreviewLoading(false)
    }
  }

  useEffect(() => {
    if (!isFeedDialogOpen) return
    const serialized = JSON.stringify(feedForm)
    if (!editorBaselineRef.current) editorBaselineRef.current = serialized
    const visible = { automation: {
      kind: 'rss_feed', is_new: !editingFeedId,
      has_unsaved_changes: !editingFeedId || serialized !== editorBaselineRef.current,
      title: feedForm.name.slice(0, 500), content: feedForm.contentTemplate.slice(0, 4000),
      title_template: feedForm.titleTemplate.slice(0, 500),
      output_mode: feedForm.outputMode, target_account_ids: feedForm.targetAccountIds.slice(0, 50),
      ai_enhance_enabled: feedForm.aiEnhanceEnabled, prompt_hint: feedForm.promptHint.slice(0, 1000),
    } }
    setViewFocus(editingFeedId ? { type: 'rss_feed', id: editingFeedId } : { type: 'rss_feed' })
    setViewVisible(visible)
    return () => {
      if (getViewContext()?.visible === visible) {
        setViewFocus(undefined)
        setViewVisible(undefined)
      }
    }
  }, [isFeedDialogOpen, editingFeedId, feedForm])

  const openAddFeedDialog = () => {
    editorBaselineRef.current = ''
    setEditingFeedId(null)
    setFeedForm(emptyFeedForm())
    setIsFeedDialogOpen(true)
  }

  const openEditFeedDialog = (feed: RSSFeedConfig) => {
    editorBaselineRef.current = ''
    setEditingFeedId(feed.id)
    setFeedForm(feedToForm(feed))
    setIsFeedDialogOpen(true)
  }

  const toggleTargetAccount = (accountId: string) => {
    setFeedForm((prev) => ({
      ...prev,
      targetAccountIds: prev.targetAccountIds.includes(accountId)
        ? prev.targetAccountIds.filter((id) => id !== accountId)
        : [...prev.targetAccountIds, accountId],
    }))
  }

  const handleSaveFeed = async () => {
    if (!feedForm.name.trim() || !feedForm.feedUrl.trim()) return
    if (feedForm.targetAccountIds.length === 0) {
      setError(t('rss.selectAccounts'))
      return
    }
    if (!feedForm.contentTemplate.trim()) {
      setError(t('rss.templateRequired'))
      return
    }
    if (feedForm.aiEnhanceEnabled && team.isAiEnabled && !feedForm.promptHint.trim()) {
      setError(t('rss.promptRequired'))
      return
    }

    setError(null)
    setStatusMessage(null)
    const payload = {
      name: feedForm.name.trim(),
      feed_url: feedForm.feedUrl.trim(),
      is_active: feedForm.isActive,
      ai_enhance_enabled: feedForm.aiEnhanceEnabled,
      article_enrichment_enabled: feedForm.articleEnrichmentEnabled,
      content_template: feedForm.contentTemplate.trim(),
      title_template: feedForm.titleTemplate.trim(),
      title_hint: feedForm.titleHint.trim(),
      output_mode: feedForm.outputMode,
      max_posts_per_day: feedForm.maxPostsPerDay,
      prompt_hint: feedForm.promptHint.trim(),
      target_account_ids: feedForm.targetAccountIds,
      initial_sync_mode: feedForm.initialSyncMode,
    }

    try {
      if (editingFeedId) {
        await updateFeed.mutateAsync({ teamId: team.id, feedId: editingFeedId, data: payload })
        setStatusMessage(t('rss.feedUpdated'))
      } else {
        await createFeed.mutateAsync({ teamId: team.id, data: payload })
        setStatusMessage(t('rss.feedAdded'))
      }
      setIsFeedDialogOpen(false)
      setEditingFeedId(null)
      setFeedForm(emptyFeedForm())
    } catch (err) {
      setError(err instanceof Error ? err.message : t('rss.saveFailed'))
    }
  }

  const handleToggleFeed = async (feedId: string, currentActive: boolean) => {
    setError(null)
    setStatusMessage(null)
    try {
      await updateFeed.mutateAsync({
        teamId: team.id,
        feedId,
        data: { is_active: !currentActive },
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : t('rss.saveFailed'))
    }
  }

  const handleDeleteFeed = async (feedId: string) => {
    if (!window.confirm(t('rss.deleteConfirm'))) return
    setError(null)
    setStatusMessage(null)
    try {
      await deleteFeed.mutateAsync({ teamId: team.id, feedId })
      setStatusMessage(t('rss.feedDeleted'))
    } catch (err) {
      setError(err instanceof Error ? err.message : t('rss.saveFailed'))
    }
  }

  if (feedsLoading) {
    return <p className="hint">{t('rss.loading')}</p>
  }

  const feedDialogTitle = editingFeedId ? t('rss.editFeed') : t('rss.addFeed')
  const feedCanSave =
    feedForm.name.trim() &&
    feedForm.feedUrl.trim() &&
    feedForm.contentTemplate.trim() &&
    feedForm.targetAccountIds.length > 0

  return (
    <div className="glass-panel stack stack--lg">
      <div className="flex-row--between">
        <div>
          <h2 className="section-card__title">{t('rss.title')}</h2>
          <p className="hint">{t('rss.hint')}</p>
        </div>
        {canEdit ? (
          <Dialog.Root
            modal={false}
            open={isFeedDialogOpen}
            onOpenChange={(open) => {
              setIsFeedDialogOpen(open)
              if (!open) {
                setEditingFeedId(null)
                setFeedForm(emptyFeedForm())
              }
            }}
          >
            <Dialog.Trigger asChild>
              <button className="btn btn--secondary btn--sm" onClick={openAddFeedDialog}>
                <Plus size={16} />
                <span>{t('rss.addFeed')}</span>
              </button>
            </Dialog.Trigger>
            <Dialog.Portal>
              <Dialog.Overlay className="dialog-overlay" />
              <Dialog.Content
                className="dialog-content"
                style={{ maxWidth: '36rem', zIndex: 59 }}
                onInteractOutside={(event) => event.preventDefault()}
              >
                <div className="drawer-header">
                  <Dialog.Title className="drawer-title">{feedDialogTitle}</Dialog.Title>
                  <Dialog.Close asChild>
                    <button className="btn btn--ghost btn--icon-sm">
                      <X size={20} />
                    </button>
                  </Dialog.Close>
                </div>
                <div className="drawer-body stack">
                  <label className="field">
                    <span>{t('rss.feedName')}</span>
                    <input
                      value={feedForm.name}
                      onChange={(e) => setFeedForm((prev) => ({ ...prev, name: e.target.value }))}
                      placeholder="Tech Blog"
                    />
                  </label>
                  <label className="field">
                    <span>{t('rss.feedUrl')}</span>
                    <input
                      type="url"
                      value={feedForm.feedUrl}
                      onChange={(e) => setFeedForm((prev) => ({ ...prev, feedUrl: e.target.value }))}
                      placeholder="https://example.com/feed.xml"
                    />
                  </label>
                  <label className="field">
                    <span>{t('rss.contentTemplate')}</span>
                    <textarea
                      rows={5}
                      value={feedForm.contentTemplate}
                      onChange={(e) => setFeedForm((prev) => ({ ...prev, contentTemplate: e.target.value }))}
                      data-testid="rss-content-template"
                    />
                    <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
                      {t('rss.templateVars')}
                    </p>
                  </label>
                  <label className="field">
                    <span>{t('rss.titleTemplate')}</span>
                    <input
                      value={feedForm.titleTemplate}
                      onChange={(e) => setFeedForm((prev) => ({ ...prev, titleTemplate: e.target.value }))}
                      placeholder={defaultTitleTemplate}
                    />
                    <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
                      {t('rss.titleTemplateHint')}
                    </p>
                  </label>
                  <div className="field">
                    <span>{t('rss.outputMode')}</span>
                    <Segmented<AutomationOutputMode>
                      value={feedForm.outputMode}
                      options={[
                        { id: 'draft', label: outputModeLabel.draft },
                        { id: 'scheduled', label: outputModeLabel.scheduled },
                        { id: 'publish_now', label: outputModeLabel.publish_now },
                      ]}
                      onChange={(v) => setFeedForm((prev) => ({ ...prev, outputMode: v }))}
                      testIdPrefix="rss-output-mode"
                    />
                  </div>
                  <label className="field">
                    <span>{t('rss.maxPostsPerDay')}</span>
                    <input
                      type="number"
                      min={1}
                      max={50}
                      value={feedForm.maxPostsPerDay}
                      onChange={(e) =>
                        setFeedForm((prev) => ({
                          ...prev,
                          maxPostsPerDay: parseInt(e.target.value, 10) || 1,
                        }))
                      }
                    />
                  </label>
                  {team.isAiEnabled ? (
                    <>
                      <ToggleSwitch
                        checked={feedForm.aiEnhanceEnabled}
                        onChange={(next) =>
                          setFeedForm((prev) => ({ ...prev, aiEnhanceEnabled: next }))
                        }
                        title={t('rss.aiEnhanceEnabled')}
                        description="Die KI schreibt jeden Feed-Eintrag im Markenstil neu. Brand-Profil aus dem KI Studio bestimmt den Vibe."
                        testId="rss-ai-enhance"
                      />
                      {feedForm.aiEnhanceEnabled ? (
                        <>
                          <ToggleSwitch
                            checked={feedForm.articleEnrichmentEnabled}
                            onChange={(next) => setFeedForm((prev) => ({ ...prev, articleEnrichmentEnabled: next }))}
                            title={t('rss.articleEnrichmentEnabled')}
                            description={t('rss.articleEnrichmentHint')}
                            testId="rss-article-enrichment"
                          />
                          <label className="field">
                            <span>{t('rss.aiPrompt')}</span>
                            <textarea
                              rows={4}
                              value={feedForm.promptHint}
                              onChange={(e) => setFeedForm((prev) => ({ ...prev, promptHint: e.target.value }))}
                              placeholder={t('rss.aiPromptPlaceholder')}
                              data-testid="rss-ai-prompt"
                            />
                            <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
                              {t('rss.aiPromptHint')}
                            </p>
                          </label>
                          <label className="field">
                            <span>{t('rss.titleHint')}</span>
                            <input
                              value={feedForm.titleHint}
                              onChange={(e) => setFeedForm((prev) => ({ ...prev, titleHint: e.target.value }))}
                              placeholder={t('rss.titleHintPlaceholder')}
                            />
                            <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
                              {t('rss.titleHintHelp')}
                            </p>
                          </label>
                        </>
                      ) : null}
                    </>
                  ) : (
                    <p className="hint">{t('rss.aiRequiresTeam')}</p>
                  )}
                  {editingFeedId && feedForm.aiEnhanceEnabled && team.isAiEnabled ? (
                    <div className="stack stack--sm" data-testid="rss-ai-preview-controls">
                      <button type="button" className="btn btn--secondary btn--xs" onClick={() => void openFeedPreview(editingFeedId)} data-testid="rss-preview-load">
                        {t('automationPreview.open')}
                      </button>
                      {previewLoading ? <p className="hint">{t('automationPreview.loading')}</p> : null}
                      {previewError ? <p className="status-banner__error" role="alert">{previewError}</p> : null}
                      {previewFeedId === editingFeedId && !previewLoading && !previewError && previewItems.length === 0 ? (
                        <p className="hint">{t('automationPreview.noItems')}</p>
                      ) : null}
                      {previewFeedId === editingFeedId && previewItems.length > 0 ? (
                        <>
                          <label className="field">
                            <span>{t('automationPreview.selectSource')}</span>
                            <select value={previewItemKey} onChange={(event) => setPreviewItemKey(event.target.value)} data-testid="rss-preview-item">
                              {previewItems.map((item) => <option key={item.item_key} value={item.item_key}>{item.title || item.link}</option>)}
                            </select>
                          </label>
                          {previewItemKey ? (
                            <AutomationAIPreview
                              key={`rss-${editingFeedId}`}
                              teamId={team.id}
                              accounts={accounts}
                              testId="rss-ai-preview"
                              storageKey={`rss-${editingFeedId}`}
                              requestPreview={() => getApiClient().previewRSSFeedAI(team.id, editingFeedId, previewItemKey, {
                                content_template: feedForm.contentTemplate,
                                title_template: feedForm.titleTemplate,
                                prompt_hint: feedForm.promptHint,
                                title_hint: feedForm.titleHint,
                                target_account_ids: feedForm.targetAccountIds,
                                article_enrichment_enabled: feedForm.articleEnrichmentEnabled,
                              })}
                            />
                          ) : null}
                        </>
                      ) : null}
                    </div>
                  ) : null}
                  <div className="field">
                    <span>{t('rss.targetAccounts')}</span>
                    <DestinationPicker
                      accounts={accounts}
                      selectedIds={feedForm.targetAccountIds}
                      onToggle={toggleTargetAccount}
                      testIdPrefix="rss-feed-dest"
                    />
                  </div>
                  <div className="field">
                    <span>{t('rss.firstCheck')}</span>
                    <Segmented<RSSInitialSyncMode>
                      value={feedForm.initialSyncMode}
                      options={[
                        { id: 'baseline', label: t('rss.firstCheckBaseline') },
                        { id: 'publish_latest', label: t('rss.firstCheckLatest') },
                      ]}
                      onChange={(v) => setFeedForm((prev) => ({ ...prev, initialSyncMode: v }))}
                      testIdPrefix="rss-first-check"
                    />
                  </div>
                  <ToggleSwitch
                    checked={feedForm.isActive}
                    onChange={(next) => setFeedForm((prev) => ({ ...prev, isActive: next }))}
                    title={t('rss.active')}
                    description="Inaktive Feeds werden nicht abgerufen."
                  />
                  <div className="flex-row--end gap-2 mt-4">
                    <Dialog.Close asChild>
                      <button className="btn btn--ghost">{t('common.cancel')}</button>
                    </Dialog.Close>
                    <button
                      className="btn btn--primary"
                      onClick={handleSaveFeed}
                      disabled={!feedCanSave || createFeed.isPending || updateFeed.isPending}
                    >
                      {createFeed.isPending || updateFeed.isPending ? t('common.loading') : editingFeedId ? t('common.save') : t('rss.addFeed')}
                    </button>
                  </div>
                </div>
              </Dialog.Content>
            </Dialog.Portal>
          </Dialog.Root>
        ) : null}
      </div>

      {(error || statusMessage) && (
        <div className="status-banner-panel" style={{ padding: '1rem' }}>
          {statusMessage && <span className="status-banner__success">{statusMessage}</span>}
          {error && <span className="status-banner__error">{error}</span>}
        </div>
      )}

      <div className="stack stack--sm">
        {rssFeeds?.length === 0 ? (
          <p className="hint">{t('rss.empty')}</p>
        ) : (
          rssFeeds?.map((feed) => (
            <div key={feed.id} className="glass-panel glass-panel--compact recurring-template-card">
              <div className="recurring-template-card__header">
                <div className="flex-row--center gap-2">
                  <span className="badge">{feed.name}</span>
                  {(feed.exhaustedItemsCount ?? 0) > 0 && (
                    <span className="badge badge--warning" data-testid="rss-exhausted-items">
                      {t('rss.exhaustedItems', { count: feed.exhaustedItemsCount })}
                    </span>
                  )}
                </div>
                {canEdit ? (
                  <div className="flex-row--center gap-1">
                    <button
                      type="button"
                      className="btn btn--ghost btn--xs"
                      onClick={() => openEditFeedDialog(feed)}
                      disabled={updateFeed.isPending}
                      aria-label={t('rss.editFeed')}
                    >
                      <Pencil size={16} />
                    </button>
                    <DropdownMenu.Root>
                      <DropdownMenu.Trigger asChild>
                        <button type="button" className="btn btn--ghost btn--xs" aria-label={t('common.options', 'Options')}>
                          <MoreVertical size={16} />
                        </button>
                      </DropdownMenu.Trigger>
                      <DropdownMenu.Portal>
                        <DropdownMenu.Content className="radix-dropdown-content" align="end">
                          <DropdownMenu.Item
                            className="radix-dropdown-item"
                            onClick={() => handleDeleteFeed(feed.id)}
                            disabled={deleteFeed.isPending}
                          >
                            <Trash2 size={14} /> {t('common.delete')}
                          </DropdownMenu.Item>
                        </DropdownMenu.Content>
                      </DropdownMenu.Portal>
                    </DropdownMenu.Root>
                    <ToggleSwitch
                      checked={feed.isActive}
                      onChange={() => handleToggleFeed(feed.id, feed.isActive)}
                      title={feed.isActive ? t('rss.active') : t('rss.inactive', { defaultValue: 'Inaktiv' })}
                      disabled={updateFeed.isPending}
                      compact
                    />
                  </div>
                ) : null}
              </div>

                <div className="recurring-template-card__meta">
                <div className="recurring-template-card__meta-row" style={{ wordBreak: 'break-all' }}>
                  <Rss size={14} />
                  <span>{feed.feedUrl}</span>
                </div>
                <div className="recurring-template-card__meta-row">
                  <Target size={14} />
                  <span>
                    {outputModeLabel[feed.outputMode]} · {feed.targetAccountIds.length} {t('rss.accounts')}
                    {feed.aiEnhanceEnabled ? ` · ${t('rss.aiEnhanceEnabled')}` : ''}
                    {feed.initialSyncMode === 'publish_latest' ? ` · ${t('rss.firstCheckLatest')}` : ''}
                  </span>
                </div>
                <div className="recurring-template-card__meta-row">
                  <Clock size={14} />
                  <span>
                    {t('rss.lastFetched')}:{' '}
                    {feed.lastFetchedAt
                      ? formatDistanceToNow(new Date(feed.lastFetchedAt), { addSuffix: true })
                      : t('rss.neverFetched')}
                  </span>
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
