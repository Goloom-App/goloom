import { format, parseISO } from 'date-fns'
import { useCallback, useEffect, useMemo, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { CalendarClock, Clock, MoreVertical, Pencil, Plus, Target, Trash2, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { createApiClient } from '../../api'
import type { BackendPostTemplate } from '../../api'
import { DestinationPicker } from '../../components/ai/DestinationPicker'
import { Segmented, ToggleSwitch } from '../../components/ui'
import { translateApiError } from '../../i18n/translateApiError'
import type { AccountRecord, AutomationOutputMode } from '../../types'
import { RecurrenceForm, recurrenceStateToJSON, parseRecurrenceJSON, type RecurrenceState } from './RecurrenceForm'
import { OccurrencePreview, computeOccurrences } from './OccurrencePreview'
import { formatRecurrenceSummary } from './recurrenceUtils'

type Api = ReturnType<typeof createApiClient>

const WEEKDAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

function expandContent(tpl: string, d: Date, counter: number, mainEvent?: Date, mainCounter?: number): string {
  let s = tpl
  s = s.replace(/\{day([+-]\d+)\}/g, (_m, off: string) => zeroPad(clampDay(d.getDate() + parseInt(off, 10))))
  s = s.replace(/\{month([+-]\d+)\}/g, (_m, off: string) => zeroPad(clampMonth(d.getMonth() + 1 + parseInt(off, 10))))
  s = s.replace(/\{year\}/g, String(d.getFullYear()))
  s = s.replace(/\{month\}/g, zeroPad(d.getMonth() + 1))
  s = s.replace(/\{day\}/g, zeroPad(d.getDate()))
  s = s.replace(/\{weekday\}/g, String(d.getDay()))
  s = s.replace(/\{weekday_name\}/g, WEEKDAY_NAMES[d.getDay()])
  s = s.replace(/\{counter\}/g, String(counter))
  if (mainCounter != null) {
    s = s.replace(/\{main_counter\}/g, String(mainCounter))
  } else {
    s = s.replace(/\{main_counter\}/g, '')
  }
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

function ContentPreview({ content, previewDate, counterStart, mainEvent, mainCounterStart }: { content: string; previewDate: Date | null; counterStart: number; mainEvent?: Date; mainCounterStart?: number }) {
  const { t } = useTranslation()
  if (!content.trim() || !previewDate) return null
  return (
    <div className="recurrence-form__preview">
      <span className="recurrence-form__label">{t('recurring.expandedPreview')}</span>
      <div className="recurrence-form__expanded">{expandContent(content, previewDate, counterStart, mainEvent, mainCounterStart)}</div>
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
  const [editingId, setEditingId] = useState<string | null>(null)
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [counterStart, setCounterStart] = useState(1)
  const [recState, setRecState] = useState<RecurrenceState>(() => parseRecurrenceJSON(
    JSON.stringify({ kind: 'weekly', weekdays: [1], hour: 9, minute: 0, timezone: 'UTC' }),
  ))
  const [targetIds, setTargetIds] = useState<string[]>([])
  const [shiftInputs, setShiftInputs] = useState<Record<string, string>>({})
  const [announcementEnabled, setAnnouncementEnabled] = useState(false)
  const [announcementTitle, setAnnouncementTitle] = useState('')
  const [announcementContent, setAnnouncementContent] = useState('')
  const [announcementDaysBefore, setAnnouncementDaysBefore] = useState(2)
  const [announcementCounterStart, setAnnouncementCounterStart] = useState(1)
  const [announcementDifferentTargets, setAnnouncementDifferentTargets] = useState(false)
  const [announcementTargetIds, setAnnouncementTargetIds] = useState<string[]>([])
  const [materializeHorizonDays, setMaterializeHorizonDays] = useState(0)
  const [aiEnhanceEnabled, setAiEnhanceEnabled] = useState(false)
  const [aiEnhanceAnnouncement, setAiEnhanceAnnouncement] = useState(false)
  const [outputMode, setOutputMode] = useState<AutomationOutputMode>('scheduled')
  const [promptHint, setPromptHint] = useState('')
  const [titleHint, setTitleHint] = useState('')

  const outputModeLabel: Record<AutomationOutputMode, string> = {
    draft: t('rss.outputModeDraft', { defaultValue: 'Draft (review)' }),
    scheduled: t('rss.outputModeScheduled', { defaultValue: 'Scheduled' }),
    publish_now: t('rss.outputModePublishNow', { defaultValue: 'Publish now' }),
  }

  const firstOccurrence = useMemo(() => {
    const occ = computeOccurrences(recState, 1)
    return occ.length > 0 ? occ[0] : null
  }, [recState])

  const announcementPreviewDate = useMemo(() => {
    if (!announcementEnabled || !firstOccurrence) {
      return null
    }
    const preview = new Date(firstOccurrence)
    preview.setDate(preview.getDate() - announcementDaysBefore)
    return preview
  }, [announcementEnabled, announcementDaysBefore, firstOccurrence])

  const accountById = useMemo(() => Object.fromEntries(accounts.map((a) => [a.id, a])), [accounts])

  const refresh = useCallback(async function refresh() {
    setLoading(true)
    try {
      const res = await api.listPostTemplates(teamId)
      setItems(res.items ?? [])
    } catch (e) {
      const raw = e instanceof Error ? e.message : t('status.templateLoadFailed')
      onStatus(translateApiError(raw, t))
    } finally {
      setLoading(false)
    }
  }, [api, teamId, onStatus, t])

  useEffect(() => {
    void refresh()
  }, [refresh])

  function toggleTargetAccount(accountId: string) {
    setTargetIds((cur) =>
      cur.includes(accountId) ? cur.filter((id) => id !== accountId) : [...cur, accountId],
    )
  }

  function resetEditorFields() {
    setEditingId(null)
    setTitle('')
    setContent('')
    setCounterStart(1)
    setRecState(parseRecurrenceJSON(
      JSON.stringify({ kind: 'weekly', weekdays: [1], hour: 9, minute: 0, timezone: 'UTC' }),
    ))
    setTargetIds([])
    setAnnouncementEnabled(false)
    setAnnouncementTitle('')
    setAnnouncementContent('')
    setAnnouncementDaysBefore(2)
    setAnnouncementCounterStart(1)
    setAnnouncementDifferentTargets(false)
    setAnnouncementTargetIds([])
    setMaterializeHorizonDays(0)
    setAiEnhanceEnabled(false)
    setAiEnhanceAnnouncement(false)
    setOutputMode('scheduled')
    setPromptHint('')
    setTitleHint('')
  }

  function closeEditor() {
    setEditorOpen(false)
    resetEditorFields()
  }

  function openEditor() {
    resetEditorFields()
    setEditorOpen(true)
  }

  function openEditorForEdit(item: BackendPostTemplate) {
    setEditingId(item.id)
    setTitle(item.title)
    setContent(item.content)
    setCounterStart(item.counter_next || 1)
    setRecState(parseRecurrenceJSON(item.recurrence_json))
    setTargetIds(item.target_account_ids ?? [])
    setAnnouncementEnabled(Boolean(item.announcement_enabled))
    setAnnouncementTitle(item.announcement_title ?? '')
    setAnnouncementContent(item.announcement_content ?? '')
    setAnnouncementDaysBefore(item.announcement_days_before ?? 2)
    setAnnouncementCounterStart(item.announcement_counter_next || 1)
    const annTargets = item.announcement_target_account_ids ?? []
    setAnnouncementDifferentTargets(annTargets.length > 0)
    setAnnouncementTargetIds(annTargets)
    setMaterializeHorizonDays(item.materialize_horizon_days ?? 0)
    setAiEnhanceEnabled(Boolean(item.ai_enhance_enabled))
    setAiEnhanceAnnouncement(Boolean(item.ai_enhance_announcement))
    setOutputMode(item.output_mode ?? 'scheduled')
    setPromptHint(item.prompt_hint ?? '')
    setTitleHint(item.title_hint ?? '')
    setEditorOpen(true)
  }

  function buildAutomationPayload() {
    const payload: Record<string, unknown> = {
      title: title.trim(),
      content: content.trim(),
      recurrence_json: recurrenceStateToJSON(recState),
      target_account_ids: targetIds,
      counter_next: Math.max(1, counterStart),
      materialize_horizon_days: Math.max(0, materializeHorizonDays),
    }
    payload.output_mode = outputMode
    if (team?.isAiEnabled) {
      payload.ai_enhance_enabled = aiEnhanceEnabled
      payload.ai_enhance_announcement = aiEnhanceEnabled && announcementEnabled && aiEnhanceAnnouncement
      payload.prompt_hint = promptHint.trim()
      payload.title_hint = titleHint.trim()
    }
    if (announcementEnabled) {
      payload.announcement_enabled = true
      payload.announcement_title = announcementTitle.trim()
      payload.announcement_content = announcementContent.trim()
      payload.announcement_days_before = announcementDaysBefore
      payload.announcement_counter_next = Math.max(1, announcementCounterStart)
      payload.announcement_target_account_ids = announcementDifferentTargets ? announcementTargetIds : []
    } else if (editingId) {
      payload.announcement_enabled = false
    }
    return payload
  }

  async function handleSave() {
    if (!title.trim() || !content.trim() || targetIds.length === 0) {
      onStatus(t('recurring.requiredFields'))
      return
    }
    if (announcementEnabled && !announcementContent.trim()) {
      onStatus(t('recurring.announcementContentRequired'))
      return
    }
    if (announcementEnabled && announcementDifferentTargets && announcementTargetIds.length === 0) {
      onStatus(t('recurring.announcementTargetsRequired'))
      return
    }
    onStatus(null)
    try {
      const payload = buildAutomationPayload()
      if (editingId) {
        await api.updatePostTemplate(teamId, editingId, payload)
        closeEditor()
        await refresh()
        onStatus(t('status.templateUpdated'))
      } else {
        await api.createPostTemplate(teamId, {
          ...payload,
          enabled: true,
        } as Parameters<typeof api.createPostTemplate>[1])
        closeEditor()
        await refresh()
        onStatus(t('status.templateCreated'))
      }
    } catch (e) {
      const raw = e instanceof Error ? e.message : editingId ? t('status.templateUpdateFailed') : t('status.templateCreateFailed')
      onStatus(translateApiError(raw, t))
    }
  }

  function formatTemplateSchedule(item: BackendPostTemplate): string {
    const summary = formatRecurrenceSummary(item.recurrence_json, t)
    if (!item.announcement_enabled) {
      return summary
    }
    return `${summary} · ${t('recurring.summaryAnnouncementInline', { days: item.announcement_days_before ?? 2 })}`
  }

  function toggleAnnouncementTargetAccount(accountId: string) {
    setAnnouncementTargetIds((cur) =>
      cur.includes(accountId) ? cur.filter((id) => id !== accountId) : [...cur, accountId],
    )
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
    <div className="recurring-posts-view">
      <div className="glass-panel">
        <div className="flex-row--between">
          <div>
            <h2 className="section-card__title flex-row--center gap-2">
              <CalendarClock size={20} />
              {t('recurring.title')}
            </h2>
            <p className="hint">{t('recurring.hint')}</p>
          </div>
          {canEdit ? (
            <button type="button" className="btn btn--secondary btn--sm" onClick={openEditor} data-testid="recurring-add-template">
              <Plus size={16} />
              <span>{t('recurring.addTemplate', 'Add template')}</span>
            </button>
          ) : null}
        </div>
        {loading ? <p className="hint">{t('common.loading')}</p> : null}
        {!loading && items.length === 0 ? <p className="hint">{t('recurring.noTemplates')}</p> : null}
        <div className="stack stack--sm">
          {items.map((item) => (
            <div key={item.id} className="glass-panel glass-panel--compact recurring-template-card">
              <div className="recurring-template-card__header">
                <div className="flex-row--center gap-2">
                  <span className="badge">{item.title || t('recurring.untitled')}</span>
                  {item.announcement_enabled ? <span className="badge badge--info">{t('recurring.announcementBadge')}</span> : null}
                  {item.ai_enhance_enabled ? <span className="badge badge--info">{t('recurring.aiEnabledBadge')}</span> : null}
                </div>
                {canEdit ? (
                  <div className="flex-row--center gap-1">
                    <button type="button" className="btn btn--ghost btn--xs" onClick={() => openEditorForEdit(item)} aria-label={t('recurring.editTemplate')}>
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
                            onClick={() => void toggleEnabled(item.id, item.enabled)}
                          >
                            {item.enabled ? t('recurring.pause') : t('recurring.resume')}
                          </DropdownMenu.Item>
                          <DropdownMenu.Item
                            className="radix-dropdown-item"
                            onClick={() => void skipNext(item.id, item.next_materialize_at)}
                          >
                            {t('recurring.skipNext')}
                          </DropdownMenu.Item>
                          <DropdownMenu.Item
                            className="radix-dropdown-item"
                            onClick={() => void shiftNext(item.id, item.next_materialize_at)}
                          >
                            {t('recurring.shiftNext')}
                          </DropdownMenu.Item>
                          <DropdownMenu.Separator className="divider" />
                          <DropdownMenu.Item
                            className="radix-dropdown-item"
                            onClick={() => void removeTemplate(item.id)}
                          >
                            <Trash2 size={14} /> {t('common.delete')}
                          </DropdownMenu.Item>
                        </DropdownMenu.Content>
                      </DropdownMenu.Portal>
                    </DropdownMenu.Root>
                    <ToggleSwitch
                      checked={item.enabled}
                      onChange={() => void toggleEnabled(item.id, item.enabled)}
                      title={item.enabled ? t('analytics.enabled') : t('analytics.paused')}
                      disabled={!canEdit}
                      compact
                    />
                  </div>
                ) : null}
              </div>

              <div className="recurring-template-card__meta">
                <div className="recurring-template-card__meta-row">
                  <CalendarClock size={14} />
                  <span>{formatTemplateSchedule(item)}</span>
                </div>
                <div className="recurring-template-card__meta-row">
                  <Clock size={14} />
                  <span>
                    {t('recurring.next')}{' '}
                    {item.next_materialize_at ? format(parseISO(item.next_materialize_at), 'PPpp') : t('common.emDash')}
                    {' · '}{t('common.counter')}: {item.counter_next}
                  </span>
                </div>
                <div className="recurring-template-card__meta-row">
                  <Target size={14} />
                  <span>
                    {item.target_account_ids.map((id) => accountById[id]?.username ?? id.slice(0, 8)).join(', ')}
                  </span>
                </div>
                {item.materialize_horizon_days ? (
                  <div className="recurring-template-card__meta-row">
                    <span className="hint">{t('recurring.materializeHorizonActive', { days: item.materialize_horizon_days })}</span>
                  </div>
                ) : null}
              </div>

              {shiftInputs[item.id] !== undefined ? (
                <div className="recurring-template-card__shift">
                  <input type="datetime-local" value={shiftInputs[item.id]} onChange={(e) => setShiftInputs((cur) => ({ ...cur, [item.id]: e.target.value }))} />
                  <button type="button" className="btn btn--primary btn--sm" onClick={() => void shiftNext(item.id, item.next_materialize_at)}>
                    {t('common.apply')}
                  </button>
                </div>
              ) : null}
            </div>
          ))}
        </div>
      </div>

      {editorOpen ? (
        <Dialog.Root open={editorOpen} onOpenChange={(open) => !open && closeEditor()}>
          <Dialog.Portal>
            <Dialog.Overlay className="dialog-overlay" />
            <Dialog.Content className="dialog-content dialog-content--wide" data-testid="recurring-template-dialog">
              <div className="drawer-header">
                <Dialog.Title className="drawer-title">
                  {editingId ? t('recurring.editTitle') : t('recurring.editorTitle')}
                </Dialog.Title>
                <Dialog.Close asChild>
                  <button type="button" className="btn btn--ghost btn--icon-sm" aria-label={t('common.cancel')}>
                    <X size={20} />
                  </button>
                </Dialog.Close>
              </div>
              <div className="drawer-body stack">
                <label className="field">
                  <span>{t('common.title')}</span>
                  <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t('recurring.titlePlaceholder')} />
                  <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
                    {t('recurring.titleTemplateHint')}
                  </p>
                </label>

                <label className="field">
                  <span>{t('common.content')}</span>
                  <textarea rows={5} value={content} onChange={(e) => setContent(e.target.value)} placeholder={t('recurring.contentPlaceholder')} />
                </label>

                <RecurrenceForm state={recState} onChange={setRecState} />
                <OccurrencePreview state={recState} />

                <label className="field">
                  <span>{t('recurring.counterStart')}</span>
                  <input
                    type="number"
                    min={1}
                    value={counterStart}
                    onChange={(e) => setCounterStart(Math.max(1, parseInt(e.target.value, 10) || 1))}
                    data-testid="recurring-counter-start"
                  />
                  <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
                    {t('recurring.counterStartHint')}
                  </p>
                </label>

                <ContentPreview
                  content={content}
                  previewDate={firstOccurrence}
                  counterStart={counterStart}
                />

                <label className="field">
                  <span>{t('recurring.materializeHorizon')}</span>
                  <select
                    value={materializeHorizonDays}
                    onChange={(e) => setMaterializeHorizonDays(parseInt(e.target.value, 10) || 0)}
                    data-testid="recurring-materialize-horizon"
                  >
                    <option value={0}>{t('recurring.materializeHorizonOff')}</option>
                    <option value={7}>{t('recurring.materializeHorizonWeek', { count: 1 })}</option>
                    <option value={14}>{t('recurring.materializeHorizonWeeks', { count: 2 })}</option>
                    <option value={21}>{t('recurring.materializeHorizonWeeks', { count: 3 })}</option>
                    <option value={28}>{t('recurring.materializeHorizonWeeks', { count: 4 })}</option>
                  </select>
                  <p className="hint" style={{ fontSize: '0.8rem', marginTop: '0.25rem' }}>
                    {t('recurring.materializeHorizonHint')}
                  </p>
                </label>

                <div className="field">
                  <span>{t('rss.outputMode')}</span>
                  <Segmented<AutomationOutputMode>
                    value={outputMode}
                    options={[
                      { id: 'draft', label: outputModeLabel.draft },
                      { id: 'scheduled', label: outputModeLabel.scheduled },
                      { id: 'publish_now', label: outputModeLabel.publish_now },
                    ]}
                    onChange={(v) => setOutputMode(v)}
                    testIdPrefix="recurring-output-mode"
                  />
                </div>
                {team?.isAiEnabled ? (
                  <>
                    <ToggleSwitch
                      checked={aiEnhanceEnabled}
                      onChange={setAiEnhanceEnabled}
                      title={t('rss.aiEnhanceEnabled')}
                      description="Die KI schreibt den Template-Text vor dem Veröffentlichen mit dem Markenstil neu."
                      testId="recurring-ai-enhance"
                    />
                    {aiEnhanceEnabled ? (
                      <>
                        {announcementEnabled ? (
                          <div className="field">
                            <span>{t('recurring.aiScope')}</span>
                            <Segmented<'main' | 'both'>
                              value={aiEnhanceAnnouncement ? 'both' : 'main'}
                              options={[
                                { id: 'main', label: t('recurring.aiScopeMainOnly') },
                                { id: 'both', label: t('recurring.aiScopeMainAndAnnouncement') },
                              ]}
                              onChange={(v) => setAiEnhanceAnnouncement(v === 'both')}
                              testIdPrefix="recurring-ai-scope"
                            />
                            <p className="hint">{t('recurring.aiScopeHint')}</p>
                          </div>
                        ) : null}
                        <label className="field">
                          <span>{t('rss.aiPrompt')}</span>
                          <textarea rows={3} value={promptHint} onChange={(e) => setPromptHint(e.target.value)} placeholder={t('rss.aiPromptPlaceholder')} />
                        </label>
                        <label className="field">
                          <span>{t('rss.titleHint')}</span>
                          <input value={titleHint} onChange={(e) => setTitleHint(e.target.value)} placeholder={t('rss.titleHintPlaceholder')} />
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

                <fieldset className="recurrence-form__announcement-config">
                  <legend className="recurrence-form__legend">{t('recurring.announcement')}</legend>
                  <ToggleSwitch
                    checked={announcementEnabled}
                    onChange={setAnnouncementEnabled}
                    title={t('recurring.announcementEnabled')}
                    testId="recurring-announcement-enabled"
                  />
                  {announcementEnabled ? (
                    <>
                      <p className="hint">{t('recurring.announcementHint')}</p>
                      <label className="field">
                        <span>{t('recurring.announcementDaysBefore')}</span>
                        <input
                          type="number"
                          min={1}
                          max={30}
                          value={announcementDaysBefore}
                          onChange={(e) => setAnnouncementDaysBefore(parseInt(e.target.value, 10) || 2)}
                          data-testid="recurring-announcement-days-before"
                        />
                      </label>
                      <label className="field">
                        <span>{t('recurring.announcementTitle')}</span>
                        <input
                          value={announcementTitle}
                          onChange={(e) => setAnnouncementTitle(e.target.value)}
                          placeholder={t('recurring.announcementTitlePlaceholder')}
                        />
                      </label>
                      <label className="field">
                        <span>{t('recurring.announcementContent')}</span>
                        <textarea
                          rows={4}
                          value={announcementContent}
                          onChange={(e) => setAnnouncementContent(e.target.value)}
                          placeholder={t('recurring.announcementContentPlaceholder')}
                          data-testid="recurring-announcement-content"
                        />
                      </label>
                      <label className="field">
                        <span>{t('recurring.announcementCounterStart')}</span>
                        <input
                          type="number"
                          min={1}
                          value={announcementCounterStart}
                          onChange={(e) => setAnnouncementCounterStart(Math.max(1, parseInt(e.target.value, 10) || 1))}
                          data-testid="recurring-announcement-counter-start"
                        />
                      </label>
                      <ContentPreview
                        content={announcementContent}
                        previewDate={announcementPreviewDate}
                        counterStart={announcementCounterStart}
                        mainEvent={firstOccurrence ?? undefined}
                        mainCounterStart={counterStart}
                      />
                      <ToggleSwitch
                        checked={announcementDifferentTargets}
                        onChange={setAnnouncementDifferentTargets}
                        title={t('recurring.announcementDifferentTargets')}
                      />
                      {announcementDifferentTargets ? (
                        <div className="field">
                          <span>{t('recurring.announcementTargets')}</span>
                          <DestinationPicker
                            accounts={accounts}
                            selectedIds={announcementTargetIds}
                            onToggle={toggleAnnouncementTargetAccount}
                            testIdPrefix="recurring-announcement-dest"
                          />
                        </div>
                      ) : null}
                    </>
                  ) : null}
                </fieldset>

                <div className="field">
                  <span>{t('common.targets')}</span>
                  <DestinationPicker
                    accounts={accounts}
                    selectedIds={targetIds}
                    onToggle={toggleTargetAccount}
                    testIdPrefix="recurring-template-dest"
                  />
                </div>

                <div className="flex-row--end gap-2 mt-4">
                  <Dialog.Close asChild>
                    <button type="button" className="btn btn--ghost">{t('common.cancel')}</button>
                  </Dialog.Close>
                  <button type="button" className="btn btn--primary" onClick={() => void handleSave()} data-testid="recurring-template-save">
                    {editingId ? t('common.save') : t('common.create')}
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
