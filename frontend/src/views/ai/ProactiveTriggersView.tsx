import { useState, useEffect } from 'react'
import { Settings } from 'lucide-react'

import { useProactiveSettings, useUpsertProactiveSettings } from '../../hooks/useAI'
import type { TeamRecord } from '../../types'

interface ProactiveTriggersViewProps {
  team: TeamRecord
}

export function ProactiveTriggersView({ team }: ProactiveTriggersViewProps) {
  const { data: proactiveSettings, isLoading: settingsLoading } = useProactiveSettings(team.id)
  const upsertSettings = useUpsertProactiveSettings()

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

  const handleCronPresetChange = (value: string) => {
    setCronPreset(value)
    if (value !== 'custom') {
      setCronSchedule(value)
    }
  }

  if (settingsLoading) {
    return <p className="hint">Loading proactive triggers...</p>
  }

  return (
    <div className="glass-panel stack">
      <div className="flex-row--between">
        <h2 className="section-card__title flex-row--center gap-2">
          <Settings size={20} />
          Proactive Settings
        </h2>
      </div>
      <p className="hint">Configure content-gap auto-fill. RSS feeds are managed under Workspace → RSS Feeds.</p>

      {(error || statusMessage) && (
        <div className="status-banner-panel" style={{ padding: '1rem', marginBottom: '1rem' }}>
          {statusMessage && <span className="status-banner__success">{statusMessage}</span>}
          {error && <span className="status-banner__error">{error}</span>}
        </div>
      )}

      <label className="field flex-row--center gap-2" style={{ flexDirection: 'row', alignItems: 'center' }}>
        <input type="checkbox" checked={autoFillEnabled} onChange={(e) => setAutoFillEnabled(e.target.checked)} />
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
        <select value={cronPreset} onChange={(e) => handleCronPresetChange(e.target.value)} style={{ marginBottom: '0.5rem' }}>
          <option value="0 * * * *">Every hour</option>
          <option value="0 9 * * *">Daily at 9am</option>
          <option value="0 18 * * *">Daily at 6pm</option>
          <option value="custom">Custom</option>
        </select>

        {cronPreset === 'custom' && (
          <input value={cronSchedule} onChange={(e) => setCronSchedule(e.target.value)} placeholder="e.g., 0 9 * * *" />
        )}
        <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
          How often the AI service checks for content gaps. Save settings after changing.
        </p>
      </div>

      <div>
        <button type="button" className="btn btn--primary" onClick={handleSaveSettings} disabled={upsertSettings.isPending}>
          {upsertSettings.isPending ? 'Saving...' : 'Save Settings'}
        </button>
      </div>
    </div>
  )
}
