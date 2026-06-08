import { useState, useEffect } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X, Plus, Trash2, Rss, Settings, Clock, Pencil } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'

import { DestinationPicker } from '../../components/ai/DestinationPicker'
import {
  useRSSFeeds,
  useCreateRSSFeed,
  useUpdateRSSFeed,
  useDeleteRSSFeed,
  useProactiveSettings,
  useUpsertProactiveSettings,
} from '../../hooks/useAI'
import type { AccountRecord, RSSFeedConfig, TeamRecord } from '../../types'

interface ProactiveTriggersViewProps {
  team: TeamRecord
  accounts: AccountRecord[]
}

type FeedFormState = {
  name: string
  feedUrl: string
  isActive: boolean
  promptHint: string
  targetAccountIds: string[]
  tonality: string
}

const emptyFeedForm = (): FeedFormState => ({
  name: '',
  feedUrl: '',
  isActive: true,
  promptHint: '',
  targetAccountIds: [],
  tonality: '',
})

function feedToForm(feed: RSSFeedConfig): FeedFormState {
  return {
    name: feed.name,
    feedUrl: feed.feedUrl,
    isActive: feed.isActive,
    promptHint: feed.promptHint,
    targetAccountIds: feed.targetAccountIds,
    tonality: feed.tonality,
  }
}

export function ProactiveTriggersView({ team, accounts }: ProactiveTriggersViewProps) {
  const { data: rssFeeds, isLoading: feedsLoading } = useRSSFeeds(team.id)
  const { data: proactiveSettings, isLoading: settingsLoading } = useProactiveSettings(team.id)

  const createFeed = useCreateRSSFeed()
  const updateFeed = useUpdateRSSFeed()
  const deleteFeed = useDeleteRSSFeed()
  const upsertSettings = useUpsertProactiveSettings()

  const [isFeedDialogOpen, setIsFeedDialogOpen] = useState(false)
  const [editingFeedId, setEditingFeedId] = useState<string | null>(null)
  const [feedForm, setFeedForm] = useState<FeedFormState>(emptyFeedForm)

  const [contentGapThresholdDays, setContentGapThresholdDays] = useState(3)
  const [autoFillEnabled, setAutoFillEnabled] = useState(false)
  const [maxTriggersPerDay, setMaxTriggersPerDay] = useState(5)
  const [cronSchedule, setCronSchedule] = useState('0 9 * * *')
  const [cronPreset, setCronPreset] = useState('0 9 * * *')

  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (proactiveSettings) {
      setContentGapThresholdDays(proactiveSettings.contentGapThresholdDays ?? 3)
      setAutoFillEnabled(proactiveSettings.autoFillEnabled ?? false)
      setMaxTriggersPerDay(proactiveSettings.maxTriggersPerDay ?? 5)
      setCronSchedule(proactiveSettings.cronSchedule || '0 9 * * *')

      const schedule = proactiveSettings.cronSchedule || '0 9 * * *'
      if (['0 * * * *', '0 9 * * *', '0 18 * * *'].includes(schedule)) {
        setCronPreset(schedule)
      } else {
        setCronPreset('custom')
      }
    }
  }, [proactiveSettings])

  if (!team.isAiEnabled) {
    return (
      <div className="empty-state">
        <p className="hint">AI features are not enabled for this team.</p>
      </div>
    )
  }

  const openAddFeedDialog = () => {
    setEditingFeedId(null)
    setFeedForm(emptyFeedForm())
    setIsFeedDialogOpen(true)
  }

  const openEditFeedDialog = (feed: RSSFeedConfig) => {
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

  const handleSaveSettings = async () => {
    setError(null)
    setStatusMessage(null)
    try {
      await upsertSettings.mutateAsync({
        teamId: team.id,
        data: {
          content_gap_threshold_days: contentGapThresholdDays,
          auto_fill_enabled: autoFillEnabled,
          max_triggers_per_day: maxTriggersPerDay,
          cron_schedule: cronSchedule,
        },
      })
      setStatusMessage('Proactive settings saved successfully')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save settings')
    }
  }

  const handleSaveFeed = async () => {
    if (!feedForm.name.trim() || !feedForm.feedUrl.trim()) return
    if (feedForm.targetAccountIds.length === 0) {
      setError('Select at least one target account for this RSS feed')
      return
    }
    if (!feedForm.promptHint.trim()) {
      setError('Add a prompt describing how the AI should turn articles into posts')
      return
    }

    setError(null)
    setStatusMessage(null)
    const payload = {
      name: feedForm.name.trim(),
      feed_url: feedForm.feedUrl.trim(),
      is_active: feedForm.isActive,
      prompt_hint: feedForm.promptHint.trim(),
      target_account_ids: feedForm.targetAccountIds,
      tonality: feedForm.tonality.trim(),
    }

    try {
      if (editingFeedId) {
        await updateFeed.mutateAsync({
          teamId: team.id,
          feedId: editingFeedId,
          data: payload,
        })
        setStatusMessage('RSS feed updated')
      } else {
        await createFeed.mutateAsync({
          teamId: team.id,
          data: payload,
        })
        setStatusMessage('RSS feed added')
      }
      setIsFeedDialogOpen(false)
      setEditingFeedId(null)
      setFeedForm(emptyFeedForm())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save RSS feed')
    }
  }

  const handleToggleFeed = async (feedId: string, currentActive: boolean) => {
    setError(null)
    setStatusMessage(null)
    try {
      await updateFeed.mutateAsync({
        teamId: team.id,
        feedId,
        data: {
          is_active: !currentActive,
        },
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update RSS feed')
    }
  }

  const handleDeleteFeed = async (feedId: string) => {
    if (!window.confirm('Are you sure you want to delete this RSS feed?')) return
    setError(null)
    setStatusMessage(null)
    try {
      await deleteFeed.mutateAsync({ teamId: team.id, feedId })
      setStatusMessage('RSS feed deleted')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete RSS feed')
    }
  }

  const handleCronPresetChange = (value: string) => {
    setCronPreset(value)
    if (value !== 'custom') {
      setCronSchedule(value)
    }
  }

  if (feedsLoading || settingsLoading) {
    return <p className="hint">Loading proactive triggers...</p>
  }

  const feedDialogTitle = editingFeedId ? 'Edit RSS Feed' : 'Add RSS Feed'
  const feedCanSave =
    feedForm.name.trim() &&
    feedForm.feedUrl.trim() &&
    feedForm.promptHint.trim() &&
    feedForm.targetAccountIds.length > 0

  return (
    <div className="two-column-detail">
      <div className="glass-panel stack">
        <div className="flex-row--between">
          <h2 className="section-card__title flex-row--center gap-2">
            <Settings size={20} />
            Proactive Settings
          </h2>
        </div>
        <p className="hint">Configure how the AI proactively generates content for your team.</p>

        {(error || statusMessage) && (
          <div className="status-banner-panel" style={{ padding: '1rem', marginBottom: '1rem' }}>
            {statusMessage && <span className="status-banner__success">{statusMessage}</span>}
            {error && <span className="status-banner__error">{error}</span>}
          </div>
        )}

        <label className="field flex-row--center gap-2" style={{ flexDirection: 'row', alignItems: 'center' }}>
          <input
            type="checkbox"
            checked={autoFillEnabled}
            onChange={(e) => setAutoFillEnabled(e.target.checked)}
          />
          <span>Enable Auto-Fill (Generate posts when calendar is empty)</span>
        </label>

        <label className="field">
          <span>Content Gap Threshold (Days)</span>
          <div className="flex-row--center gap-2">
            <input
              type="range"
              min="1"
              max="14"
              value={contentGapThresholdDays}
              onChange={(e) => setContentGapThresholdDays(parseInt(e.target.value, 10))}
              style={{ flex: 1 }}
            />
            <span style={{ minWidth: '3rem', textAlign: 'right' }}>{contentGapThresholdDays} days</span>
          </div>
          <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
            Trigger generation if no posts are scheduled within this many days.
          </p>
        </label>

        <label className="field">
          <span>Max Triggers Per Day</span>
          <input
            type="number"
            min="1"
            max="20"
            value={maxTriggersPerDay}
            onChange={(e) => setMaxTriggersPerDay(parseInt(e.target.value, 10) || 1)}
          />
        </label>

        <div className="field">
          <span>Cron Schedule</span>
          <select
            value={cronPreset}
            onChange={(e) => handleCronPresetChange(e.target.value)}
            style={{ marginBottom: '0.5rem' }}
          >
            <option value="0 * * * *">Every hour</option>
            <option value="0 9 * * *">Daily at 9am</option>
            <option value="0 18 * * *">Daily at 6pm</option>
            <option value="custom">Custom</option>
          </select>

          {cronPreset === 'custom' && (
            <input
              value={cronSchedule}
              onChange={(e) => setCronSchedule(e.target.value)}
              placeholder="e.g., 0 9 * * *"
            />
          )}
          <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
            When should the system check for content gaps and new RSS items?
          </p>
        </div>

        <div>
          <button
            type="button"
            className="btn btn--primary"
            onClick={handleSaveSettings}
            disabled={upsertSettings.isPending}
          >
            {upsertSettings.isPending ? 'Saving...' : 'Save Settings'}
          </button>
        </div>
      </div>

      <div className="glass-panel stack">
        <div className="flex-row--between">
          <h2 className="section-card__title flex-row--center gap-2">
            <Rss size={20} />
            RSS Feeds
          </h2>
          <Dialog.Root
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
                <span>Add Feed</span>
              </button>
            </Dialog.Trigger>
            <Dialog.Portal>
              <Dialog.Overlay className="dialog-overlay" />
              <Dialog.Content className="dialog-content" style={{ maxWidth: '36rem' }}>
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
                    <span>Feed Name</span>
                    <input
                      value={feedForm.name}
                      onChange={(e) => setFeedForm((prev) => ({ ...prev, name: e.target.value }))}
                      placeholder="e.g., TechCrunch"
                    />
                  </label>
                  <label className="field">
                    <span>Feed URL</span>
                    <input
                      type="url"
                      value={feedForm.feedUrl}
                      onChange={(e) => setFeedForm((prev) => ({ ...prev, feedUrl: e.target.value }))}
                      placeholder="https://example.com/feed.xml"
                    />
                  </label>
                  <label className="field">
                    <span>AI Prompt</span>
                    <textarea
                      rows={4}
                      value={feedForm.promptHint}
                      onChange={(e) => setFeedForm((prev) => ({ ...prev, promptHint: e.target.value }))}
                      placeholder="e.g., Write a concise post highlighting the key takeaway and why our audience should care."
                    />
                    <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
                      Sent to the AI together with each new article title and content.
                    </p>
                  </label>
                  <div className="field">
                    <span>Target Accounts</span>
                    <DestinationPicker
                      accounts={accounts}
                      selectedIds={feedForm.targetAccountIds}
                      onToggle={toggleTargetAccount}
                      testIdPrefix="rss-feed-dest"
                    />
                  </div>
                  <label className="field">
                    <span>Tonality (optional)</span>
                    <input
                      value={feedForm.tonality}
                      onChange={(e) => setFeedForm((prev) => ({ ...prev, tonality: e.target.value }))}
                      placeholder="Overrides team profile tonality for this feed"
                    />
                  </label>
                  <label className="field flex-row--center gap-2" style={{ flexDirection: 'row', alignItems: 'center' }}>
                    <input
                      type="checkbox"
                      checked={feedForm.isActive}
                      onChange={(e) => setFeedForm((prev) => ({ ...prev, isActive: e.target.checked }))}
                    />
                    <span>Active</span>
                  </label>
                  <div className="flex-row--end gap-2 mt-4">
                    <Dialog.Close asChild>
                      <button className="btn btn--ghost">Cancel</button>
                    </Dialog.Close>
                    <button
                      className="btn btn--primary"
                      onClick={handleSaveFeed}
                      disabled={!feedCanSave || createFeed.isPending || updateFeed.isPending}
                    >
                      {createFeed.isPending || updateFeed.isPending ? 'Saving...' : editingFeedId ? 'Save Changes' : 'Add Feed'}
                    </button>
                  </div>
                </div>
              </Dialog.Content>
            </Dialog.Portal>
          </Dialog.Root>
        </div>

        <p className="hint" style={{ fontSize: '0.85rem' }}>
          RSS feeds run independently of Auto-Fill. New articles create AI drafts for the selected accounts.
        </p>

        <div className="stack stack--sm mt-4">
          {rssFeeds?.length === 0 ? (
            <p className="hint">No RSS feeds configured.</p>
          ) : (
            rssFeeds?.map((feed) => (
              <div key={feed.id} className="glass-panel glass-panel--compact">
                <div className="flex-row--between mb-2">
                  <div className="flex-row--center gap-2">
                    <span className="badge">{feed.name}</span>
                    <label className="flex-row--center gap-1" style={{ fontSize: '0.8rem', cursor: 'pointer' }}>
                      <input
                        type="checkbox"
                        checked={feed.isActive}
                        onChange={() => handleToggleFeed(feed.id, feed.isActive)}
                        disabled={updateFeed.isPending}
                      />
                      <span>Active</span>
                    </label>
                  </div>
                  <div className="flex-row--center gap-1">
                    <button
                      type="button"
                      className="btn btn--ghost btn--xs"
                      onClick={() => openEditFeedDialog(feed)}
                      disabled={updateFeed.isPending}
                    >
                      <Pencil size={16} />
                    </button>
                    <button
                      type="button"
                      className="btn btn--ghost btn--xs"
                      onClick={() => handleDeleteFeed(feed.id)}
                      disabled={deleteFeed.isPending}
                    >
                      <Trash2 size={16} />
                    </button>
                  </div>
                </div>
                <p className="hint" style={{ fontSize: '0.85rem', wordBreak: 'break-all' }}>
                  {feed.feedUrl}
                </p>
                {feed.promptHint && (
                  <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.5rem' }}>
                    Prompt: {feed.promptHint}
                  </p>
                )}
                <p className="hint" style={{ fontSize: '0.75rem', marginTop: '0.25rem' }}>
                  {feed.targetAccountIds.length} target account{feed.targetAccountIds.length === 1 ? '' : 's'}
                  {feed.tonality ? ` · Tonality: ${feed.tonality}` : ''}
                </p>
                <div className="flex-row--center gap-1 mt-2 hint" style={{ fontSize: '0.75rem' }}>
                  <Clock size={12} />
                  <span>
                    Last fetched:{' '}
                    {feed.lastFetchedAt
                      ? formatDistanceToNow(new Date(feed.lastFetchedAt), { addSuffix: true })
                      : 'Never (will baseline on first check)'}
                  </span>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
