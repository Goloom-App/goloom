import type { Dispatch, SetStateAction } from 'react'
import { format, formatDistanceToNow, isValid, parseISO } from 'date-fns'
import { Icon } from '../../icons'
import { DestinationAvatar } from '../../components/post/DestinationAvatar'
import type { AccountRecord, ProviderInstanceRecord, ProviderName, TeamRecord } from '../../types'
import type { AccountConnectDraft } from './accountConnectTypes'
import { defaultAccountConnectDraft } from './accountConnectTypes'

function formatSyncAt(iso?: string): string {
  const raw = iso?.trim()
  if (!raw) {
    return 'Never'
  }
  const parsed = parseISO(raw)
  if (!isValid(parsed)) {
    return 'Never'
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
  return (
    <div className="accounts-view two-column-detail">
      <div className="glass-panel">
        <h2 className="section-card__title">Connected accounts</h2>
        <p className="hint">
          Social accounts are attached to the workspace selected in the header — including your personal workspace. Use them as post
          destinations in the composer.
        </p>
        {teamAccounts.length === 0 ? (
          <p className="hint">No accounts connected for this workspace yet.</p>
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
                      Engagement: {formatSyncAt(account.postEngagementSyncedAt)} · Followers: {formatSyncAt(account.accountMetricsSyncedAt)}
                    </div>
                  </div>
                </div>
                {canEditTeamAccounts ? (
                  <button type="button" className="button button--secondary" onClick={() => void onDeleteTeamAccount(account.id)} disabled={syncing}>
                    <Icon name="trash" className="inline-icon" />
                    <span>Remove</span>
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
        {!canEditTeamAccounts && selectedTeam ? <p className="hint">View-only members cannot connect or remove accounts.</p> : null}
      </div>

      <div className="glass-panel">
        <h2 className="section-card__title">Connect an account</h2>
        {!selectedTeam ? (
          <p className="hint">Select or create a team first.</p>
        ) : !canEditTeamAccounts ? (
          <p className="hint">You need editor or owner access on this workspace to connect accounts.</p>
        ) : (
          <>
            <label className="field">
              <span>Provider</span>
              <select
                value={accountDraft.provider}
                onChange={(event) => {
                  const p = event.target.value as ProviderName
                  setAccountDraft({ ...defaultAccountConnectDraft(), provider: p })
                }}
              >
                <option value="mastodon">Mastodon</option>
                <option value="friendica">Friendica</option>
                <option value="bluesky">Bluesky</option>
              </select>
            </label>

            {instancesForAccountConnect.length > 0 ? (
              <label className="field">
                <span>Registered instance</span>
                <select
                  value={accountDraft.providerInstanceId}
                  onChange={(event) => setAccountDraft((c) => ({ ...c, providerInstanceId: event.target.value }))}
                >
                  <option value="">— Custom URL (below) —</option>
                  {instancesForAccountConnect.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name} ({p.instanceUrl})
                    </option>
                  ))}
                </select>
              </label>
            ) : (
              <p className="hint">
                No {accountDraft.provider} instance is registered yet. Ask an administrator to add one under Admin, or use the instance URL
                field when the provider allows it.
              </p>
            )}

            {!accountDraft.providerInstanceId.trim() ? (
              <label className="field">
                <span>{accountDraft.provider === 'bluesky' ? 'PDS URL (optional)' : 'Instance base URL'}</span>
                <input
                  value={accountDraft.instanceUrl}
                  onChange={(event) => setAccountDraft((c) => ({ ...c, instanceUrl: event.target.value }))}
                  placeholder={accountDraft.provider === 'bluesky' ? 'https://bsky.social' : 'https://social.example'}
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
                    Authorize in browser
                  </button>
                </div>
                <p className="hint">Browser login requires a registered Mastodon instance with OAuth. Or paste an access token below.</p>
                <label className="field">
                  <span>Access token (manual)</span>
                  <input
                    type="password"
                    autoComplete="off"
                    value={accountDraft.accessToken}
                    onChange={(event) => setAccountDraft((c) => ({ ...c, accessToken: event.target.value }))}
                    placeholder="OAuth access token"
                  />
                </label>
                <label className="field">
                  <span>Refresh token (optional)</span>
                  <input
                    type="password"
                    autoComplete="off"
                    value={accountDraft.refreshToken}
                    onChange={(event) => setAccountDraft((c) => ({ ...c, refreshToken: event.target.value }))}
                  />
                </label>
                <button type="button" className="button button--secondary" onClick={() => void onConnectSocialAccount()} disabled={syncing}>
                  Connect with token
                </button>
              </>
            ) : null}

            {accountDraft.provider === 'friendica' ? (
              <>
                <label className="field">
                  <span>Username</span>
                  <input
                    value={accountDraft.identifier}
                    onChange={(event) => setAccountDraft((c) => ({ ...c, identifier: event.target.value }))}
                    placeholder="Local username"
                  />
                </label>
                <label className="field">
                  <span>Access token</span>
                  <input
                    type="password"
                    autoComplete="off"
                    value={accountDraft.accessToken}
                    onChange={(event) => setAccountDraft((c) => ({ ...c, accessToken: event.target.value }))}
                  />
                </label>
                <button type="button" className="button button--primary" onClick={() => void onConnectSocialAccount()} disabled={syncing}>
                  Connect Friendica
                </button>
              </>
            ) : null}

            {accountDraft.provider === 'bluesky' ? (
              <>
                <label className="field">
                  <span>Sign-in method</span>
                  <select
                    value={accountDraft.blueskyAuthMode}
                    onChange={(event) =>
                      setAccountDraft((c) => ({ ...c, blueskyAuthMode: event.target.value as 'app_password' | 'access_token' }))
                    }
                  >
                    <option value="app_password">App password</option>
                    <option value="access_token">Access token (JWT)</option>
                  </select>
                </label>
                {accountDraft.blueskyAuthMode === 'app_password' ? (
                  <>
                    <label className="field">
                      <span>Handle</span>
                      <input
                        value={accountDraft.identifier}
                        onChange={(event) => setAccountDraft((c) => ({ ...c, identifier: event.target.value }))}
                        placeholder="you.bsky.social"
                      />
                    </label>
                    <label className="field">
                      <span>App password</span>
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
                      <span>Access token</span>
                      <input
                        type="password"
                        autoComplete="off"
                        value={accountDraft.accessToken}
                        onChange={(event) => setAccountDraft((c) => ({ ...c, accessToken: event.target.value }))}
                      />
                    </label>
                    <label className="field">
                      <span>Refresh token (optional)</span>
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
                  Connect Bluesky
                </button>
              </>
            ) : null}
          </>
        )}
      </div>
    </div>
  )
}
