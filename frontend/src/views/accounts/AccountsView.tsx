import type { Dispatch, SetStateAction } from 'react'
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
}) {
  const { t } = useTranslation()
  const neverLabel = t('common.never')

  return (
    <div className="accounts-view two-column-detail">
      <div className="glass-panel">
        <h2 className="section-card__title">{t('accounts.connectedTitle')}</h2>
        <p className="hint">{t('accounts.connectedHint')}</p>
        {teamAccounts.length === 0 ? (
          <p className="hint">{t('accounts.noAccounts')}</p>
        ) : (
          <ul className="account-connect-list">
            {teamAccounts.map((account) => (
              <li key={account.id} className="account-connect-list__row">
                <div className="inline-cluster">
                  <DestinationAvatar account={account} />
                  <div>
                    <strong>{account.name}</strong>
                    <div className="hint">
                      {account.provider} · @{account.username} · {account.instance}
                    </div>
                    <div className="hint account-sync-hint">
                      {t('accounts.engagementSync', {
                        engagement: formatSyncAt(account.postEngagementSyncedAt, neverLabel),
                        followers: formatSyncAt(account.accountMetricsSyncedAt, neverLabel),
                      })}
                    </div>
                  </div>
                </div>
                {canEditTeamAccounts ? (
                  <button type="button" className="button button--secondary" onClick={() => void onDeleteTeamAccount(account.id)} disabled={syncing}>
                    <Icon name="trash" className="inline-icon" />
                    <span>{t('common.remove')}</span>
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
        {!canEditTeamAccounts && selectedTeam ? <p className="hint">{t('accounts.viewOnlyHint')}</p> : null}
      </div>

      <div className="glass-panel">
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

            {accountDraft.provider === 'mastodon' ? (
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
      </div>
    </div>
  )
}
