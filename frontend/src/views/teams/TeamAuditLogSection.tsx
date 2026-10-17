import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { History } from 'lucide-react'

import { SectionCard } from '../../components/ui'
import type { BackendAuditEvent, createApiClient } from '../../api'

// Keep historical aliases filterable; the input also accepts unknown action names.
const AUDIT_ACTIONS = [
  'post.create', 'post.update', 'post.delete', 'post.cancel', 'post.versions_update',
  'post.automation_create', 'post.publish_started', 'post.publish_success', 'post.publish_failure', 'post.publish_uncertain',
  'post.retry', 'post.acknowledge', 'post.import', 'post.cleanup',
  'account.connect', 'account.disconnect', 'account.update', 'account.migrate_in', 'account.migrate_out', 'account.import',
  'member.add', 'member.remove', 'member.role_change', 'team.create', 'team.update',
  'ai_config.update', 'brand_profile.update', 'brand_profile.delete',
  'api_token.create', 'api_token.revoke', 'invitation.create', 'invitation.revoke', 'invitation.accept',
  'campaign_format.create', 'campaign_format.update', 'campaign_format.delete', 'campaign.create',
  'style_example.create', 'style_example.delete',
  'knowledge_source.create', 'knowledge_source.update', 'knowledge_source.delete',
  'rss_feed.create', 'rss_feed.update', 'rss_feed.delete',
  'recurring.create', 'recurring.update', 'recurring.delete', 'recurring.skip', 'recurring.shift', 'recurring.regenerate',
  'media.upload', 'media.delete', 'external_monitor.update', 'proactive_settings.update',
  'ai_job.submit', 'ai_job.cancel', 'ai_job.complete', 'ai_job.fallback', 'ai_job.fail',
]

// Render only documented safe metadata fields.
const DETAIL_KEYS = new Set([
  'channel', 'outcome', 'changed_fields', 'role_before', 'role_after', 'previous_role', 'role',
  'request_id', 'correlation_id', 'job_id', 'feed_id', 'template_id', 'occurrence', 'occurrence_id',
  'post_id', 'account_id', 'source_team_id', 'destination_team_id', 'initiator_user_id',
  'initiator_token_id', 'error_code', 'created_count', 'deleted_count', 'imported_count',
  'occurrence_at', 'shift_to', 'mode', 'size_bytes', 'provider', 'source', 'attempt', 'item_key_hash',
])

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
  const [failed, setFailed] = useState(false)
  const [revision, setRevision] = useState(0)
  const [actionFilter, setActionFilter] = useState('')
  const [offset, setOffset] = useState(0)
  const limit = 25

  useEffect(() => {
    let current = true
    setLoading(true)
    setFailed(false)
    if (!api) {
      setFailed(true)
      setLoading(false)
      return
    }
    void api.listTeamAuditLog(teamId, { action: actionFilter || undefined, limit, offset })
      .then((res) => {
        if (!current) return
        setEntries(res.entries ?? [])
        setTotal(res.total ?? 0)
      })
      .catch(() => { if (current) setFailed(true) })
      .finally(() => { if (current) setLoading(false) })
    return () => { current = false }
  }, [api, teamId, actionFilter, offset, revision])

  useEffect(() => {
    const refresh = () => setRevision((value) => value + 1)
    const onMutation = (event: Event) => {
      if ((event as CustomEvent<string>).detail === teamId) refresh()
    }
    const interval = window.setInterval(() => {
      if (document.visibilityState === 'visible') refresh()
    }, 30_000)
    window.addEventListener('focus', refresh)
    window.addEventListener('goloom:team-mutation', onMutation)
    return () => {
      window.clearInterval(interval)
      window.removeEventListener('focus', refresh)
      window.removeEventListener('goloom:team-mutation', onMutation)
    }
  }, [teamId])

  const pageCount = Math.max(1, Math.ceil(total / limit))
  const currentPage = Math.floor(offset / limit) + 1

  return (
    <SectionCard icon={<History size={18} />} title={t('audit.title')} subtitle={t('audit.hint')}>
      <div className="inline-cluster flex-wrap mb-1">
        <input
          className="select--sm"
          list="audit-actions"
          value={actionFilter}
          placeholder={t('audit.allActions')}
          onChange={(e) => {
            setActionFilter(e.target.value)
            setOffset(0)
          }}
          aria-label={t('audit.filterAction')}
        />
        <datalist id="audit-actions">
          {AUDIT_ACTIONS.map((action) => (
            <option key={action} value={action}>{actionLabel(t, action)}</option>
          ))}
        </datalist>
        <button type="button" className="btn btn--sm" onClick={() => setRevision((value) => value + 1)} disabled={loading}>
          {t('audit.refresh')}
        </button>
      </div>

      {loading ? (
        <p className="hint">{t('common.loading')}</p>
      ) : failed ? (
        <p role="alert">{t('audit.loadFailed')}</p>
      ) : entries.length === 0 ? (
        <p className="hint">{t(offset > 0 ? 'audit.emptyPage' : 'audit.empty')}</p>
      ) : (
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
                      <strong>{e.actor_name || e.actor_email || t(e.actor_kind === 'system' ? 'audit.automation' : 'audit.unknownActor')}</strong>
                      {e.actor_kind === 'api_token' ? (
                        <span className="badge badge--component badge--component-mcp">
                          {t('audit.viaApiKey', { name: e.token_name || '—' })}
                        </span>
                      ) : (
                        <span className="badge badge--default">{t(e.actor_kind === 'system' ? 'audit.automation' : e.actor_kind === 'user' ? 'audit.member' : 'audit.unknownActor')}</span>
                      )}
                      {e.actor_name && e.actor_email && <span className="hint text-sm">{e.actor_email}</span>}
                    </div>
                  </td>
                  <td className="text-nowrap">{actionLabel(t, e.action)}</td>
                  <td className="text-sm">
                    <div>{e.summary}</div>
                    {e.target_type && <div className="hint">{e.target_type}{e.target_id ? `: ${e.target_id}` : ''}</div>}
                    {Object.entries(e.metadata ?? {}).filter(([key]) => DETAIL_KEYS.has(key)).map(([key, value]) => (
                      <div key={key} className="hint">{key}: {String(value)}</div>
                    ))}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {!loading && !failed && (total > 0 || offset > 0) && (
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
      )}
    </SectionCard>
  )
}
