import { format, parseISO } from 'date-fns'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { createApiClient } from '../../api'
import type { BackendPostTemplate } from '../../api'
import { translateApiError } from '../../i18n/translateApiError'
import type { AccountRecord } from '../../types'

type Api = ReturnType<typeof createApiClient>

export function RecurringPostsView({
  teamId,
  api,
  accounts,
  canEdit,
  onStatus,
}: {
  teamId: string
  api: Api
  accounts: AccountRecord[]
  canEdit: boolean
  onStatus: (msg: string | null) => void
}) {
  const { t } = useTranslation()
  const [items, setItems] = useState<BackendPostTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [editorOpen, setEditorOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [recurrenceJson, setRecurrenceJson] = useState(
    JSON.stringify(
      {
        kind: 'weekly',
        weekdays: [1],
        hour: 9,
        minute: 0,
        timezone: 'UTC',
      },
      null,
      2,
    ),
  )
  const [targetIds, setTargetIds] = useState<string[]>([])

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

  async function handleCreate() {
    if (!title.trim() || !content.trim() || targetIds.length === 0) {
      onStatus(t('recurring.requiredFields'))
      return
    }
    onStatus(null)
    try {
      await api.createPostTemplate(teamId, {
        title: title.trim(),
        content: content.trim(),
        recurrence_json: recurrenceJson,
        target_account_ids: targetIds,
        enabled: true,
      })
      setEditorOpen(false)
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

  return (
    <div className="recurring-posts-view two-column-detail">
      <div className="glass-panel">
        <div className="flex-row--wrap" style={{ justifyContent: 'space-between' }}>
          <div>
            <h2 className="section-card__title">{t('recurring.title')}</h2>
            <p className="hint">{t('recurring.hint')}</p>
          </div>
          {canEdit ? (
            <button type="button" className="button button--primary" onClick={() => setEditorOpen(true)}>
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
                <span className="hint">{item.enabled ? t('analytics.enabled') : t('analytics.paused')}</span>
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
        <div className="modal-backdrop" onClick={() => setEditorOpen(false)}>
          <div className="glass-panel recurring-editor-modal" onClick={(e) => e.stopPropagation()}>
            <h3 className="section-card__title">{t('recurring.editorTitle')}</h3>
            <label className="field">
              <span>{t('common.title')}</span>
              <input value={title} onChange={(e) => setTitle(e.target.value)} />
            </label>
            <label className="field">
              <span>{t('common.content')}</span>
              <textarea rows={5} value={content} onChange={(e) => setContent(e.target.value)} placeholder={t('recurring.contentPlaceholder')} />
            </label>
            <label className="field">
              <span>{t('recurring.recurrenceJson')}</span>
              <textarea rows={10} className="monospace-small" value={recurrenceJson} onChange={(e) => setRecurrenceJson(e.target.value)} />
            </label>
            <p className="hint">{t('common.targets')}</p>
            <div className="composer-destination-row">
              {accounts.map((a) => {
                const on = targetIds.includes(a.id)
                return (
                  <button
                    key={a.id}
                    type="button"
                    className={`composer-destination-toggle ${on ? 'composer-destination-toggle--selected' : ''}`}
                    onClick={() =>
                      setTargetIds((cur) => (cur.includes(a.id) ? cur.filter((x) => x !== a.id) : [...cur, a.id]))
                    }
                  >
                    @{a.username}
                  </button>
                )
              })}
            </div>
            <div className="inline-cluster mt-1">
              <button type="button" className="button button--primary" onClick={() => void handleCreate()}>
                {t('common.create')}
              </button>
              <button type="button" className="button button--secondary" onClick={() => setEditorOpen(false)}>
                {t('common.cancel')}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}
