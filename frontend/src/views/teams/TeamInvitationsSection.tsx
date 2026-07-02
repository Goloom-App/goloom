import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { MailPlus } from 'lucide-react'

import { SectionCard } from '../../components/ui'
import type { BackendTeamInvitation, createApiClient } from '../../api'
import { translateApiError } from '../../i18n/translateApiError'

function buildInviteLink(origin: string, pathname: string, token: string): string {
  const basePath = pathname.endsWith('/') ? pathname : `${pathname}/`
  return `${origin}${basePath}?invite=${encodeURIComponent(token)}`
}

export function TeamInvitationsSection({
  api,
  teamId,
}: {
  api: ReturnType<typeof createApiClient> | null
  teamId: string
}) {
  const { t } = useTranslation()
  const [pending, setPending] = useState<BackendTeamInvitation[]>([])
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<'editor' | 'viewer'>('editor')
  const [inviteLink, setInviteLink] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!api) return
    try {
      const res = await api.listTeamInvitations(teamId)
      setPending(res.items ?? [])
    } catch {
      setPending([])
    }
  }, [api, teamId])

  useEffect(() => {
    setInviteLink(null)
    setCopied(false)
    setError(null)
    void load()
  }, [load])

  async function createInvitation() {
    if (!api || !email.trim()) return
    setSubmitting(true)
    setError(null)
    setCopied(false)
    try {
      const res = await api.createTeamInvitation(teamId, { email: email.trim(), role })
      setInviteLink(buildInviteLink(window.location.origin, window.location.pathname, res.token))
      setEmail('')
      await load()
    } catch (cause) {
      setError(translateApiError(cause instanceof Error ? cause.message : String(cause), t))
    } finally {
      setSubmitting(false)
    }
  }

  async function revoke(invitationId: string) {
    if (!api) return
    setError(null)
    try {
      await api.deleteTeamInvitation(teamId, invitationId)
      await load()
    } catch (cause) {
      setError(translateApiError(cause instanceof Error ? cause.message : String(cause), t))
    }
  }

  async function copyLink() {
    if (!inviteLink) return
    try {
      await navigator.clipboard.writeText(inviteLink)
      setCopied(true)
    } catch {
      /* clipboard unavailable (e.g. insecure context); the link stays selectable */
    }
  }

  return (
    <SectionCard icon={<MailPlus size={18} />} title={t('teams.inviteTitle')} subtitle={t('teams.inviteHint')}>
      <div className="inline-cluster flex-wrap">
        <label className="field grow">
          <span>{t('teams.inviteEmailLabel')}</span>
          <input
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            placeholder="name@example.com"
            data-testid="invite-email"
          />
        </label>
        <label className="field">
          <span>{t('teams.inviteRoleLabel')}</span>
          <select value={role} onChange={(event) => setRole(event.target.value as 'editor' | 'viewer')} data-testid="invite-role">
            <option value="editor">{t('roles.editor')}</option>
            <option value="viewer">{t('roles.viewer')}</option>
          </select>
        </label>
        <button
          type="button"
          className="btn btn--primary"
          onClick={() => void createInvitation()}
          disabled={submitting || !email.trim()}
          data-testid="invite-submit"
        >
          {t('teams.inviteSubmit')}
        </button>
      </div>

      {error ? (
        <p role="alert" style={{ color: 'var(--color-danger, #dc2626)', fontSize: 13 }}>
          {error}
        </p>
      ) : null}

      {inviteLink ? (
        <div className="glass-panel glass-panel--compact mt-1" data-testid="invite-link-panel">
          <strong>{t('teams.inviteLinkLabel')}</strong>
          <p className="hint">{t('teams.inviteLinkNote')}</p>
          <div className="inline-cluster flex-wrap">
            <code className="grow" style={{ wordBreak: 'break-all', userSelect: 'all' }} data-testid="invite-link">
              {inviteLink}
            </code>
            <button type="button" className="btn btn--secondary btn--sm" onClick={() => void copyLink()}>
              {copied ? t('teams.inviteCopied') : t('teams.inviteCopy')}
            </button>
          </div>
        </div>
      ) : null}

      <div className="mt-1">
        <h4 className="subsection-title">{t('teams.invitePendingTitle')}</h4>
        {pending.length === 0 ? (
          <p className="hint">{t('teams.invitePendingEmpty')}</p>
        ) : (
          <div className="stack stack--sm">
            {pending.map((inv) => (
              <div key={inv.id} className="glass-panel glass-panel--compact flex-row--between" data-testid="invite-pending-row">
                <div>
                  <strong>{inv.email}</strong>
                  <p className="eyebrow">
                    {t(`roles.${inv.role}`)} · {t('teams.inviteExpires', { date: new Date(inv.expires_at).toLocaleDateString() })}
                  </p>
                </div>
                <button
                  type="button"
                  className="btn btn--xs btn--danger-ghost"
                  onClick={() => void revoke(inv.id)}
                  data-testid="invite-revoke"
                >
                  {t('teams.inviteRevoke')}
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </SectionCard>
  )
}
