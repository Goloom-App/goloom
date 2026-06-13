import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { History } from 'lucide-react'

import { SectionCard } from '../../components/ui'
import type { BackendAuditEvent, createApiClient } from '../../api'

// Known audited actions, used for the filter dropdown. Mirrors the action
// strings recorded server-side in api/*.go.
const AUDIT_ACTIONS = [
  'post.create',
  'post.update',
  'post.delete',
  'post.cancel',
  'account.connect',
  'account.disconnect',
  'account.update',
  'member.add',
  'member.remove',
  'team.update',
  'ai_config.update',
  'brand_profile.update',
  'api_token.create',
]

function actionLabel(t: TFunction, action: string): string {
  return t(`audit.actions.${action.replace(/\./g, '_')}`, action)
}

export function TeamAuditLogSection({
  api,
  teamId,
}: {
  api: ReturnType<typeof createApiClient> | null
  teamId: string
}) {
  const { t } = useTranslation()
  const [entries, setEntries] = useState<BackendAuditEvent[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [actionFilter, setActionFilter] = useState('')
  const [offset, setOffset] = useState(0)
  const limit = 25

  const load = useCallback(async () => {
    if (!api) return
    setLoading(true)
    try {
      const res = await api.listTeamAuditLog(teamId, {
        action: actionFilter || undefined,
        limit,
        offset,
      })
      setEntries(res.entries ?? [])
      setTotal(res.total ?? 0)
    } catch {
      setEntries([])
      setTotal(0)
    } finally {
      setLoading(false)
    }
  }, [api, teamId, actionFilter, offset])

  useEffect(() => {
    void load()
  }, [load])

  const pageCount = Math.max(1, Math.ceil(total / limit))
  const currentPage = Math.floor(offset / limit) + 1

  return (
    <SectionCard icon={<History size={18} />} title={t('audit.title')} subtitle={t('audit.hint')}>
      <div className="inline-cluster flex-wrap mb-1">
        <select
          className="select--sm"
          value={actionFilter}
          onChange={(e) => {
            setActionFilter(e.target.value)
            setOffset(0)
          }}
          aria-label={t('audit.filterAction')}
        >
          <option value="">{t('audit.allActions')}</option>
          {AUDIT_ACTIONS.map((action) => (
            <option key={action} value={action}>
              {actionLabel(t, action)}
            </option>
          ))}
        </select>
      </div>

      {loading ? (
        <p className="hint">{t('common.loading')}</p>
      ) : entries.length === 0 ? (
        <p className="hint">{t('audit.empty')}</p>
      ) : (
        <>
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>{t('audit.colTime')}</th>
                  <th>{t('audit.colActor')}</th>
                  <th>{t('audit.colAction')}</th>
                  <th>{t('audit.colDetails')}</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((e) => (
                  <tr key={e.id}>
                    <td className="text-nowrap text-sm">{new Date(e.created_at).toLocaleString()}</td>
                    <td>
                      <div className="audit-actor">
                        <strong>{e.actor_name || e.actor_email || t('audit.unknownActor')}</strong>
                        {e.actor_kind === 'api_token' ? (
                          <span className="badge badge--component badge--component-mcp">
                            {t('audit.viaApiKey', { name: e.token_name || '—' })}
                          </span>
                        ) : (
                          <span className="badge badge--default">{t('audit.member')}</span>
                        )}
                      </div>
                    </td>
                    <td className="text-nowrap">{actionLabel(t, e.action)}</td>
                    <td className="text-sm">{e.summary}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="inline-cluster mt-1" style={{ alignItems: 'center' }}>
            <button
              type="button"
              className="btn btn--sm"
              disabled={offset <= 0}
              onClick={() => setOffset(Math.max(0, offset - limit))}
            >
              {t('admin.logPrevious')}
            </button>
            <span className="text-sm hint">{t('admin.logPageOf', { page: currentPage, total: pageCount })}</span>
            <button
              type="button"
              className="btn btn--sm"
              disabled={offset + limit >= total}
              onClick={() => setOffset(offset + limit)}
            >
              {t('admin.logNext')}
            </button>
          </div>
        </>
      )}
    </SectionCard>
  )
}
