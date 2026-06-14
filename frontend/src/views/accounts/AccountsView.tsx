import { useState, type Dispatch, type SetStateAction } from 'react'
import { format, formatDistanceToNow, isValid, parseISO } from 'date-fns'
import { useTranslation } from 'react-i18next'

import { Icon } from '../../icons'
import { DestinationAvatar } from '../../components/post/DestinationAvatar'
import type { AccountRecord, ProviderInstanceRecord, ProviderName, TeamRecord } from '../../types'
import type { AccountConnectDraft } from './accountConnectTypes'
import { defaultAccountConnectDraft } from './accountConnectTypes'

function formatSyncAt(iso: string | undefined, neverLabel: string): string {
  const raw = iso?.trim()
  if (!raw) {
    return neverLabel
  }
  const parsed = parseISO(raw)
  if (!isValid(parsed)) {
    return neverLabel
  }
  return `${format(parsed, 'PPp')} (${formatDistanceToNow(parsed, { addSuffix: true })})`
}

export function AccountsView({
  selectedTeam,
  teamAccounts,
  canEditTeamAccounts,
  syncing,
  accountDraft,
  setAccountDraft,
  instancesForAccountConnect,
  onDeleteTeamAccount,
  onConnectSocialAccount,
  onMastodonOAuthConnect,
  onUpdateTeamAccount,
  editingAccountId,
  setEditingAccountId,
}: {
  selectedTeam: TeamRecord | null
  teamAccounts: AccountRecord[]
  canEditTeamAccounts: boolean
  syncing: boolean
  accountDraft: AccountConnectDraft
  setAccountDraft: Dispatch<SetStateAction<AccountConnectDraft>>
  instancesForAccountConnect: ProviderInstanceRecord[]
  onDeleteTeamAccount: (accountId: string) => void | Promise<void>
  onConnectSocialAccount: () => void | Promise<void>
  onMastodonOAuthConnect: () => void | Promise<void>
  onUpdateTeamAccount: (accountId: string, payload: {
    name?: string
    max_chars_override?: number
    access_token?: string
    refresh_token?: string
  }) => void | Promise<void>
  editingAccountId: string | null
  setEditingAccountId: (id: string | null) => void
}) {
  const { t } = useTranslation()
  const neverLabel = t('common.never')

  const editingAccount = editingAccountId ? teamAccounts.find((a) => a.id === editingAccountId) ?? null : null

  const [editName, setEditName] = useState('')
  const [editMaxChars, setEditMaxChars] = useState('')
  const [editAccessToken, setEditAccessToken] = useState('')
  const [editRefreshToken, setEditRefreshToken] = useState('')

  const initEditForm = (account: AccountRecord) => {
    setEditName(account.name ?? '')
    setEditMaxChars(account.maxChars?.toString() ?? '')
    setEditAccessToken('')
    setEditRefreshToken('')
  }

  const cancelEdit = () => {
    setEditingAccountId(null)
    setEditName('')
    setEditMaxChars('')
    setEditAccessToken('')
    setEditRefreshToken('')
  }

  const saveEdit = () => {
    if (!editingAccountId) return
    const payload: {
      name?: string
      max_chars_override?: number
      access_token?: string
      refresh_token?: string
    } = {}
    if (editName.trim() && editName.trim() !== editingAccount?.name) {
      payload.name = editName.trim()
    }
    const parsedMax = parseInt(editMaxChars, 10)
    if (!Number.isNaN(parsedMax) && parsedMax > 0 && parsedMax !== editingAccount?.maxChars) {
      payload.max_chars_override = parsedMax
    }
    if (editAccessToken.trim()) {
      payload.access_token = editAccessToken.trim()
    }
    if (editRefreshToken.trim()) {
      payload.refresh_token = editRefreshToken.trim()
    }
    void onUpdateTeamAccount(editingAccountId, payload)
    cancelEdit()
  }

  const editingAccountForm = editingAccount && (
    <div className="edit-account-form stack stack--sm">
      <div className="inline-cluster" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
        <h3 className="subsection-title" style={{ margin: 0 }}>{t('accounts.editAccount')}</h3>
        <span className="admin-pill admin-pill--accent">{t('accounts.editing')}</span>
      </div>
      <p className="hint">{t('accounts.editAccountHint')}</p>

      <label className="field">
        <span>{t('accounts.displayName')}</span>
        <input value={editName} onChange={(e) => setEditName(e.target.value)} placeholder={editingAccount.name} />
      </label>

      <label className="field">
        <span>{t('accounts.maxCharsOverride')}</span>
        <input type="number" value={editMaxChars} onChange={(e) => setEditMaxChars(e.target.value)} placeholder={editingAccount.maxChars.toString()} />
      </label>

      <div className="field">
        <span className="hint">{t('accounts.accountName')}: {editingAccount.name}</span>
        <span className="hint" style={{ display: 'block' }}>@{editingAccount.username} · {editingAccount.instance}</span>
      </div>

      <details className="advanced-config" open={false}>
        <summary className="advanced-config__summary">{t('admin.advancedConfig')}</summary>
        <p className="hint mt-1">{t('accounts.rotateCredentialsHint')}</p>
        <div className="stack stack--sm" style={{ marginTop: '0.5rem' }}>
          <label className="field">
            <span>{t('accounts.accessTokenManual')}</span>
            <input
              type="password"
              autoComplete="off"
              value={editAccessToken}
              onChange={(e) => setEditAccessToken(e.target.value)}
              placeholder={t('accounts.placeholderOAuthToken')}
            />
          </label>
          <label className="field">
            <span>{t('accounts.refreshTokenOptional')}</span>
            <input
              type="password"
              autoComplete="off"
              value={editRefreshToken}
              onChange={(e) => setEditRefreshToken(e.target.value)}
            />
          </label>
        </div>
      </details>

      <div className="inline-cluster">
        <button type="button" className="button button--primary" onClick={saveEdit} disabled={syncing}>
          {t('accounts.updateAccount')}
        </button>
        <button type="button" className="button button--secondary" onClick={cancelEdit}>
          {t('common.cancel')}
        </button>
      </div>
    </div>
  )

  return (
    <div className="accounts-view two-column-detail">
      <div className="glass-panel">
        <h2 className="section-card__title">{t('accounts.connectedTitle')}</h2>
        <p className="hint">{t('accounts.connectedHint')}</p>
        {teamAccounts.length === 0 ? (
          <p className="hint">{t('accounts.noAccounts')}</p>
        ) : (
          <ul className="account-cards">
            {teamAccounts.map((account) => {
              const isEditing = editingAccountId === account.id
              return (
                <li key={account.id} className={`account-card ${isEditing ? 'account-card--editing' : ''}`}>
                  <div className="account-card__main">
                    <div className="inline-cluster" style={{ alignItems: 'center' }}>
                      <DestinationAvatar account={account} />
                      <div>
                        <div className="account-card__title-row">
                          <strong>{account.name}</strong>
                          <span className="account-card__provider">{account.provider}</span>
                          {isEditing ? <span className="admin-pill admin-pill--accent">{t('accounts.editing')}</span> : null}
                        </div>
                        <div className="hint">
                          @{account.username} · {account.instance}
                        </div>
                      </div>
                    </div>
                    <p className="hint account-card__meta">
                      {t('accounts.engagementSync', {
                        engagement: formatSyncAt(account.postEngagementSyncedAt, neverLabel),
                        followers: formatSyncAt(account.accountMetricsSyncedAt, neverLabel),
                      })}
                    </p>
                  </div>
                  {canEditTeamAccounts ? (
                    <div className="account-card__actions">
                      <button
                        type="button"
                        className="button button--secondary"
                        onClick={() => {
                          setEditingAccountId(account.id)
                          initEditForm(account)
                        }}
                        disabled={syncing}
                      >
                        <Icon name="edit" className="inline-icon" />
                        <span>{t('common.edit')}</span>
                      </button>
                      <button
                        type="button"
                        className="button button--secondary"
                        onClick={() => void onDeleteTeamAccount(account.id)}
                        disabled={syncing}
                      >
                        <Icon name="trash" className="inline-icon" />
                        <span>{t('common.remove')}</span>
                      </button>
                    </div>
                  ) : null}
                </li>
              )
            })}
          </ul>
        )}
        {!canEditTeamAccounts && selectedTeam ? <p className="hint">{t('accounts.viewOnlyHint')}</p> : null}
      </div>

      <div className="glass-panel">
        {editingAccountForm ? (
          editingAccountForm
        ) : (
          <>
            <h2 className="section-card__title">{t('accounts.connectTitle')}</h2>
            {!selectedTeam ? (
              <p className="hint">{t('accounts.selectTeamFirst')}</p>
            ) : !canEditTeamAccounts ? (
              <p className="hint">{t('accounts.needEditorAccess')}</p>
            ) : (
              <>
                <label className="field">
                  <span>{t('common.provider')}</span>
                  <select
                    value={accountDraft.provider}
                    onChange={(event) => {
                      const p = event.target.value as ProviderName
                      setAccountDraft({ ...defaultAccountConnectDraft(), provider: p })
                    }}
                  >
                    <option value="mastodon">{t('accounts.providerMastodon')}</option>
                    <option value="pixelfed">{t('accounts.providerPixelfed')}</option>
                    <option value="friendica">{t('accounts.providerFriendica')}</option>
                    <option value="bluesky">{t('accounts.providerBluesky')}</option>
                  </select>
                </label>

                {instancesForAccountConnect.length > 0 ? (
                  <label className="field">
                    <span>{t('accounts.registeredInstance')}</span>
                    <select
                      value={accountDraft.providerInstanceId}
                      onChange={(event) => setAccountDraft((c) => ({ ...c, providerInstanceId: event.target.value }))}
                    >
                      <option value="">{t('accounts.customUrlOption')}</option>
                      {instancesForAccountConnect.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name} ({p.instanceUrl})
                        </option>
                      ))}
                    </select>
                  </label>
                ) : (
                  <p className="hint">{t('accounts.noInstanceRegistered', { provider: accountDraft.provider })}</p>
                )}

                {!accountDraft.providerInstanceId.trim() ? (
                  <label className="field">
                    <span>{accountDraft.provider === 'bluesky' ? t('accounts.pdsUrlOptional') : t('accounts.instanceBaseUrl')}</span>
                    <input
                      value={accountDraft.instanceUrl}
                      onChange={(event) => setAccountDraft((c) => ({ ...c, instanceUrl: event.target.value }))}
                      placeholder={accountDraft.provider === 'bluesky' ? t('accounts.placeholderBsky') : t('accounts.placeholderInstance')}
                    />
                  </label>
                ) : null}

                {accountDraft.provider === 'mastodon' || accountDraft.provider === 'pixelfed' ? (
                  <>
                    <div className="flex-row--wrap" style={{ marginTop: '0.5rem' }}>
                      <button
                        type="button"
                        className="button button--primary"
                        onClick={() => void onMastodonOAuthConnect()}
                        disabled={syncing || !accountDraft.providerInstanceId.trim()}
                      >
                        {t('accounts.authorizeInBrowser')}
                      </button>
                    </div>
                    <p className="hint">{t('accounts.mastodonOAuthHint')}</p>
                    {accountDraft.provider === 'pixelfed' ? <p className="hint">{t('accounts.pixelfedMediaHint')}</p> : null}
                    <label className="field">
                      <span>{t('accounts.accessTokenManual')}</span>
                      <input
                        type="password"
                        autoComplete="off"
                        value={accountDraft.accessToken}
                        onChange={(event) => setAccountDraft((c) => ({ ...c, accessToken: event.target.value }))}
                        placeholder={t('accounts.placeholderOAuthToken')}
                      />
                    </label>
                    <label className="field">
                      <span>{t('accounts.refreshTokenOptional')}</span>
                      <input
                        type="password"
                        autoComplete="off"
                        value={accountDraft.refreshToken}
                        onChange={(event) => setAccountDraft((c) => ({ ...c, refreshToken: event.target.value }))}
                      />
                    </label>
                    <button type="button" className="button button--secondary" onClick={() => void onConnectSocialAccount()} disabled={syncing}>
                      {t('accounts.connectWithToken')}
                    </button>
                  </>
                ) : null}

                {accountDraft.provider === 'friendica' ? (
                  <>
                    <label className="field">
                      <span>{t('accounts.username')}</span>
                      <input
                        value={accountDraft.identifier}
                        onChange={(event) => setAccountDraft((c) => ({ ...c, identifier: event.target.value }))}
                        placeholder={t('accounts.placeholderLocalUsername')}
                      />
                    </label>
                    <label className="field">
                      <span>{t('accounts.accessToken')}</span>
                      <input
                        type="password"
                        autoComplete="off"
                        value={accountDraft.accessToken}
                        onChange={(event) => setAccountDraft((c) => ({ ...c, accessToken: event.target.value }))}
                      />
                    </label>
                    <button type="button" className="button button--primary" onClick={() => void onConnectSocialAccount()} disabled={syncing}>
                      {t('accounts.connectFriendica')}
                    </button>
                  </>
                ) : null}

                {accountDraft.provider === 'bluesky' ? (
                  <>
                    <label className="field">
                      <span>{t('accounts.signInMethod')}</span>
                      <select
                        value={accountDraft.blueskyAuthMode}
                        onChange={(event) =>
                          setAccountDraft((c) => ({ ...c, blueskyAuthMode: event.target.value as 'app_password' | 'access_token' }))
                        }
                      >
                        <option value="app_password">{t('accounts.optionAppPassword')}</option>
                        <option value="access_token">{t('accounts.optionAccessToken')}</option>
                      </select>
                    </label>
                    {accountDraft.blueskyAuthMode === 'app_password' ? (
                      <>
                        <label className="field">
                          <span>{t('accounts.handle')}</span>
                          <input
                            value={accountDraft.identifier}
                            onChange={(event) => setAccountDraft((c) => ({ ...c, identifier: event.target.value }))}
                            placeholder={t('accounts.placeholderHandle')}
                          />
                        </label>
                        <label className="field">
                          <span>{t('accounts.appPassword')}</span>
                          <input
                            type="password"
                            autoComplete="off"
                            value={accountDraft.appPassword}
                            onChange={(event) => setAccountDraft((c) => ({ ...c, appPassword: event.target.value }))}
                          />
                        </label>
                      </>
                    ) : (
                      <>
                        <label className="field">
                          <span>{t('accounts.accessToken')}</span>
                          <input
                            type="password"
                            autoComplete="off"
                            value={accountDraft.accessToken}
                            onChange={(event) => setAccountDraft((c) => ({ ...c, accessToken: event.target.value }))}
                          />
                        </label>
                        <label className="field">
                          <span>{t('accounts.refreshTokenOptional')}</span>
                          <input
                            type="password"
                            autoComplete="off"
                            value={accountDraft.refreshToken}
                            onChange={(event) => setAccountDraft((c) => ({ ...c, refreshToken: event.target.value }))}
                          />
                        </label>
                      </>
                    )}
                    <button type="button" className="button button--primary" onClick={() => void onConnectSocialAccount()} disabled={syncing}>
                      {t('accounts.connectBluesky')}
                    </button>
                  </>
                ) : null}
              </>
            )}
          </>
        )}
      </div>
    </div>
  )
}
