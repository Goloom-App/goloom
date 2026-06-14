import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { GoloomLogo } from '../brand/GoloomLogo'
import type { AuthStatusRecord } from '../../types'
import { MeshBackground } from './MeshBackground'

export function AuthShell({ theme, children }: { theme: 'dark' | 'light'; children: ReactNode }) {
  const { t } = useTranslation()
  return (
    <div className="app-shell auth-shell" data-theme={theme}>
      <MeshBackground />
      <div className="auth-screen-center">
        <section className="auth-card">
          <div className="auth-card__hero auth-card__hero--compact">
            <GoloomLogo size="lg" className="auth-card__logo" />
            <div className="auth-card__copy">
              <h1>goloom</h1>
              <p className="hint auth-card__tagline">{t('auth.tagline')}</p>
            </div>
          </div>
          {children}
        </section>
      </div>
    </div>
  )
}

export function AuthPanel({
  view,
  authStatus,
  authTokenDraft,
  authError,
  authSubmitting,
  recoveryMode,
  onViewChange,
  onTokenChange,
  onSubmit,
  onStartOIDCLogin,
  onUseRecovery,
}: {
  view: 'bootstrap' | 'login'
  authStatus: AuthStatusRecord | null
  authTokenDraft: string
  authError: string | null
  authSubmitting: boolean
  recoveryMode: boolean
  onViewChange: (view: 'bootstrap' | 'login') => void
  onTokenChange: (value: string) => void
  onSubmit: (mode: 'bootstrap' | 'login') => void
  onStartOIDCLogin: () => void
  onUseRecovery: () => void
}) {
  const { t } = useTranslation()
  const initial = Boolean(authStatus?.initialSetupRequired)
  const recovery = Boolean(authStatus?.bootstrapRecoveryEnabled && !initial)
  const isBootstrap = view === 'bootstrap'
  const showDevHints = authStatus?.appEnv !== 'production'

  if (!authStatus && !authSubmitting) {
    return (
      <div className="auth-panel">
        <p className="hint auth-panel__solo">{t('auth.serverUnreachable')}</p>
      </div>
    )
  }

  if (!authStatus) {
    return (
      <div className="auth-panel">
        <p className="hint auth-panel__solo">{t('auth.connecting')}</p>
      </div>
    )
  }

  // OIDC-first: when OIDC is configured this is the primary (and auto-started)
  // path. The token/bootstrap form only appears for first-start, OIDC-less
  // deployments, or when explicitly opened via the recovery fallback URL.
  if (authStatus.oidcOAuthEnabled && !initial && !recoveryMode) {
    return (
      <div className="auth-panel">
        <div className="auth-panel__content">
          <div className="auth-panel__header auth-panel__header--solo">
            <div>
              <p className="eyebrow">{t('auth.signIn')}</p>
              <h2>{t('auth.signIn')}</h2>
              <p className="hint">{t('auth.signInHintOidc')}</p>
            </div>
          </div>
          <div className="auth-form">
            <div className="inline-cluster">
              <button type="button" className="button button--prominent" onClick={onStartOIDCLogin} disabled={authSubmitting}>
                {authSubmitting ? t('auth.redirecting') : t('auth.continueOidc')}
              </button>
            </div>
            {authError ? (
              <div className="status-banner">
                <span className="status-banner__error">{authError}</span>
              </div>
            ) : null}
            <button type="button" className="auth-recovery-link" onClick={onUseRecovery}>
              {t('auth.useRecoveryToken')}
            </button>
          </div>
        </div>
      </div>
    )
  }

  const submitMode: 'bootstrap' | 'login' = initial || isBootstrap ? 'bootstrap' : 'login'
  const tokenLabel = initial || isBootstrap ? t('auth.administratorToken') : t('auth.accessToken')
  const tokenHint = initial
    ? t('auth.tokenHintInitial')
    : isBootstrap
      ? t('auth.tokenHintBootstrap')
      : t('auth.tokenHintLogin')

  return (
    <div className="auth-panel">
      {recovery ? (
        <div className="auth-tabs" role="tablist" aria-label={t('auth.signInMode')}>
          <button type="button" className={view === 'login' ? 'button button--prominent' : 'button button--secondary'} onClick={() => onViewChange('login')}>
            {t('auth.signIn')}
          </button>
          <button type="button" className={view === 'bootstrap' ? 'button button--prominent' : 'button button--secondary'} onClick={() => onViewChange('bootstrap')}>
            {t('auth.bootstrapRecovery')}
          </button>
        </div>
      ) : null}

      <div className="auth-panel__content">
        <div className="auth-panel__header auth-panel__header--solo">
          <div>
            <p className="eyebrow">{initial ? t('auth.firstStart') : isBootstrap ? t('auth.recovery') : t('auth.signIn')}</p>
            <h2>{initial ? t('auth.welcome') : isBootstrap ? t('auth.bootstrapRecovery') : t('auth.signIn')}</h2>
            <p className="hint">
              {initial
                ? t('auth.welcomeHint')
                : authStatus.oidcOAuthEnabled && !isBootstrap
                  ? t('auth.signInHintOidc')
                  : tokenHint}
            </p>
          </div>
        </div>

        {authStatus.hasUsers || authStatus.oidcOAuthEnabled || initial ? (
          <div className="auth-form">
            {authStatus.oidcOAuthEnabled && !recoveryMode && (initial || !isBootstrap) ? (
              <div className="inline-cluster">
                <button type="button" className="button button--prominent" onClick={onStartOIDCLogin} disabled={authSubmitting}>
                  {authSubmitting ? t('auth.redirecting') : t('auth.continueOidc')}
                </button>
              </div>
            ) : null}

            {authStatus.oidcOAuthEnabled && !recoveryMode && (initial || !isBootstrap) ? <p className="hint auth-form__divider-label">{t('auth.orUseToken')}</p> : null}

            <label className="field">
              <span>{tokenLabel}</span>
              <input
                type="password"
                autoComplete="off"
                value={authTokenDraft}
                onChange={(event) => onTokenChange(event.target.value)}
                placeholder={
                  initial ? t('auth.placeholderInitial') : isBootstrap ? t('auth.placeholderBootstrap') : t('auth.placeholderBearer')
                }
              />
            </label>
            <p className="hint">{tokenHint}</p>

            {authError ? (
              <div className="status-banner">
                <span className="status-banner__error">{authError}</span>
              </div>
            ) : null}

            <div className="inline-cluster">
              <button type="button" className="button button--prominent" onClick={() => onSubmit(submitMode)} disabled={authSubmitting}>
                {authSubmitting ? t('auth.signingIn') : t('auth.signInWithToken')}
              </button>
            </div>

            {showDevHints ? <p className="hint">{t('auth.settingsApiHint')}</p> : null}
          </div>
        ) : null}

        {authStatus.hasUsers && !authStatus.oidcOAuthEnabled && !authStatus.bootstrapRecoveryEnabled ? (
          <p className="hint">{t('auth.oidcNotConfigured')}</p>
        ) : null}
      </div>
    </div>
  )
}
