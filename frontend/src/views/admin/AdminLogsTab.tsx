import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Icon } from '../../icons'
import { OptionPill } from '../../components/ui'
import type { BackendLogEntry } from '../../api'

type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR'

const LOG_LEVELS: LogLevel[] = ['DEBUG', 'INFO', 'WARN', 'ERROR']

// Display labels for the technical component buckets derived server-side from
// the log's source file (see domain.LogComponentFromSource).
const COMPONENT_LABELS: Record<string, string> = {
  ai: 'AI',
  mcp: 'MCP',
  automation: 'Automation',
  provider: 'Provider',
  api: 'API',
  system: 'System',
}

function componentLabel(component: string): string {
  return COMPONENT_LABELS[component] ?? component
}

function levelClass(level: string): string {
  switch (level) {
    case 'ERROR':
      return 'badge badge--danger'
    case 'WARN':
      return 'badge badge--warn'
    case 'DEBUG':
      return 'badge badge--neutral'
    default:
      return 'badge badge--info'
  }
}

export function AdminLogsTab({ api }: { api: ReturnType<typeof import('../../api').createApiClient> | null }) {
  const { t } = useTranslation()
  const [entries, setEntries] = useState<BackendLogEntry[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [levelFilter, setLevelFilter] = useState('')
  const [searchFilter, setSearchFilter] = useState('')
  const [componentFilter, setComponentFilter] = useState('')
  const [availableComponents, setAvailableComponents] = useState<string[]>([])
  const [showArchived, setShowArchived] = useState(false)
  const [offset, setOffset] = useState(0)
  const limit = 50

  const load = useCallback(async () => {
    if (!api) return
    setLoading(true)
    try {
      const res = await api.listLogEntries({
        level: levelFilter || undefined,
        search: searchFilter || undefined,
        component: componentFilter || undefined,
        archived: showArchived || undefined,
        limit,
        offset,
      })
      // The API returns null (not []) when no rows match; coalesce so the
      // render never crashes on entries.length / entries.map (blank screen).
      setEntries(res.entries ?? [])
      setTotal(res.total ?? 0)
      if (res.components) {
        setAvailableComponents(res.components)
      }
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [api, levelFilter, searchFilter, componentFilter, showArchived, offset])

  useEffect(() => {
    void load()
  }, [load])

  const handleArchive = async (id: string) => {
    if (!api) return
    try {
      await api.archiveLogEntry(id)
      void load()
    } catch {
      /* ignore */
    }
  }

  const handleUnarchive = async (id: string) => {
    if (!api) return
    try {
      await api.unarchiveLogEntry(id)
      void load()
    } catch {
      /* ignore */
    }
  }

  const handleDelete = async (id: string) => {
    if (!api) return
    try {
      await api.deleteLogEntry(id)
      void load()
    } catch {
      /* ignore */
    }
  }

  const handlePrune = async () => {
    if (!api) return
    try {
      await api.pruneLogEntries()
      void load()
    } catch {
      /* ignore */
    }
  }

  const pageCount = Math.max(1, Math.ceil(total / limit))
  const currentPage = Math.floor(offset / limit) + 1

  return (
    <div className="admin-tab-panel stack stack--lg">
      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <div>
            <h2 className="admin-section__title">{t('admin.logs')}</h2>
            <p className="hint admin-section__hint">{t('admin.logsHint')}</p>
          </div>
        </header>

        {/* Filters */}
        <div className="admin-log-filters" style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', marginBottom: '1rem' }}>
          <select
            className="input input--sm"
            value={levelFilter}
            onChange={(e) => { setLevelFilter(e.target.value); setOffset(0) }}
            aria-label={t('admin.logLevelFilter')}
          >
            <option value="">{t('admin.logAllLevels')}</option>
            {LOG_LEVELS.map((lvl) => (
              <option key={lvl} value={lvl}>
                {lvl}
              </option>
            ))}
          </select>

          <select
            className="input input--sm"
            value={componentFilter}
            onChange={(e) => { setComponentFilter(e.target.value); setOffset(0) }}
            aria-label={t('admin.logComponentFilter')}
          >
            <option value="">{t('admin.logAllComponents')}</option>
            {availableComponents.map((c) => (
              <option key={c} value={c}>
                {componentLabel(c)}
              </option>
            ))}
          </select>

          <input
            className="input input--sm"
            type="text"
            placeholder={t('admin.logSearchPlaceholder')}
            value={searchFilter}
            onChange={(e) => { setSearchFilter(e.target.value); setOffset(0) }}
            aria-label={t('admin.logSearchPlaceholder')}
          />

          <OptionPill
            active={showArchived}
            onClick={() => { setShowArchived(!showArchived); setOffset(0) }}
          >
            {t('admin.logShowArchived')}
          </OptionPill>

          <button type="button" className="button button--sm" onClick={() => void handlePrune()}>
            <Icon name="archive" className="inline-icon" />
            {t('admin.logPrune')}
          </button>
        </div>

        {/* Loading */}
        {loading ? (
          <p className="hint">{t('common.loading')}</p>
        ) : entries.length === 0 ? (
          <p className="hint">{t('admin.logNoEntries')}</p>
        ) : (
          <>
            {/* Table */}
            <div className="table-wrap">
              <table className="table admin-log-table">
                <thead>
                  <tr>
                    <th>{t('admin.logLevel')}</th>
                    <th>{t('admin.logComponent')}</th>
                    <th>{t('admin.logTime')}</th>
                    <th>{t('admin.logMessage')}</th>
                    <th>{t('admin.logSource')}</th>
                    <th>{t('admin.logActions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((e) => (
                    <tr key={e.id} className={e.archived_at ? 'opacity-50' : ''}>
                      <td>
                        <span className={levelClass(e.level)}>{e.level}</span>
                      </td>
                      <td>
                        {e.component ? (
                          <span className={`badge badge--component badge--component-${e.component}`}>
                            {componentLabel(e.component)}
                          </span>
                        ) : null}
                      </td>
                      <td className="text-nowrap text-sm">
                        {new Date(e.created_at).toLocaleString()}
                      </td>
                      <td className="admin-log-message">
                        <code className="inline-code">{e.message}</code>
                        {e.attributes && Object.keys(e.attributes).length > 0 && (
                          <details className="admin-log-attrs">
                            <summary className="hint text-xs">{t('admin.logAttributes')}</summary>
                            <pre className="text-xs">{JSON.stringify(e.attributes, null, 2)}</pre>
                          </details>
                        )}
                      </td>
                      <td className="text-xs text-nowrap">
                        {e.source_file ? `${e.source_file}:${e.source_line ?? ''}` : '-'}
                      </td>
                      <td>
                        <div style={{ display: 'flex', gap: '0.25rem' }}>
                          {e.archived_at ? (
                            <button
                              type="button"
                              className="button button--sm button--ghost"
                              title={t('admin.logUnarchive')}
                              onClick={() => void handleUnarchive(e.id)}
                            >
                              <Icon name="refresh" className="inline-icon" />
                            </button>
                          ) : (
                            <button
                              type="button"
                              className="button button--sm button--ghost"
                              title={t('admin.logArchive')}
                              onClick={() => void handleArchive(e.id)}
                            >
                              <Icon name="archive" className="inline-icon" />
                            </button>
                          )}
                          <button
                            type="button"
                            className="button button--sm button--ghost button--danger"
                            title={t('common.delete')}
                            onClick={() => void handleDelete(e.id)}
                          >
                            <Icon name="close" className="inline-icon" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            <div className="admin-log-pagination" style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', marginTop: '0.75rem' }}>
              <button
                type="button"
                className="button button--sm"
                disabled={offset <= 0}
                onClick={() => setOffset(Math.max(0, offset - limit))}
              >
                {t('admin.logPrevious')}
              </button>
              <span className="text-sm hint">
                {t('admin.logPageOf', { page: currentPage, total: pageCount })}
              </span>
              <button
                type="button"
                className="button button--sm"
                disabled={offset + limit >= total}
                onClick={() => setOffset(offset + limit)}
              >
                {t('admin.logNext')}
              </button>
            </div>
          </>
        )}
      </section>
    </div>
  )
}
