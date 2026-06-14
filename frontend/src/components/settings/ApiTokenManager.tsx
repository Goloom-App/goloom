import { useMemo, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { format, isValid, parseISO } from 'date-fns'
import { useTranslation } from 'react-i18next'

import type { BackendAPIToken } from '../../api'
import { apiTokenDisplayName, isApiTokenExpired } from '../../views/settings/apiTokens'

// Scopes are grouped so the modal reads as read / write / delete families. The
// keys match the backend scope vocabulary (internal/auth/scopes.go).
const SCOPE_GROUPS: { group: string; scopes: string[] }[] = [
  { group: 'read', scopes: ['read'] },
  { group: 'write', scopes: ['write', 'write:draft', 'write:schedule'] },
  { group: 'delete', scopes: ['delete', 'delete:draft', 'delete:schedule'] },
]

export interface ApiTokenManagerTeam {
  id: string
  name: string
}

export interface CreateApiTokenPayload {
  name: string
  description?: string
  expires_at?: string
  scopes?: string[]
  team_id?: string
}

function defaultExpiryYmd(): string {
  const d = new Date()
  d.setUTCDate(d.getUTCDate() + 90)
  return d.toISOString().slice(0, 10)
}

export function ApiTokenManager({
  teams,
  tokens,
  loading,
  syncing,
  createToken,
  removeToken,
}: {
  teams: ApiTokenManagerTeam[]
  tokens: BackendAPIToken[]
  loading: boolean
  syncing: boolean
  createToken: (payload: CreateApiTokenPayload) => Promise<string>
  removeToken: (tokenID: string, expired: boolean) => Promise<void>
}) {
  const { t } = useTranslation()
  const [createOpen, setCreateOpen] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [teamId, setTeamId] = useState('all')
  const [expiresYmd, setExpiresYmd] = useState(defaultExpiryYmd)
  const [scopes, setScopes] = useState<string[]>([])
  const [formError, setFormError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  const [reveal, setReveal] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  const teamName = useMemo(() => {
    const map = new Map(teams.map((tm) => [tm.id, tm.name]))
    return (id?: string) => (id ? map.get(id) ?? id : null)
  }, [teams])

  function resetForm() {
    setName('')
    setDescription('')
    setTeamId('all')
    setExpiresYmd(defaultExpiryYmd())
    setScopes([])
    setFormError(null)
  }

  function toggleScope(scope: string) {
    setScopes((prev) => (prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]))
  }

  async function handleCreate() {
    if (!name.trim()) {
      return
    }
    const expEnd = new Date(`${expiresYmd}T23:59:59.999Z`)
    if (!expiresYmd.trim() || Number.isNaN(expEnd.getTime()) || expEnd.getTime() <= Date.now()) {
      setFormError(t('common.expiryHint'))
      return
    }
    setSubmitting(true)
    setFormError(null)
    try {
      const token = await createToken({
        name: name.trim(),
        description: description.trim() || undefined,
        expires_at: expEnd.toISOString(),
        scopes: scopes.length > 0 ? scopes : undefined,
        team_id: teamId !== 'all' ? teamId : undefined,
      })
      setCreateOpen(false)
      resetForm()
      setCopied(false)
      setReveal(token)
    } catch (cause) {
      setFormError(cause instanceof Error ? cause.message : t('common.actionFailed'))
    } finally {
      setSubmitting(false)
    }
  }

  async function copyToken() {
    if (!reveal) {
      return
    }
    try {
      await navigator.clipboard.writeText(reveal)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard can be blocked; the token stays visible for manual copy.
    }
  }

  return (
    <div className="glass-panel">
      <div className="inline-cluster" style={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <div>
          <h2 className="section-card__title">{t('settings.apiTokens')}</h2>
          <p className="hint">{t('settings.apiTokensHint')}</p>
        </div>
        <button type="button" className="button button--primary" onClick={() => { resetForm(); setCreateOpen(true) }}>
          {t('settings.newToken')}
        </button>
      </div>

      {loading ? <p className="hint">{t('common.loadingTokens')}</p> : null}
      <table className="data-table">
        <thead>
          <tr>
            <th>{t('settings.label')}</th>
            <th>{t('settings.tokenScopes')}</th>
            <th>{t('settings.tokenTeam')}</th>
            <th>{t('settings.created')}</th>
            <th>{t('settings.expires')}</th>
            <th>{t('settings.lastUsed')}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {tokens.map((token) => {
            const expired = isApiTokenExpired(token)
            const label = apiTokenDisplayName(token.name)
            return (
              <tr key={token.id}>
                <td>
                  <div className="post-table-title">{label}</div>
                  {token.description ? <div className="hint text-xs">{token.description}</div> : null}
                </td>
                <td>
                  {token.scopes && token.scopes.length > 0 ? (
                    <span className="token-scope-chips">
                      {token.scopes.map((s) => (
                        <span key={s} className="badge">{s}</span>
                      ))}
                    </span>
                  ) : (
                    <span className="hint">{t('settings.scopeFull')}</span>
                  )}
                </td>
                <td>{teamName(token.team_id) ?? t('common.allAccounts')}</td>
                <td>{format(parseISO(token.created_at), 'PPp')}</td>
                <td>{token.expires_at && isValid(parseISO(token.expires_at)) ? format(parseISO(token.expires_at), 'PPp') : '—'}</td>
                <td>{token.last_used_at ? format(parseISO(token.last_used_at), 'PPp') : '—'}</td>
                <td>
                  <button
                    type="button"
                    className="button button--secondary"
                    onClick={() => {
                      if (window.confirm(expired ? t('settings.confirmDelete', { label }) : t('settings.confirmRevoke', { label }))) {
                        void removeToken(token.id, expired)
                      }
                    }}
                    disabled={syncing}
                  >
                    {expired ? t('common.delete') : t('settings.revoke')}
                  </button>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      {tokens.length === 0 && !loading ? <p className="hint">{t('settings.noTokens')}</p> : null}

      {/* Create dialog */}
      <Dialog.Root open={createOpen} onOpenChange={(open) => { if (!submitting) setCreateOpen(open) }}>
        <Dialog.Portal>
          <Dialog.Overlay className="dialog-overlay" />
          <Dialog.Content className="dialog-content" style={{ maxWidth: 560 }}>
            <div className="drawer-header">
              <Dialog.Title className="drawer-title">{t('settings.createTokenTitle')}</Dialog.Title>
              <Dialog.Close asChild>
                <button type="button" className="button button--secondary" aria-label={t('common.close')}><X size={18} /></button>
              </Dialog.Close>
            </div>
            <div className="drawer-body stack">
              <label className="field">
                <span>{t('settings.label')}</span>
                <input value={name} onChange={(e) => setName(e.target.value)} placeholder={t('settings.labelPlaceholder')} autoFocus />
              </label>
              <label className="field">
                <span>{t('settings.tokenDescription')}</span>
                <input value={description} onChange={(e) => setDescription(e.target.value)} placeholder={t('settings.tokenDescriptionPlaceholder')} />
              </label>
              <label className="field">
                <span>{t('settings.tokenTeam')}</span>
                <select value={teamId} onChange={(e) => setTeamId(e.target.value)}>
                  <option value="all">{t('settings.tokenTeamAll')}</option>
                  {teams.map((tm) => (
                    <option key={tm.id} value={tm.id}>{tm.name}</option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span>{t('settings.expiresUtc')}</span>
                <input type="date" value={expiresYmd} onChange={(e) => setExpiresYmd(e.target.value)} />
              </label>
              <div className="field">
                <span>{t('settings.tokenScopes')}</span>
                <p className="hint text-xs">{t('settings.tokenScopesHint')}</p>
                {SCOPE_GROUPS.map(({ group, scopes: groupScopes }) => (
                  <div key={group} className="token-scope-group">
                    <span className="token-scope-group__title">{t(`settings.scopeGroup_${group}`)}</span>
                    <div className="brand-option-grid">
                      {groupScopes.map((scope) => (
                        <label key={scope} className="token-scope-option">
                          <input type="checkbox" checked={scopes.includes(scope)} onChange={() => toggleScope(scope)} />
                          <span>{scope}</span>
                        </label>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
              {formError ? <p className="status-banner__error">{formError}</p> : null}
              <div className="inline-cluster">
                <button type="button" className="button button--primary" onClick={() => void handleCreate()} disabled={submitting || !name.trim()}>
                  {t('settings.createToken')}
                </button>
                <Dialog.Close asChild>
                  <button type="button" className="button button--secondary" disabled={submitting}>{t('common.cancel')}</button>
                </Dialog.Close>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>

      {/* Reveal dialog */}
      <Dialog.Root open={reveal !== null} onOpenChange={(open) => { if (!open) setReveal(null) }}>
        <Dialog.Portal>
          <Dialog.Overlay className="dialog-overlay" />
          <Dialog.Content className="dialog-content" style={{ maxWidth: 560 }}>
            <div className="drawer-header">
              <Dialog.Title className="drawer-title">{t('settings.tokenRevealTitle')}</Dialog.Title>
              <Dialog.Close asChild>
                <button type="button" className="button button--secondary" aria-label={t('common.close')}><X size={18} /></button>
              </Dialog.Close>
            </div>
            <div className="drawer-body stack">
              <p className="hint">{t('settings.tokenRevealHint')}</p>
              <button type="button" className="token-reveal__value" onClick={() => void copyToken()} title={t('settings.copyToken')}>
                <code>{reveal}</code>
              </button>
              <p className="hint text-xs">{copied ? t('settings.copied') : t('settings.copyToken')}</p>
              <div className="inline-cluster">
                <Dialog.Close asChild>
                  <button type="button" className="button button--primary">{t('common.close')}</button>
                </Dialog.Close>
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  )
}
