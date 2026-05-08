import { format, parseISO } from 'date-fns'
import { useEffect, useMemo, useState } from 'react'
import { createApiClient } from '../../api'
import type { BackendPostTemplate } from '../../api'
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

  async function refresh() {
    setLoading(true)
    try {
      const res = await api.listPostTemplates(teamId)
      setItems(res.items ?? [])
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
  }, [teamId])

  async function handleCreate() {
    if (!title.trim() || !content.trim() || targetIds.length === 0) {
      onStatus('Title, content, and at least one account are required.')
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
      onStatus('Template created')
    } catch (e) {
      onStatus(e instanceof Error ? e.message : 'Failed to create template')
    }
  }

  async function toggleEnabled(id: string, currentEnabled: boolean) {
    try {
      await api.updatePostTemplate(teamId, id, { enabled: !currentEnabled })
      await refresh()
    } catch (e) {
      onStatus(e instanceof Error ? e.message : 'Update failed')
    }
  }

  async function removeTemplate(id: string) {
    if (!window.confirm('Delete this recurring template?')) {
      return
    }
    try {
      await api.deletePostTemplate(teamId, id)
      await refresh()
    } catch (e) {
      onStatus(e instanceof Error ? e.message : 'Delete failed')
    }
  }

  async function skipNext(id: string, nextIso?: string) {
    if (!nextIso) {
      onStatus('No scheduled occurrence to skip.')
      return
    }
    try {
      await api.skipPostTemplateOccurrence(teamId, id, nextIso)
      await refresh()
      onStatus('Occurrence skipped')
    } catch (e) {
      onStatus(e instanceof Error ? e.message : 'Skip failed')
    }
  }

  return (
    <div className="recurring-posts-view two-column-detail">
      <div className="glass-panel">
        <div className="inline-cluster" style={{ justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap' }}>
          <div>
            <h2 className="section-card__title">Recurring posts</h2>
            <p className="hint">
              Templates expand into scheduled posts using <code className="inline-code">{'{year}'}</code>,{' '}
              <code className="inline-code">{'{month}'}</code>, <code className="inline-code">{'{day}'}</code>,{' '}
              <code className="inline-code">{'{counter}'}</code>. Use weekly or monthly recurrence JSON (
              <code className="inline-code">monthly_anchor_offset</code> supports “days before” anchors).
            </p>
          </div>
          {canEdit ? (
            <button type="button" className="button button--primary" onClick={() => setEditorOpen(true)}>
              New template
            </button>
          ) : null}
        </div>
        {loading ? <p className="hint">Loading…</p> : null}
        {!loading && items.length === 0 ? <p className="hint">No templates yet.</p> : null}
        <ul className="recurring-template-list">
          {items.map((t) => (
            <li key={t.id} className="glass-panel recurring-template-card">
              <div className="recurring-template-card__header">
                <strong>{t.title || '(untitled)'}</strong>
                <span className="hint">{t.enabled ? 'enabled' : 'paused'}</span>
              </div>
              <p className="hint monospace-small">{t.recurrence_json}</p>
              <p className="hint">
                Next:{' '}
                {t.next_materialize_at ? format(parseISO(t.next_materialize_at), 'PPpp') : '—'} · Counter: {t.counter_next}
              </p>
              <p className="hint">
                Targets:{' '}
                {t.target_account_ids.map((id) => accountById[id]?.username ?? id.slice(0, 8)).join(', ')}
              </p>
              {canEdit ? (
                <div className="inline-cluster" style={{ marginTop: '0.75rem', flexWrap: 'wrap' }}>
                  <button type="button" className="button button--secondary" onClick={() => void toggleEnabled(t.id, t.enabled)}>
                    {t.enabled ? 'Pause' : 'Resume'}
                  </button>
                  <button type="button" className="button button--secondary" onClick={() => void skipNext(t.id, t.next_materialize_at)}>
                    Skip next
                  </button>
                  <button type="button" className="button button--secondary" onClick={() => void removeTemplate(t.id)}>
                    Delete
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
            <h3 className="section-card__title">New recurring template</h3>
            <label className="field">
              <span>Title</span>
              <input value={title} onChange={(e) => setTitle(e.target.value)} />
            </label>
            <label className="field">
              <span>Content</span>
              <textarea rows={5} value={content} onChange={(e) => setContent(e.target.value)} placeholder="Happy {year}-{month}-{day}! (#{counter})" />
            </label>
            <label className="field">
              <span>Recurrence (JSON)</span>
              <textarea rows={10} className="monospace-small" value={recurrenceJson} onChange={(e) => setRecurrenceJson(e.target.value)} />
            </label>
            <p className="hint">Targets</p>
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
            <div className="inline-cluster" style={{ marginTop: '1rem' }}>
              <button type="button" className="button button--primary" onClick={() => void handleCreate()}>
                Create
              </button>
              <button type="button" className="button button--secondary" onClick={() => setEditorOpen(false)}>
                Cancel
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}
