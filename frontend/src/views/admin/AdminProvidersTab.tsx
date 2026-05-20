import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'

import { Icon } from '../../icons'
import type { AccountRecord, ProviderInstanceRecord, ProviderName } from '../../types'
import type { AdminProviderDraft } from './adminTypes'
import { defaultAdminProviderDraft } from './adminTypes'

export function AdminProvidersTab({
  providerInstances,
  accounts,
  adminProviderDraft,
  setAdminProviderDraft,
  editingProviderId,
  setEditingProviderId,
  showAdminProviderAdvanced,
  setShowAdminProviderAdvanced,
  syncing,
  onSaveAdminProvider,
  onDeleteProviderInstance,
}: {
  providerInstances: ProviderInstanceRecord[]
  accounts: AccountRecord[]
  adminProviderDraft: AdminProviderDraft
  setAdminProviderDraft: Dispatch<SetStateAction<AdminProviderDraft>>
  editingProviderId: string | null
  setEditingProviderId: (id: string | null) => void
  showAdminProviderAdvanced: boolean
  setShowAdminProviderAdvanced: (open: boolean) => void
  syncing: boolean
  onSaveAdminProvider: () => void | Promise<void>
  onDeleteProviderInstance: (instanceId: string) => void | Promise<void>
}) {
  const { t } = useTranslation()

  return (
    <div className="admin-tab-panel stack stack--lg">
      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <div>
            <h2 className="admin-section__title">{t('admin.providerOnboarding')}</h2>
            <p className="hint admin-section__hint">{t('admin.providerOnboardingHint')}</p>
          </div>
          {editingProviderId ? (
            <span className="admin-pill admin-pill--accent">{t('admin.editingInstance')}</span>
          ) : null}
        </header>

        <div className="admin-provider-form">
          <div className="admin-provider-form__row">
            <label className="field admin-provider-form__provider">
              <span>{t('common.provider')}</span>
              <select
                value={adminProviderDraft.provider}
                onChange={(event) => {
                  const p = event.target.value as ProviderName
                  setAdminProviderDraft((current) => ({
                    ...current,
                    provider: p,
                    instanceUrl: p === 'bluesky' ? 'https://bsky.social' : current.instanceUrl,
                  }))
                }}
              >
                <option value="mastodon">{t('accounts.providerMastodon')}</option>
                <option value="friendica">{t('accounts.providerFriendica')}</option>
                <option value="bluesky">{t('accounts.providerBluesky')}</option>
              </select>
            </label>
            {editingProviderId ? (
              <button
                type="button"
                className="button button--secondary"
                onClick={() => {
                  setEditingProviderId(null)
                  setAdminProviderDraft(defaultAdminProviderDraft())
                  setShowAdminProviderAdvanced(false)
                }}
              >
                {t('admin.cancelEdit')}
              </button>
            ) : null}
          </div>

          <div className="admin-provider-form__grid">
            <label className="field">
              <span>{t('admin.displayName')}</span>
              <input
                value={adminProviderDraft.name}
                onChange={(event) => setAdminProviderDraft((c) => ({ ...c, name: event.target.value }))}
                placeholder={t('admin.placeholderDisplayName')}
              />
            </label>
            <label className="field">
              <span>{t('admin.instanceUrl')}</span>
              <input
                value={adminProviderDraft.instanceUrl}
                onChange={(event) => setAdminProviderDraft((c) => ({ ...c, instanceUrl: event.target.value }))}
                placeholder={t('admin.placeholderMastodon')}
              />
            </label>
          </div>

          <details
            className="advanced-config admin-provider-form__advanced"
            open={showAdminProviderAdvanced}
            onToggle={(event) => setShowAdminProviderAdvanced(event.currentTarget.open)}
          >
            <summary className="advanced-config__summary">{t('admin.advancedConfig')}</summary>
            <p className="hint mt-1">{t('admin.advancedConfigHint')}</p>
            <div className="admin-provider-form__grid">
              <label className="field">
                <span>{t('admin.clientId')}</span>
                <input
                  value={adminProviderDraft.clientId}
                  onChange={(event) => setAdminProviderDraft((c) => ({ ...c, clientId: event.target.value }))}
                  placeholder={t('admin.placeholderClientId')}
                />
              </label>
              <label className="field">
                <span>{t('admin.clientSecret')}</span>
                <input
                  type="password"
                  value={adminProviderDraft.clientSecret}
                  onChange={(event) => setAdminProviderDraft((c) => ({ ...c, clientSecret: event.target.value }))}
                  placeholder={t('admin.placeholderKeepSecret')}
                />
              </label>
              <label className="field admin-provider-form__full">
                <span>{t('admin.scopes')}</span>
                <input value={adminProviderDraft.scopes} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, scopes: event.target.value }))} />
              </label>
              <label className="field admin-provider-form__full">
                <span>{t('admin.authorizationEndpoint')}</span>
                <input
                  value={adminProviderDraft.authorizationEndpoint}
                  onChange={(event) => setAdminProviderDraft((c) => ({ ...c, authorizationEndpoint: event.target.value }))}
                />
              </label>
              <label className="field admin-provider-form__full">
                <span>{t('admin.tokenEndpoint')}</span>
                <input
                  value={adminProviderDraft.tokenEndpoint}
                  onChange={(event) => setAdminProviderDraft((c) => ({ ...c, tokenEndpoint: event.target.value }))}
                />
              </label>
            </div>
          </details>

          <button type="button" className="button button--primary" onClick={() => void onSaveAdminProvider()} disabled={syncing}>
            {editingProviderId ? t('admin.updateProvider') : t('admin.registerProvider')}
          </button>
        </div>
      </section>

      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <h2 className="admin-section__title">{t('admin.registeredInstances')}</h2>
          <span className="admin-pill admin-pill--muted">{t('admin.instanceCount', { count: providerInstances.length })}</span>
        </header>

        {providerInstances.length > 0 ? (
          <ul className="admin-provider-cards">
            {providerInstances.map((p) => {
              const onboarded = accounts.filter((a) => a.providerInstanceId === p.id).length
              const isEditing = editingProviderId === p.id
              return (
                <li key={p.id} className={`admin-provider-card ${isEditing ? 'admin-provider-card--active' : ''}`}>
                  <div className="admin-provider-card__main">
                    <div className="admin-provider-card__title-row">
                      <strong>{p.name}</strong>
                      <span className="admin-provider-card__provider">{p.provider}</span>
                    </div>
                    <code className="inline-code admin-provider-card__url">{p.instanceUrl}</code>
                    <p className="hint admin-provider-card__meta">
                      {onboarded === 1
                        ? t('admin.accountsOnboarded', { count: onboarded })
                        : t('admin.accountsOnboarded_plural', { count: onboarded })}
                    </p>
                  </div>
                  <div className="admin-provider-card__actions">
                    <button
                      type="button"
                      className="button button--secondary"
                      onClick={() => {
                        setEditingProviderId(p.id)
                        setShowAdminProviderAdvanced(true)
                        setAdminProviderDraft({
                          provider: p.provider,
                          name: p.name,
                          instanceUrl: p.instanceUrl,
                          clientId: p.clientId,
                          clientSecret: '',
                          scopes: p.scopes.join(','),
                          authorizationEndpoint: p.authorizationEndpoint,
                          tokenEndpoint: p.tokenEndpoint,
                        })
                      }}
                    >
                      {t('common.edit')}
                    </button>
                    <button
                      type="button"
                      className="button button--secondary"
                      onClick={() => {
                        if (window.confirm(t('admin.confirmRemoveInstance', { name: p.name }))) {
                          void onDeleteProviderInstance(p.id)
                        }
                      }}
                      disabled={syncing || onboarded > 0}
                      title={onboarded > 0 ? t('admin.disconnectBeforeRemove') : t('admin.removeProviderInstance')}
                    >
                      <Icon name="trash" className="inline-icon" />
                      <span>{t('common.remove')}</span>
                    </button>
                  </div>
                </li>
              )
            })}
          </ul>
        ) : (
          <p className="hint">{t('admin.noProviderInstances')}</p>
        )}
      </section>
    </div>
  )
}
