import { useTranslation } from 'react-i18next'

import type { RuntimeConfigRecord } from '../../types'

function ConfigPill({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation()
  return (
    <span className={`admin-pill ${enabled ? 'admin-pill--ok' : 'admin-pill--muted'}`}>
      <span className={`status-dot ${enabled ? 'status-dot--active' : 'status-dot--neutral'}`} aria-hidden />
      {enabled ? t('admin.stateEnabled') : t('admin.stateDisabled')}
    </span>
  )
}

export function AdminConfigurationsTab({ adminRuntime }: { adminRuntime: RuntimeConfigRecord | null }) {
  const { t } = useTranslation()

  if (!adminRuntime) {
    return (
      <div className="admin-tab-panel">
        <p className="hint">{t('admin.noRuntimeConfig')}</p>
      </div>
    )
  }

  return (
    <div className="admin-tab-panel stack stack--lg">
      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <div>
            <h2 className="admin-section__title">{t('admin.tabConfigurations')}</h2>
            <p className="hint admin-section__hint">{t('admin.configurationsHint')}</p>
          </div>
        </header>
      </section>

      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <h2 className="admin-section__title">{t('admin.securitySection')}</h2>
          <ConfigPill enabled={adminRuntime.security.encryptionConfigured} />
        </header>
        <div className="admin-kv-grid">
          <div className="admin-kv-card">
            <span className="admin-kv-card__label">{t('admin.encryption')}</span>
            <span className="admin-kv-card__value">
              {adminRuntime.security.encryptionConfigured ? t('admin.encryptionConfigured') : t('admin.encryptionMissing')}
            </span>
          </div>
          <div className="admin-kv-card">
            <span className="admin-kv-card__label">{t('admin.rateLimitAnonymous')}</span>
            <span className="admin-kv-card__value">{adminRuntime.security.rateLimitPerMinute}</span>
          </div>
          <div className="admin-kv-card">
            <span className="admin-kv-card__label">{t('admin.rateLimitBearer')}</span>
            <span className="admin-kv-card__value">{adminRuntime.security.rateLimitAuthenticatedPerMinute}</span>
          </div>
        </div>
        <div className="admin-config-block">
          <p className="eyebrow">{t('admin.allowedOrigins')}</p>
          {adminRuntime.security.allowedOrigins.length > 0 ? (
            <ul className="admin-tag-list">
              {adminRuntime.security.allowedOrigins.map((origin) => (
                <li key={origin}>
                  <code className="inline-code">{origin}</code>
                </li>
              ))}
            </ul>
          ) : (
            <p className="hint">{t('admin.noAllowedOrigins')}</p>
          )}
        </div>
      </section>

      <section className="admin-section glass-panel">
        <header className="admin-section__head">
          <h2 className="admin-section__title">{t('admin.oidcSection')}</h2>
          <ConfigPill enabled={adminRuntime.oidc.enabled} />
        </header>
        <div className="admin-kv-grid">
          <div className="admin-kv-card admin-kv-card--wide">
            <span className="admin-kv-card__label">{t('admin.oidcIssuer')}</span>
            <span className="admin-kv-card__value admin-kv-card__value--truncate">
              {adminRuntime.oidc.issuerUrl || t('common.emDash')}
            </span>
          </div>
          <div className="admin-kv-card admin-kv-card--wide">
            <span className="admin-kv-card__label">{t('admin.oidcClientId')}</span>
            <span className="admin-kv-card__value admin-kv-card__value--truncate">
              {adminRuntime.oidc.clientId || t('common.emDash')}
            </span>
          </div>
          <div className="admin-kv-card">
            <span className="admin-kv-card__label">{t('admin.oidcClientSecret')}</span>
            <span className="admin-kv-card__value">
              {adminRuntime.oidc.hasSecret ? t('admin.secretConfigured') : t('admin.secretNotConfigured')}
            </span>
          </div>
        </div>
        <p className="hint">{t('admin.oidcConfigHint')}</p>
      </section>
    </div>
  )
}
