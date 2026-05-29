import { useState, useEffect } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X, Plus, Trash2, Rss, Settings, Clock } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'

import {
  useRSSFeeds,
  useCreateRSSFeed,
  useUpdateRSSFeed,
  useDeleteRSSFeed,
  useProactiveSettings,
  useUpsertProactiveSettings,
} from '../../hooks/useAI'
import type { TeamRecord } from '../../types'

interface ProactiveTriggersViewProps {
  team: TeamRecord
}

export function ProactiveTriggersView({ team }: ProactiveTriggersViewProps) {
  const { data: rssFeeds, isLoading: feedsLoading } = useRSSFeeds(team.id)
  const { data: proactiveSettings, isLoading: settingsLoading } = useProactiveSettings(team.id)
  
  const createFeed = useCreateRSSFeed()
  const updateFeed = useUpdateRSSFeed()
  const deleteFeed = useDeleteRSSFeed()
  const upsertSettings = useUpsertProactiveSettings()

  const [isFeedDialogOpen, setIsFeedDialogOpen] = useState(false)
  const [feedName, setFeedName] = useState('')
  const [feedUrl, setFeedUrl] = useState('')
  const [feedIsActive, setFeedIsActive] = useState(true)

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

  const handleAddFeed = async () => {
    if (!feedName.trim() || !feedUrl.trim()) return
    setError(null)
    setStatusMessage(null)
    try {
      await createFeed.mutateAsync({
        teamId: team.id,
        data: {
          name: feedName.trim(),
          feed_url: feedUrl.trim(),
          is_active: feedIsActive,
        },
      })
      setIsFeedDialogOpen(false)
      setFeedName('')
      setFeedUrl('')
      setFeedIsActive(true)
      setStatusMessage('RSS feed added')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add RSS feed')
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
          <Dialog.Root open={isFeedDialogOpen} onOpenChange={setIsFeedDialogOpen}>
            <Dialog.Trigger asChild>
              <button className="btn btn--secondary btn--sm">
                <Plus size={16} />
                <span>Add Feed</span>
              </button>
            </Dialog.Trigger>
            <Dialog.Portal>
              <Dialog.Overlay className="dialog-overlay" />
              <Dialog.Content className="dialog-content">
                <div className="drawer-header">
                  <Dialog.Title className="drawer-title">Add RSS Feed</Dialog.Title>
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
                      value={feedName}
                      onChange={(e) => setFeedName(e.target.value)}
                      placeholder="e.g., TechCrunch"
                    />
                  </label>
                  <label className="field">
                    <span>Feed URL</span>
                    <input
                      type="url"
                      value={feedUrl}
                      onChange={(e) => setFeedUrl(e.target.value)}
                      placeholder="https://example.com/feed.xml"
                    />
                  </label>
                  <label className="field flex-row--center gap-2" style={{ flexDirection: 'row', alignItems: 'center' }}>
                    <input
                      type="checkbox"
                      checked={feedIsActive}
                      onChange={(e) => setFeedIsActive(e.target.checked)}
                    />
                    <span>Active</span>
                  </label>
                  <div className="flex-row--end gap-2 mt-4">
                    <Dialog.Close asChild>
                      <button className="btn btn--ghost">Cancel</button>
                    </Dialog.Close>
                    <button
                      className="btn btn--primary"
                      onClick={handleAddFeed}
                      disabled={!feedName.trim() || !feedUrl.trim() || createFeed.isPending}
                    >
                      {createFeed.isPending ? 'Adding...' : 'Add Feed'}
                    </button>
                  </div>
                </div>
              </Dialog.Content>
            </Dialog.Portal>
          </Dialog.Root>
        </div>

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
                  <button
                    type="button"
                    className="btn btn--ghost btn--xs"
                    onClick={() => handleDeleteFeed(feed.id)}
                    disabled={deleteFeed.isPending}
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
                <p className="hint" style={{ fontSize: '0.85rem', wordBreak: 'break-all' }}>
                  {feed.feedUrl}
                </p>
                <div className="flex-row--center gap-1 mt-2 hint" style={{ fontSize: '0.75rem' }}>
                  <Clock size={12} />
                  <span>
                    Last fetched: {feed.lastFetchedAt ? formatDistanceToNow(new Date(feed.lastFetchedAt), { addSuffix: true }) : 'Never'}
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
