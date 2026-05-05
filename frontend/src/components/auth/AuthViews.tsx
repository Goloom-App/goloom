import type { ReactNode } from 'react'
import { GoloomLogo } from '../brand/GoloomLogo'
import type { AuthStatusRecord } from '../../types'

export function AuthShell({ theme, children }: { theme: 'dark' | 'light'; children: ReactNode }) {
  return (
    <div className="app-shell auth-shell" data-theme={theme}>
      <div className="auth-screen-center">
        <section className="auth-card">
          <div className="auth-card__hero auth-card__hero--compact">
            <GoloomLogo size="lg" className="auth-card__logo" />
            <div className="auth-card__copy">
              <h1>goloom</h1>
              <p className="hint auth-card__tagline">Social scheduling for teams</p>
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
  onViewChange,
  onTokenChange,
  onSubmit,
  onStartOIDCLogin,
}: {
  view: 'bootstrap' | 'login'
  authStatus: AuthStatusRecord | null
  authTokenDraft: string
  authError: string | null
  authSubmitting: boolean
  onViewChange: (view: 'bootstrap' | 'login') => void
  onTokenChange: (value: string) => void
  onSubmit: (mode: 'bootstrap' | 'login') => void
  onStartOIDCLogin: () => void
}) {
  const initial = Boolean(authStatus?.initialSetupRequired)
  const recovery = Boolean(authStatus?.bootstrapRecoveryEnabled && !initial)
  const isBootstrap = view === 'bootstrap'
  const showDevHints = authStatus?.appEnv !== 'production'

  if (!authStatus && !authSubmitting) {
    return (
      <div className="auth-panel">
        <p className="hint auth-panel__solo">Could not reach the server. Check that the app is running and try again.</p>
      </div>
    )
  }

  if (!authStatus) {
    return (
      <div className="auth-panel">
        <p className="hint auth-panel__solo">Connecting…</p>
      </div>
    )
  }

  const submitMode: 'bootstrap' | 'login' = initial || isBootstrap ? 'bootstrap' : 'login'
  const tokenLabel = initial || isBootstrap ? 'Administrator token' : 'Access token'
  const tokenHint = initial
    ? 'On first start the server prints a one-time token to stdout (for example container logs). Paste it here.'
    : isBootstrap
      ? 'Paste the bootstrap token from BOOTSTRAP_ADMIN_TOKEN (recovery mode).'
      : 'Paste an API token, OIDC ID token, or other bearer token issued for your account.'

  return (
    <div className="auth-panel">
      {recovery ? (
        <div className="auth-tabs" role="tablist" aria-label="Sign-in mode">
          <button type="button" className={view === 'login' ? 'button button--prominent' : 'button button--secondary'} onClick={() => onViewChange('login')}>
            Sign in
          </button>
          <button type="button" className={view === 'bootstrap' ? 'button button--prominent' : 'button button--secondary'} onClick={() => onViewChange('bootstrap')}>
            Bootstrap recovery
          </button>
        </div>
      ) : null}

      <div className="auth-panel__content">
        <div className="auth-panel__header auth-panel__header--solo">
          <div>
            <p className="eyebrow">{initial ? 'First start' : isBootstrap ? 'Recovery' : 'Sign in'}</p>
            <h2>{initial ? 'Welcome' : isBootstrap ? 'Bootstrap recovery' : 'Sign in'}</h2>
            <p className="hint">
              {initial
                ? 'Complete setup with OpenID Connect or the administrator token from your server log.'
                : authStatus.oidcOAuthEnabled && !isBootstrap
                  ? 'Use your identity provider, or sign in with a token.'
                  : tokenHint}
            </p>
          </div>
        </div>

        {authStatus.hasUsers || authStatus.oidcOAuthEnabled || initial ? (
          <div className="auth-form">
            {authStatus.oidcOAuthEnabled && (initial || !isBootstrap) ? (
              <div className="inline-cluster">
                <button type="button" className="button button--prominent" onClick={onStartOIDCLogin} disabled={authSubmitting}>
                  {authSubmitting ? 'Redirecting…' : 'Continue with OpenID Connect'}
                </button>
              </div>
            ) : null}

            {authStatus.oidcOAuthEnabled && (initial || !isBootstrap) ? <p className="hint auth-form__divider-label">or use a token</p> : null}

            <label className="field">
              <span>{tokenLabel}</span>
              <input
                type="password"
                autoComplete="off"
                value={authTokenDraft}
                onChange={(event) => onTokenChange(event.target.value)}
                placeholder={initial ? 'Paste token from server log' : isBootstrap ? 'Bootstrap token' : 'Bearer token'}
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
                {authSubmitting ? 'Signing in…' : 'Sign in with token'}
              </button>
            </div>

            {showDevHints ? <p className="hint">API base URL can be changed later under Settings if this UI is not served from the same host as the API.</p> : null}
          </div>
        ) : null}

        {authStatus.hasUsers && !authStatus.oidcOAuthEnabled && !authStatus.bootstrapRecoveryEnabled ? (
          <p className="hint">
            OIDC browser login is not configured. Use an API token or ask an admin to set OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_REDIRECT_URI, and PUBLIC_BASE_URL.
          </p>
        ) : null}
      </div>
    </div>
  )
}
