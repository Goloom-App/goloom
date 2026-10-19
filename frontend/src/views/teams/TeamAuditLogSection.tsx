import { useEffect, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { ChevronDown, ChevronUp, History } from 'lucide-react'

import { SectionCard } from '../../components/ui'
import type { BackendAuditEvent, createApiClient } from '../../api'

// Keep historical aliases filterable; the input also accepts unknown action names.
const AUDIT_ACTIONS = [
  'post.create', 'post.update', 'post.delete', 'post.cancel', 'post.versions_update',
  'post.automation_create', 'post.publish_started', 'post.publish_success', 'post.publish_failure', 'post.publish_uncertain',
  'post.repair', 'post.retry', 'post.acknowledge', 'post.import', 'post.cleanup',
  'account.connect', 'account.disconnect', 'account.update', 'account.migrate_in', 'account.migrate_out', 'account.import',
  'member.add', 'member.remove', 'member.role_change', 'team.create', 'team.update',
  'ai_config.update', 'brand_profile.update', 'brand_profile.delete',
  'api_token.create', 'api_token.revoke', 'invitation.create', 'invitation.revoke', 'invitation.accept',
  'campaign_format.create', 'campaign_format.update', 'campaign_format.delete', 'campaign.create',
  'style_example.create', 'style_example.delete',
  'knowledge_source.create', 'knowledge_source.update', 'knowledge_source.delete',
  'rss_feed.create', 'rss_feed.update', 'rss_feed.delete',
  'recurring.create', 'recurring.update', 'recurring.delete', 'recurring.skip', 'recurring.shift', 'recurring.regenerate',
  'media.upload', 'media.rename', 'media.delete', 'external_monitor.update', 'proactive_settings.update',
  'ai_job.submit', 'ai_job.cancel', 'ai_job.complete', 'ai_job.fallback', 'ai_job.fail',
]

// Render only documented safe metadata fields.
const DETAIL_KEYS = new Set([
  'channel', 'outcome', 'changed_fields', 'role_before', 'role_after', 'previous_role', 'role',
  'request_id', 'correlation_id', 'job_id', 'feed_id', 'template_id', 'occurrence', 'occurrence_id',
  'post_id', 'account_id', 'source_team_id', 'destination_team_id', 'initiator_user_id',
  'initiator_token_id', 'error_code', 'created_count', 'deleted_count', 'imported_count',
  'occurrence_at', 'shift_to', 'mode', 'size_bytes', 'provider', 'source', 'attempt', 'item_key_hash',
  'removed_count', 'status_before', 'status_after', 'author_user_id', 'attempt_id',
])

function actionLabel(t: TFunction, action: string): string {
  return t(`audit.actions.${action.replace(/\./g, '_')}`, action)
}

function AuditDetails({ entry }: { entry: BackendAuditEvent }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  const detailsId = useId()
  const metadata = Object.entries(entry.metadata ?? {}).filter(([key]) => DETAIL_KEYS.has(key))
  const target = entry.target_type ? `${entry.target_type}${entry.target_id ? `: ${entry.target_id}` : ''}` : ''
  const preview = entry.summary || target || (metadata[0] ? `${metadata[0][0]}: ${String(metadata[0][1])}` : '—')
  const hasDetails = Boolean(target || metadata.length)

  return (
    <div className="audit-details">
      <div className="audit-details__summary">{preview}</div>
      {hasDetails && (
        <>
          <button
            type="button"
            className="btn btn--sm audit-details__toggle"
            aria-expanded={expanded}
            aria-controls={detailsId}
            onClick={() => setExpanded((value) => !value)}
          >
            {expanded ? <ChevronUp size={14} aria-hidden="true" /> : <ChevronDown size={14} aria-hidden="true" />}
            {t(expanded ? 'audit.hideDetails' : 'audit.showDetails')}
          </button>
          <div id={detailsId} hidden={!expanded}>
            {expanded && (
              <>
                {target && <div className="hint">{target}</div>}
                {metadata.map(([key, value]) => (
                  <div key={key} className="hint">
                    {key}: {key === 'changed_fields' ? String(value).split(',').map((field) => field.trim()).join(', ') : String(value)}
                  </div>
                ))}
              </>
            )}
          </div>
        </>
      )}
    </div>
  )
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
          <table className="data-table audit-table">
            <colgroup>
              <col style={{ width: '18%' }} />
              <col style={{ width: '20%' }} />
              <col style={{ width: '26%' }} />
              <col style={{ width: '36%' }} />
            </colgroup>
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
                  <td className="text-sm">{new Date(e.created_at).toLocaleString()}</td>
                  <td>
                    <div className="audit-actor">
                      <strong>{e.actor_name || e.actor_email || t(e.actor_kind === 'system' ? 'audit.automation' : 'audit.unknownActor')}</strong>
                      {e.actor_kind === 'api_token' ? (
                        <span className="badge badge--component badge--component-mcp">
                          {t('audit.viaApiKey', { name: e.token_name || '—' })}
                        </span>
                      ) : (
                        <span className="badge badge--default">{t(e.actor_kind === 'system' ? 'audit.automation' : (e.actor_kind === 'oidc' || e.actor_kind === 'user') ? 'audit.member' : 'audit.unknownActor')}</span>
                      )}
                      {e.actor_name && e.actor_email && <span className="hint text-sm">{e.actor_email}</span>}
                    </div>
                  </td>
                  <td>{actionLabel(t, e.action)}</td>
                  <td className="text-sm">
                    <AuditDetails entry={e} />
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
