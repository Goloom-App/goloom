import { format, parseISO } from 'date-fns'
import { useCallback, useEffect, useMemo, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { createApiClient } from '../../api'
import type { BackendPostTemplate } from '../../api'
import { DestinationPicker } from '../../components/ai/DestinationPicker'
import { translateApiError } from '../../i18n/translateApiError'
import type { AccountRecord, AutomationOutputMode } from '../../types'
import { RecurrenceForm, recurrenceStateToJSON, parseRecurrenceJSON, type RecurrenceState } from './RecurrenceForm'
import { OccurrencePreview, computeOccurrences } from './OccurrencePreview'

type Api = ReturnType<typeof createApiClient>

const WEEKDAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

function expandContent(tpl: string, d: Date, counter: number, mainEvent?: Date): string {
  let s = tpl
  s = s.replace(/\{day([+-]\d+)\}/g, (_m, off: string) => zeroPad(clampDay(d.getDate() + parseInt(off, 10))))
  s = s.replace(/\{month([+-]\d+)\}/g, (_m, off: string) => zeroPad(clampMonth(d.getMonth() + 1 + parseInt(off, 10))))
  s = s.replace(/\{year\}/g, String(d.getFullYear()))
  s = s.replace(/\{month\}/g, zeroPad(d.getMonth() + 1))
  s = s.replace(/\{day\}/g, zeroPad(d.getDate()))
  s = s.replace(/\{weekday\}/g, String(d.getDay()))
  s = s.replace(/\{weekday_name\}/g, WEEKDAY_NAMES[d.getDay()])
  s = s.replace(/\{counter\}/g, String(counter))
  if (mainEvent) {
    s = s.replace(/\{main_day\}/g, zeroPad(mainEvent.getDate()))
    s = s.replace(/\{main_month\}/g, zeroPad(mainEvent.getMonth() + 1))
    s = s.replace(/\{main_weekday_name\}/g, WEEKDAY_NAMES[mainEvent.getDay()])
  } else {
    s = s.replace(/\{main_day\}/g, '')
    s = s.replace(/\{main_month\}/g, '')
    s = s.replace(/\{main_weekday_name\}/g, '')
  }
  return s
}

function zeroPad(n: number): string { return n < 10 ? '0' + n : String(n) }
function clampDay(d: number): number { return Math.max(1, Math.min(31, d)) }
function clampMonth(m: number): number { return Math.max(1, Math.min(12, m)) }

function ContentPreview({ content, firstOccurrence, announceTemplate, templates }: { content: string; firstOccurrence: Date | null; announceTemplate?: string; templates: BackendPostTemplate[] }) {
  const { t } = useTranslation()
  if (!content.trim() || !firstOccurrence) return null
  // For announcement templates, show expanded preview with {main_*} variables expanded from parent
  let mainEvent: Date | undefined
  if (announceTemplate) {
    const parent = templates.find((t) => t.id === announceTemplate)
    if (parent?.next_materialize_at) {
      mainEvent = parseISO(parent.next_materialize_at)
    }
  }
  return (
    <div className="recurrence-form__preview">
      <span className="recurrence-form__label">{t('recurring.expandedPreview')}</span>
      <div className="recurrence-form__expanded">{expandContent(content, firstOccurrence, 1, mainEvent)}</div>
    </div>
  )
}

export function RecurringPostsView({
  teamId,
  api,
  accounts,
  canEdit,
  onStatus,
  team,
}: {
  teamId: string
  api: Api
  accounts: AccountRecord[]
  canEdit: boolean
  onStatus: (msg: string | null) => void
  team?: { isAiEnabled?: boolean }
}) {
  const { t } = useTranslation()
  const [items, setItems] = useState<BackendPostTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [editorOpen, setEditorOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [recState, setRecState] = useState<RecurrenceState>(() => parseRecurrenceJSON(
    JSON.stringify({ kind: 'weekly', weekdays: [1], hour: 9, minute: 0, timezone: 'UTC' }),
  ))
  const [targetIds, setTargetIds] = useState<string[]>([])
  const [shiftInputs, setShiftInputs] = useState<Record<string, string>>({})
  const [isAnnouncement, setIsAnnouncement] = useState(false)
  const [announceTemplateId, setAnnounceTemplateId] = useState('')
  const [announceDaysBefore, setAnnounceDaysBefore] = useState(2)
  const [aiEnhanceEnabled, setAiEnhanceEnabled] = useState(false)
  const [outputMode, setOutputMode] = useState<AutomationOutputMode>('scheduled')
  const [promptHint, setPromptHint] = useState('')
  const [tonality, setTonality] = useState('')

  const outputModeLabel: Record<AutomationOutputMode, string> = {
    draft: t('rss.outputModeDraft', { defaultValue: 'Draft (review)' }),
    scheduled: t('rss.outputModeScheduled', { defaultValue: 'Scheduled' }),
    publish_now: t('rss.outputModePublishNow', { defaultValue: 'Publish now' }),
  }

  const firstOccurrence = useMemo(() => {
    const occ = computeOccurrences(recState, 1)
    return occ.length > 0 ? occ[0] : null
  }, [recState])

  const accountById = useMemo(() => Object.fromEntries(accounts.map((a) => [a.id, a])), [accounts])

  const refresh = useCallback(async function refresh() {
    setLoading(true)
    try {
      const res = await api.listPostTemplates(teamId)
      setItems(res.items ?? [])
    } finally {
      setLoading(false)
    }
  }, [api, teamId])

  useEffect(() => {
    void refresh()
  }, [refresh])

  function toggleTargetAccount(accountId: string) {
    setTargetIds((cur) =>
      cur.includes(accountId) ? cur.filter((id) => id !== accountId) : [...cur, accountId],
    )
  }

  function closeEditor() {
    setEditorOpen(false)
  }

  function openEditor() {
    setTitle('')
    setContent('')
    setRecState(parseRecurrenceJSON(
      JSON.stringify({ kind: 'weekly', weekdays: [1], hour: 9, minute: 0, timezone: 'UTC' }),
    ))
    setTargetIds([])
    setIsAnnouncement(false)
    setAnnounceTemplateId('')
    setAnnounceDaysBefore(2)
    setAiEnhanceEnabled(false)
    setOutputMode('scheduled')
    setPromptHint('')
    setTonality('')
    setEditorOpen(true)
  }

  async function handleCreate() {
    if (!title.trim() || !content.trim() || targetIds.length === 0) {
      onStatus(t('recurring.requiredFields'))
      return
    }
    onStatus(null)
    try {
      const payload: Parameters<typeof api.createPostTemplate>[1] = {
        title: title.trim(),
        content: content.trim(),
        recurrence_json: recurrenceStateToJSON(recState),
        target_account_ids: targetIds,
        enabled: true,
      }
      if (isAnnouncement && announceTemplateId) {
        payload.announces_template_id = announceTemplateId
        payload.announcement_days_before = announceDaysBefore
      } else {
        payload.output_mode = outputMode
        if (team?.isAiEnabled) {
          payload.ai_enhance_enabled = aiEnhanceEnabled
          payload.prompt_hint = promptHint.trim()
          payload.tonality = tonality.trim()
        }
      }
      await api.createPostTemplate(teamId, payload)
      closeEditor()
      setTitle('')
      setContent('')
      await refresh()
      onStatus(t('status.templateCreated'))
    } catch (e) {
      const raw = e instanceof Error ? e.message : t('status.templateCreateFailed')
      onStatus(translateApiError(raw, t))
    }
  }

  async function toggleEnabled(id: string, currentEnabled: boolean) {
    try {
      await api.updatePostTemplate(teamId, id, { enabled: !currentEnabled })
      await refresh()
    } catch (e) {
      const raw = e instanceof Error ? e.message : t('status.templateUpdateFailed')
      onStatus(translateApiError(raw, t))
    }
  }

  async function removeTemplate(id: string) {
    if (!window.confirm(t('recurring.confirmDelete'))) {
      return
    }
    try {
      await api.deletePostTemplate(teamId, id)
      await refresh()
    } catch (e) {
      const raw = e instanceof Error ? e.message : t('status.templateDeleteFailed')
      onStatus(translateApiError(raw, t))
    }
  }

  async function skipNext(id: string, nextIso?: string) {
    if (!nextIso) {
      onStatus(t('status.noOccurrenceToSkip'))
      return
    }
    try {
      await api.skipPostTemplateOccurrence(teamId, id, nextIso)
      await refresh()
      onStatus(t('status.occurrenceSkipped'))
    } catch (e) {
      const raw = e instanceof Error ? e.message : t('status.skipFailed')
      onStatus(translateApiError(raw, t))
    }
  }

  async function shiftNext(id: string, nextIso?: string) {
    if (!nextIso) {
      onStatus(t('status.noOccurrenceToSkip'))
      return
    }
    const shiftVal = shiftInputs[id]
    if (!shiftVal) {
      setShiftInputs((cur) => ({ ...cur, [id]: '' }))
      return
    }
    try {
      await api.skipPostTemplateOccurrence(teamId, id, nextIso, new Date(shiftVal).toISOString())
      setShiftInputs((cur) => { const c = { ...cur }; delete c[id]; return c })
      await refresh()
      onStatus(t('status.occurrenceShifted'))
    } catch (e) {
      const raw = e instanceof Error ? e.message : t('status.skipFailed')
      onStatus(translateApiError(raw, t))
    }
  }

  return (
    <div className="recurring-posts-view two-column-detail">
      <div className="glass-panel">
        <div className="flex-row--wrap" style={{ justifyContent: 'space-between' }}>
          <div>
            <h2 className="section-card__title">{t('recurring.title')}</h2>
            <p className="hint">{t('recurring.hint')}</p>
          </div>
          {canEdit ? (
            <button type="button" className="button button--primary" onClick={openEditor}>
              {t('recurring.newTemplate')}
            </button>
          ) : null}
        </div>
        {loading ? <p className="hint">{t('common.loading')}</p> : null}
        {!loading && items.length === 0 ? <p className="hint">{t('recurring.noTemplates')}</p> : null}
        <ul className="recurring-template-list">
          {items.map((item) => (
            <li key={item.id} className="glass-panel recurring-template-card">
              <div className="recurring-template-card__header">
                <strong>{item.title || t('recurring.untitled')}</strong>
                <span className="hint">
                  {item.announces_template_id ? <span className="badge badge--info">{t('recurring.announcementBadge')}</span> : null}
                  {item.enabled ? t('analytics.enabled') : t('analytics.paused')}
                </span>
              </div>
              <p className="hint monospace-small">{item.recurrence_json}</p>
              <p className="hint">
                {t('recurring.next')}{' '}
                {item.next_materialize_at ? format(parseISO(item.next_materialize_at), 'PPpp') : t('common.emDash')} · {t('common.counter')}:{' '}
                {item.counter_next}
              </p>
              <p className="hint">
                {t('recurring.targets')}{' '}
                {item.target_account_ids.map((id) => accountById[id]?.username ?? id.slice(0, 8)).join(', ')}
              </p>
              {canEdit ? (
                <div className="inline-cluster mt-1" style={{ flexWrap: 'wrap' }}>
                  <button type="button" className="button button--secondary" onClick={() => void toggleEnabled(item.id, item.enabled)}>
                    {item.enabled ? t('recurring.pause') : t('recurring.resume')}
                  </button>
                  <button type="button" className="button button--secondary" onClick={() => void skipNext(item.id, item.next_materialize_at)}>
                    {t('recurring.skipNext')}
                  </button>
                  <button type="button" className="button button--secondary" onClick={() => void shiftNext(item.id, item.next_materialize_at)}>
                    {t('recurring.shiftNext')}
                  </button>
                  {shiftInputs[item.id] !== undefined ? (
                    <span className="inline-cluster">
                      <input type="datetime-local" value={shiftInputs[item.id]} onChange={(e) => setShiftInputs((cur) => ({ ...cur, [item.id]: e.target.value }))} />
                      <button type="button" className="button button--primary" onClick={() => void shiftNext(item.id, item.next_materialize_at)}>
                        {t('common.apply')}
                      </button>
                    </span>
                  ) : null}
                  <button type="button" className="button button--secondary" onClick={() => void removeTemplate(item.id)}>
                    {t('common.delete')}
                  </button>
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      </div>

      {editorOpen ? (
        <Dialog.Root open={editorOpen} onOpenChange={(open) => !open && closeEditor()}>
          <Dialog.Portal>
            <Dialog.Overlay className="dialog-overlay" />
            <Dialog.Content className="dialog-content" style={{ maxWidth: '36rem' }} data-testid="recurring-template-dialog">
              <div className="drawer-header">
                <Dialog.Title className="drawer-title">{t('recurring.editorTitle')}</Dialog.Title>
                <Dialog.Close asChild>
                  <button type="button" className="btn btn--ghost btn--icon-sm" aria-label={t('common.cancel')}>
                    <X size={20} />
                  </button>
                </Dialog.Close>
              </div>
              <div className="drawer-body stack">
                <label className="field">
                  <span>{t('common.title')}</span>
                  <input value={title} onChange={(e) => setTitle(e.target.value)} />
                </label>

                <label className="field">
                  <span>{t('common.content')}</span>
                  <textarea rows={5} value={content} onChange={(e) => setContent(e.target.value)} placeholder={t('recurring.contentPlaceholder')} />
                </label>

                <RecurrenceForm state={recState} onChange={setRecState} />
                <OccurrencePreview state={recState} />

                <div className="field">
                  <span>{t('common.targets')}</span>
                  <DestinationPicker
                    accounts={accounts}
                    selectedIds={targetIds}
                    onToggle={toggleTargetAccount}
                    testIdPrefix="recurring-template-dest"
                  />
                </div>

                <div className="recurrence-form__section">
                  <label className="field field--checkbox">
                    <input type="checkbox" checked={isAnnouncement} onChange={(e) => setIsAnnouncement(e.target.checked)} />
                    <span>{t('recurring.announcement')}</span>
                  </label>
                  {isAnnouncement ? (
                    <div className="recurrence-form__announcement-config">
                      <label className="field">
                        <span>{t('recurring.announcementFor')}</span>
                        <select value={announceTemplateId} onChange={(e) => setAnnounceTemplateId(e.target.value)}>
                          <option value="">—</option>
                          {items.filter((i) => i.enabled && i.next_materialize_at).map((i) => (
                            <option key={i.id} value={i.id}>{i.title || t('recurring.untitled')}</option>
                          ))}
                        </select>
                      </label>
                      <label className="field">
                        <span>{t('recurring.announcementDaysBefore')}</span>
                        <input type="number" min={1} max={30} value={announceDaysBefore} onChange={(e) => setAnnounceDaysBefore(parseInt(e.target.value, 10) || 2)} />
                      </label>
                      <p className="hint">{t('recurring.announcementHint')}</p>
                    </div>
                  ) : null}
                </div>

                <ContentPreview content={content} firstOccurrence={firstOccurrence} announceTemplate={isAnnouncement ? announceTemplateId : undefined} templates={items} />

                {!isAnnouncement ? (
                  <>
                    <label className="field">
                      <span>{t('rss.outputMode')}</span>
                      <select value={outputMode} onChange={(e) => setOutputMode(e.target.value as AutomationOutputMode)}>
                        <option value="draft">{outputModeLabel.draft}</option>
                        <option value="scheduled">{outputModeLabel.scheduled}</option>
                        <option value="publish_now">{outputModeLabel.publish_now}</option>
                      </select>
                    </label>
                    {team?.isAiEnabled ? (
                      <>
                        <label className="field field--checkbox">
                          <input type="checkbox" checked={aiEnhanceEnabled} onChange={(e) => setAiEnhanceEnabled(e.target.checked)} />
                          <span>{t('rss.aiEnhanceEnabled')}</span>
                        </label>
                        {aiEnhanceEnabled ? (
                          <>
                            <label className="field">
                              <span>{t('rss.aiPrompt')}</span>
                              <textarea rows={3} value={promptHint} onChange={(e) => setPromptHint(e.target.value)} placeholder={t('rss.aiPromptPlaceholder')} />
                            </label>
                            <label className="field">
                              <span>{t('rss.tonalityOverride')}</span>
                              <input value={tonality} onChange={(e) => setTonality(e.target.value)} placeholder={t('rss.tonalityPlaceholder')} />
                            </label>
                          </>
                        ) : null}
                      </>
                    ) : null}
                  </>
                ) : null}

                <div className="flex-row--end gap-2 mt-4">
                  <Dialog.Close asChild>
                    <button type="button" className="btn btn--ghost">{t('common.cancel')}</button>
                  </Dialog.Close>
                  <button type="button" className="btn btn--primary" onClick={() => void handleCreate()}>
                    {t('common.create')}
                  </button>
                </div>
              </div>
            </Dialog.Content>
          </Dialog.Portal>
        </Dialog.Root>
      ) : null}
    </div>
  )
}
