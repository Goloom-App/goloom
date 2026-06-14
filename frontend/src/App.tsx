import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { addHours, parseISO, set, startOfDay, startOfMonth } from 'date-fns'

import { AuthPanel, AuthShell } from './components/auth/AuthViews'
import { PostComposer } from './components/Composer/PostComposer'
import { ComposerPreviews } from './components/Composer/ComposerPreviews'
import { buildMediaExcludePayload, defaultEditorDraft, toInputDateTime } from './components/Composer/editorDraft'
import { accountContentOverrideForSave, isAccountOverCharLimit } from './components/Composer/composerUtils'
import type { EditorDraftState } from './components/Composer/types'
import { SocialPreview } from './components/post/SocialPreview'
import { MobilePreviewOverlay } from './components/post/MobilePreviewOverlay'
import { AppShell } from './components/Shell/AppShell'
import { Sun, Moon, Edit, Trash2, Copy, X } from 'lucide-react'
import { AnalyticsView } from './views/Analytics/AnalyticsView'
import { ArchiveView } from './views/calendar/ArchiveView'
import { DashboardView } from './views/dashboard/DashboardView'
import { calendarCellsForMonth } from './views/calendar/calendarUtils'
import { ContentCalendarView } from './views/calendar/ContentCalendarView'
import { ScheduleView } from './views/calendar/ScheduleView'
import { AccountsView } from './views/accounts/AccountsView'
import { defaultAccountConnectDraft, type AccountConnectDraft } from './views/accounts/accountConnectTypes'
import { AdminView } from './views/admin/AdminView'
import { defaultAdminProviderDraft, type AdminProviderDraft } from './views/admin/adminTypes'
import { MediaLibraryView } from './views/media/MediaLibraryView'
import { SettingsView } from './views/settings/SettingsView'
import { TeamSettingsView } from './views/teams/TeamSettingsView'
import { ImportOldPostsDialog } from './components/settings/ImportOldPostsDialog'
import { AIChatWidget } from './components/ai/AIChatWidget'
import { BrandProfileView } from './views/ai/BrandProfileView'
import { AIGeneratorView } from './views/ai/AIGeneratorView'
import { CampaignFormatView } from './views/ai/CampaignFormatView'
import { AutomationView } from './views/automation/AutomationView'
import { ReviewQueueView } from './views/review/ReviewQueueView'
import { useReviewQueueCount } from './hooks/useReviewQueue'
import { useIsMobile, useResolvedTheme } from './hooks/useTheme'
import {
  ApiError,
  createApiClient,
  requestAuthStatus,
  requestStartOIDCLogin,
  type BackendAPIToken,
  type BackendAdminMetrics,
  type BackendAdminSyncStatus,
  type BackendPostMetric,
  type BackendPostVersion,
  type BackendTeam,
} from './api'
import { loadInitialSection, loadInitialTeamId, loadStoredSettings, writeStoredSettings, LAST_SECTION_STORAGE_KEY, LAST_TEAM_STORAGE_KEY } from './appStorage'
import { toAccountRecord, toAuthStatusRecord, toPostRecord, toProviderInstanceRecord, toRuntimeConfigRecord, toTeamMemberRecord, toTeamRecord, toUserRecord } from './mappers'
import { engagementForAccount } from './postMetrics'
import { postsForTeam, resolveScheduleChange, sharedAccountLabels } from './schedule'
import { setAppLanguage, type SupportedLanguage } from './i18n'
import { sectionHeading } from './i18n/sections'
import { translateApiError } from './i18n/translateApiError'
import type { AccountRecord, AppSection, AuthStatusRecord, PostRecord, PostVersionRecord, ProviderInstanceRecord, RuntimeConfigRecord, SettingsState, TeamRecord, TeamRole, UserRecord } from './types'

/** Survives React Strict Mode remounts: hash is promoted here, then one reload applies the session. */
const OIDC_PENDING_SESSION_KEY = 'goloom.oidc.pending_session_v1'
// Set right before an automatic OIDC redirect; prevents a redirect loop if the
// IdP bounces back unauthenticated. Cleared once a session is established.
const OIDC_AUTO_ATTEMPTED_KEY = 'goloom.oidc.auto_attempted'
// Set on explicit sign-out so the login screen stays put instead of bouncing
// straight back into the IdP. Cleared once a session is established.
const OIDC_SUPPRESS_AUTO_KEY = 'goloom.oidc.suppress_auto'

const CONTENT_REFRESH_SECTIONS: AppSection[] = ['dashboard', 'calendar', 'archive', 'contentCalendar', 'analytics']

function App() {
  const { t } = useTranslation()
  const [section, setSection] = useState<AppSection>(() => loadInitialSection())
  const isMobile = useIsMobile()
  const [currentDate] = useState<Date>(new Date())
  const [contentCalendarMonth, setContentCalendarMonth] = useState(() => startOfMonth(new Date()))
  const [calendarDragOverKey, setCalendarDragOverKey] = useState<string | null>(null)
  const prevSectionRef = useRef<AppSection | null>(null)
  const prevSectionBeforeComposerRef = useRef<AppSection | null>(null)
  const [settings, setSettings] = useState<SettingsState>(() => loadStoredSettings())

  useEffect(() => {
    const lang = settings.ui.language
    if (lang) {
      setAppLanguage(lang as SupportedLanguage)
    }
  }, [settings.ui.language])
  const [activeConnection, setActiveConnection] = useState(() => ({
    apiBaseUrl: loadStoredSettings().general.apiBaseUrl,
    bearerToken: loadStoredSettings().general.bearerToken,
  }))
  const [authStatus, setAuthStatus] = useState<AuthStatusRecord | null>(null)
  const [authStatusLoading, setAuthStatusLoading] = useState(true)
  const [authView, setAuthView] = useState<'bootstrap' | 'login'>('login')
  // Recovery mode (token/bootstrap sign-in) is reached via the ?login=recovery
  // fallback URL; it also suppresses the automatic OIDC redirect.
  const [recoveryMode, setRecoveryMode] = useState<boolean>(() => {
    if (typeof window === 'undefined') {
      return false
    }
    return new URLSearchParams(window.location.search).get('login') === 'recovery'
  })
  const [authTokenDraft, setAuthTokenDraft] = useState(() => loadStoredSettings().general.bearerToken)
  const [principalUser, setPrincipalUser] = useState<UserRecord | null>(null)
  // Token id of the current request principal (from /v1/me). For a browser
  // session this lets the token list mark "this browser".
  const [currentTokenId, setCurrentTokenId] = useState<string | null>(null)
  // Auth is determined by a /v1/me probe (cookie session or dev bearer override),
  // not by a token stored client-side. authProbed gates the first render.
  const [authenticated, setAuthenticated] = useState(false)
  const [authProbed, setAuthProbed] = useState(false)
  const [authError, setAuthError] = useState<string | null>(null)
  const [authSubmitting, setAuthSubmitting] = useState(false)
  const [teams, setTeams] = useState<TeamRecord[]>([])
  const [accounts, setAccounts] = useState<AccountRecord[]>([])
  const [posts, setPosts] = useState<PostRecord[]>([])
  const [postVersions, setPostVersions] = useState<PostVersionRecord[]>([])
  const [selectedTeamId, setSelectedTeamId] = useState(() => loadInitialTeamId())
  const [expandedPostId, setExpandedPostId] = useState<string | null>(null)
  const [previewPostMetrics, setPreviewPostMetrics] = useState<BackendPostMetric[]>([])
  const [editingPostId, setEditingPostId] = useState<string | null>(null)
  const [editingAccountId, setEditingAccountId] = useState<string | null>(null)
  const [composerMode, setComposerMode] = useState<'create' | 'edit'>('create')
  const [editorDraft, setEditorDraft] = useState<EditorDraftState>(() => defaultEditorDraft(currentDate, []))
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [dismissedNoticeKey, setDismissedNoticeKey] = useState<string | null>(null)
  const [directoryUsers, setDirectoryUsers] = useState<UserRecord[]>([])
  const [providerInstances, setProviderInstances] = useState<ProviderInstanceRecord[]>([])
  const [adminMetrics, setAdminMetrics] = useState<BackendAdminMetrics | null>(null)
  const [adminMetricsLoading, setAdminMetricsLoading] = useState(false)
  const [adminRuntime, setAdminRuntime] = useState<RuntimeConfigRecord | null>(null)
  const [adminSyncStatus, setAdminSyncStatus] = useState<BackendAdminSyncStatus | null>(null)
  const [adminSyncLoading, setAdminSyncLoading] = useState(false)
  const [apiTokens, setApiTokens] = useState<BackendAPIToken[]>([])
  const [apiTokensLoading, setApiTokensLoading] = useState(false)
  const [teamSettingsName, setTeamSettingsName] = useState('')
  const [teamSettingsDescription, setTeamSettingsDescription] = useState('')
  const [addMemberUserId, setAddMemberUserId] = useState('')
  const [memberRoleEdits, setMemberRoleEdits] = useState<Record<string, TeamRole>>({})
  const [teamAiEnabled, setTeamAiEnabled] = useState(false)
  const [externalPostMonitorEnabled, setExternalPostMonitorEnabled] = useState(false)
  const [importOldPostsOpen, setImportOldPostsOpen] = useState(false)
  const [teamAiProvider, setTeamAiProvider] = useState('openai')
  const [teamAiModel, setTeamAiModel] = useState('')
  const [teamAiBaseUrl, setTeamAiBaseUrl] = useState('')
  const [teamAiApiKey, setTeamAiApiKey] = useState('')
  const [teamAiKeySet, setTeamAiKeySet] = useState(false)
  const [adminAIEnabledTeams, setAdminAIEnabledTeams] = useState<BackendTeam[]>([])
  const [adminAIEnabledTeamsLoading, setAdminAIEnabledTeamsLoading] = useState(false)
  const [adminProviderDraft, setAdminProviderDraft] = useState<AdminProviderDraft>(() => defaultAdminProviderDraft())
  const [editingProviderId, setEditingProviderId] = useState<string | null>(null)
  const [showAdminProviderAdvanced, setShowAdminProviderAdvanced] = useState(false)
  const [accountDraft, setAccountDraft] = useState<AccountConnectDraft>(() => defaultAccountConnectDraft())
  const [mobilePreviewPostId, setMobilePreviewPostId] = useState<string | null>(null)
  const [teamBrandColor, setTeamBrandColor] = useState('')

  // The api client is always built. Web sessions authenticate via the HttpOnly
  // cookie (sent automatically, same-origin); an optional bearer override is only
  // used by the dev "Browser session" panel.
  const api = useMemo(
    () => createApiClient({ baseUrl: activeConnection.apiBaseUrl.trim(), token: activeConnection.bearerToken.trim() }),
    [activeConnection.apiBaseUrl, activeConnection.bearerToken],
  )

  useEffect(() => {
    writeStoredSettings(settings)
  }, [settings])

  useEffect(() => {
    if (typeof window === 'undefined') {
      return
    }
    window.localStorage.setItem(LAST_SECTION_STORAGE_KEY, section)
  }, [section])

  useEffect(() => {
    if (typeof window === 'undefined') {
      return
    }
    const teamID = selectedTeamId.trim()
    if (!teamID) {
      window.localStorage.removeItem(LAST_TEAM_STORAGE_KEY)
      return
    }
    window.localStorage.setItem(LAST_TEAM_STORAGE_KEY, teamID)
  }, [selectedTeamId])

  const resolvedTheme = useResolvedTheme(settings.ui.colorScheme)

  const noticeKey = `${loading ? 'L' : ''}|${error ?? ''}|${statusMessage ?? ''}`
  useEffect(() => {
    setDismissedNoticeKey(null)
  }, [noticeKey])

  useEffect(() => {
    let cancelled = false
    const apiOrigin = settings.general.apiBaseUrl.trim() || (typeof window !== 'undefined' ? window.location.origin : '')
    requestAuthStatus(apiOrigin)
      .then((status) => {
        if (cancelled) {
          return
        }
        const mapped = toAuthStatusRecord(status)
        setAuthStatus(mapped)
        if (!activeConnection.bearerToken.trim()) {
          setAuthView(mapped.bootstrapRecoveryEnabled ? 'login' : 'login')
        }
      })
      .catch(() => {
        if (cancelled) {
          return
        }
        setAuthStatus(null)
      })
      .finally(() => {
        if (!cancelled) {
          setAuthStatusLoading(false)
        }
      })

    return () => {
      cancelled = true
    }
  }, [activeConnection.bearerToken, settings.general.apiBaseUrl])

  useEffect(() => {
    const teamID = new URLSearchParams(window.location.search).get('team')
    if (teamID) {
      setSelectedTeamId(teamID)
    }
  }, [])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const oauthStatus = params.get('oauth_status')
    if (!oauthStatus) {
      return
    }

    const provider = params.get('oauth_provider') || 'provider'
    const message =
      params.get('oauth_message') ||
      (oauthStatus === 'success' ? t('status.oauthConnected', { provider }) : t('status.oauthFailed', { provider }))

    if (oauthStatus === 'success') {
      setStatusMessage(message)
      setError(null)
      setAuthError(null)
    } else if (provider === 'oidc') {
      // Login screen only shows authError; general `error` is for the main app shell.
      setAuthError(message)
      setError(null)
      setStatusMessage(null)
    } else {
      setError(message)
      setAuthError(null)
      setStatusMessage(null)
    }

    params.delete('oauth_status')
    params.delete('oauth_provider')
    params.delete('oauth_message')
    const nextQuery = params.toString()
    const nextURL = `${window.location.pathname}${nextQuery ? `?${nextQuery}` : ''}${window.location.hash}`
    window.history.replaceState({}, document.title, nextURL)
  }, [t])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('section') !== 'accounts') {
      return
    }
    setSection('accounts')
    params.delete('section')
    const nextQuery = params.toString()
    const nextURL = `${window.location.pathname}${nextQuery ? `?${nextQuery}` : ''}${window.location.hash}`
    window.history.replaceState({}, document.title, nextURL)
  }, [])

  useEffect(() => {
    const raw = sessionStorage.getItem(OIDC_PENDING_SESSION_KEY)
    if (!raw) {
      return
    }
    sessionStorage.removeItem(OIDC_PENDING_SESSION_KEY)
    let payload: { token: string; baseUrl: string }
    try {
      payload = JSON.parse(raw) as { token: string; baseUrl: string }
    } catch {
      setAuthError(t('auth.invalidOidcHandoff'))
      return
    }
    const token = typeof payload.token === 'string' ? payload.token.trim() : ''
    const baseUrl = typeof payload.baseUrl === 'string' ? payload.baseUrl.trim() : ''
    if (!token) {
      setAuthError(t('auth.emptyOidcToken'))
      return
    }
    const apiBase = baseUrl || (typeof window !== 'undefined' ? window.location.origin : '')
    void (async () => {
      setAuthSubmitting(true)
      setAuthError(null)
      setStatusMessage(null)
      try {
        const meResponse = await createApiClient({ baseUrl: apiBase, token }).me()
        setActiveConnection({ apiBaseUrl: apiBase, bearerToken: token })
        setSettings((current) => ({
          ...current,
          general: { ...current.general, apiBaseUrl: apiBase, bearerToken: token },
        }))
        setAuthTokenDraft(token)
        setPrincipalUser(toUserRecord(meResponse.user))
        setCurrentTokenId(meResponse.token_id ?? null)
        setStatusMessage(t('auth.signedInOidc'))
      } catch (cause) {
        if (cause instanceof ApiError && cause.status === 401) {
          setAuthError(t('auth.oidcRejected'))
        } else {
          setAuthError(
            cause instanceof Error ? translateApiError(cause.message, t) : t('auth.signInFailed'),
          )
        }
      } finally {
        setAuthSubmitting(false)
      }
    })()
  }, [])

  useEffect(() => {
    const hash = window.location.hash
    if (!hash.startsWith('#goloom_oidc_token=')) {
      return
    }
    const encoded = hash.slice('#goloom_oidc_token='.length)
    let token: string
    try {
      token = decodeURIComponent(encoded)
    } catch {
      setAuthError(t('auth.invalidSignInResponse'))
      window.history.replaceState({}, document.title, `${window.location.pathname}${window.location.search}`)
      return
    }
    if (!token.trim()) {
      setAuthError(t('auth.invalidSignInResponse'))
      window.history.replaceState({}, document.title, `${window.location.pathname}${window.location.search}`)
      return
    }
    const baseUrl = loadStoredSettings().general.apiBaseUrl.trim() || window.location.origin
    try {
      sessionStorage.setItem(OIDC_PENDING_SESSION_KEY, JSON.stringify({ token, baseUrl }))
    } catch {
      setAuthError(t('auth.storeSignInFailed'))
      return
    }
    window.history.replaceState({}, document.title, `${window.location.pathname}${window.location.search}`)
    window.location.reload()
  }, [])
  // OIDC-first: when OIDC is configured and the user is not signed in, start the
  // login automatically. Guards prevent a redirect loop (auto_attempted) and a
  // bounce right after an explicit sign-out (suppress_auto); the ?login=recovery
  // fallback and first-start/token-only setups always show the form instead.
  useEffect(() => {
    if (authStatusLoading || !authStatus) {
      return
    }
    // Wait for the session probe; never redirect a signed-in (cookie) user.
    if (!authProbed || authenticated) {
      return
    }
    if (!authStatus.oidcOAuthEnabled || authStatus.initialSetupRequired || recoveryMode || authError) {
      return
    }
    if (typeof window !== 'undefined' && window.sessionStorage) {
      if (sessionStorage.getItem(OIDC_SUPPRESS_AUTO_KEY) || sessionStorage.getItem(OIDC_AUTO_ATTEMPTED_KEY)) {
        return
      }
      sessionStorage.setItem(OIDC_AUTO_ATTEMPTED_KEY, '1')
    }
    void startOIDCLogin()
    // startOIDCLogin is a stable function declaration; deps cover the gating state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authStatus, authStatusLoading, authProbed, authenticated, recoveryMode, authError])

  const enterRecoveryMode = useCallback(() => {
    setRecoveryMode(true)
    setAuthError(null)
    if (typeof window !== 'undefined') {
      const url = `${window.location.pathname}?login=recovery${window.location.hash}`
      window.history.replaceState({}, document.title, url)
    }
  }, [])

  const clearAuthenticatedState = useCallback((message?: string) => {
    // Stay on the login screen after an explicit sign-out instead of bouncing
    // straight back into the IdP.
    if (typeof window !== 'undefined' && window.sessionStorage) {
      sessionStorage.setItem(OIDC_SUPPRESS_AUTO_KEY, '1')
      sessionStorage.removeItem(OIDC_AUTO_ATTEMPTED_KEY)
    }
    setActiveConnection((current) => ({ ...current, bearerToken: '' }))
    setSettings((current) => ({
      ...current,
      general: { ...current.general, bearerToken: '' },
    }))
    setAuthTokenDraft('')
    setPrincipalUser(null)
    setCurrentTokenId(null)
    setAuthenticated(false)
    setTeams([])
    setAccounts([])
    setPosts([])
    setSelectedTeamId('')
    setExpandedPostId(null)
    setEditingPostId(null)
    prevSectionBeforeComposerRef.current = null
    setSection('calendar')
    setLoading(false)
    if (message) {
      setStatusMessage(message)
    }
  }, [])

  async function authenticateWithToken(mode: 'bootstrap' | 'login') {
    const token = authTokenDraft.trim()
    if (!token) {
      setAuthError(mode === 'bootstrap' ? t('auth.enterBootstrapToken') : t('auth.enterBearerToken'))
      return
    }

    setAuthSubmitting(true)
    setAuthError(null)
    setStatusMessage(null)

    try {
      // Exchange the token for an HttpOnly cookie session (no token kept client-side).
      const res = await api.sessionFromToken(token)
      setAuthTokenDraft('')
      if (typeof window !== 'undefined' && window.sessionStorage) {
        sessionStorage.removeItem(OIDC_AUTO_ATTEMPTED_KEY)
        sessionStorage.removeItem(OIDC_SUPPRESS_AUTO_KEY)
      }
      setPrincipalUser(toUserRecord(res.user))
      setAuthenticated(true)
      setAuthProbed(true)
      setStatusMessage(mode === 'bootstrap' ? t('status.bootstrapAdminSignedIn') : t('status.signedIn'))
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        setAuthError(mode === 'bootstrap' ? t('auth.bootstrapTokenRejected') : t('auth.bearerTokenRejected'))
      } else {
        setAuthError(cause instanceof Error ? translateApiError(cause.message, t) : t('auth.signInFailed'))
      }
    } finally {
      setAuthSubmitting(false)
    }
  }

  async function startOIDCLogin() {
    setAuthError(null)
    setStatusMessage(null)
    setAuthSubmitting(true)
    try {
      const baseUrl = settings.general.apiBaseUrl.trim() || window.location.origin
      const returnTo = `${window.location.origin}${window.location.pathname}${window.location.search}`
      const { authorization_url: authorizationUrl } = await requestStartOIDCLogin(baseUrl, returnTo)
      window.location.href = authorizationUrl
    } catch (cause) {
      setAuthSubmitting(false)
      setAuthError(cause instanceof Error ? translateApiError(cause.message, t) : t('auth.oidcStartFailed'))
    }
  }

  function updateAPIBaseURL(value: string) {
    setAuthStatusLoading(true)
    setSettings((current) => ({
      ...current,
      general: { ...current.general, apiBaseUrl: value },
    }))
  }

  const loadDashboard = useCallback(async (opts?: { silent?: boolean }) => {
    if (!api) {
      return
    }

    const silent = Boolean(opts?.silent)
    if (!silent) {
      setLoading(true)
      setError(null)
    }

    try {
      const meResponse = await api.me()
      setPrincipalUser(toUserRecord(meResponse.user))
      setCurrentTokenId(meResponse.token_id ?? null)
      // Session established: reset the OIDC auto-login guards.
      if (typeof window !== 'undefined' && window.sessionStorage) {
        sessionStorage.removeItem(OIDC_AUTO_ATTEMPTED_KEY)
        sessionStorage.removeItem(OIDC_SUPPRESS_AUTO_KEY)
      }

      const [usersResponse, teamsResponse, providerInstancesResponse] = await Promise.all([
        api.listUsers(),
        api.listTeams(),
        api.listProviderInstances(),
      ])

      setDirectoryUsers((usersResponse.items ?? []).map(toUserRecord))
      const mappedProviderInstances = (providerInstancesResponse.items ?? []).map(toProviderInstanceRecord)
      setProviderInstances(mappedProviderInstances)

      const teamPayloads = await Promise.all(
        (teamsResponse.items ?? []).map(async (team) => {
          const [membersResponse, accountsResponse, postsResponse, versionsResponse] = await Promise.all([
            api.listTeamMembers(team.id),
            api.listAccounts(team.id),
            api.listPosts(team.id),
            api.listAllPostVersions(team.id),
          ])

          const mappedAccounts = (accountsResponse.items ?? []).map((account) => toAccountRecord(account, mappedProviderInstances))
          const mappedPosts = (postsResponse.items ?? []).map(toPostRecord)
          const mappedMembers = (membersResponse.items ?? []).map(toTeamMemberRecord)
          const mappedVersions = (versionsResponse.items ?? []).map((v: BackendPostVersion) => ({
            postId: v.post_id,
            accountId: v.account_id,
            content: v.content,
          }))

          return {
            team: toTeamRecord(team, mappedMembers, mappedAccounts.map((account) => account.id)),
            accounts: mappedAccounts,
            posts: mappedPosts,
            versions: mappedVersions,
          }
        }),
      )

      setTeams(teamPayloads.map((payload) => payload.team))
      setAccounts(teamPayloads.flatMap((payload) => payload.accounts))
      setPosts(teamPayloads.flatMap((payload) => payload.posts))
      setPostVersions(teamPayloads.flatMap((payload) => payload.versions))
      setExpandedPostId((current) => (current && teamPayloads.flatMap((payload) => payload.posts).some((post) => post.id === current) ? current : null))
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        clearAuthenticatedState(t('status.sessionExpired'))
        setAuthError(t('status.sessionExpiredSignIn'))
        return
      }
      if (!silent) {
        setError(translateApiError(cause instanceof Error ? cause.message : t('status.failedLoadDashboard'), t))
      }
    } finally {
      if (!silent) {
        setLoading(false)
      }
    }
  }, [api, clearAuthenticatedState])

  // Probe the session (cookie or dev bearer) on load / connection change. Drives
  // the auth gate; a 401 simply means "not signed in" (no error, no logout).
  useEffect(() => {
    let cancelled = false
    void api
      .me()
      .then((me) => {
        if (cancelled) {
          return
        }
        setPrincipalUser(toUserRecord(me.user))
        setCurrentTokenId(me.token_id ?? null)
        if (typeof window !== 'undefined' && window.sessionStorage) {
          sessionStorage.removeItem(OIDC_AUTO_ATTEMPTED_KEY)
          sessionStorage.removeItem(OIDC_SUPPRESS_AUTO_KEY)
        }
        setAuthenticated(true)
      })
      .catch(() => {
        if (!cancelled) {
          setAuthenticated(false)
        }
      })
      .finally(() => {
        if (!cancelled) {
          setAuthProbed(true)
        }
      })
    return () => {
      cancelled = true
    }
  }, [api])

  useEffect(() => {
    if (api && authenticated) {
      const timer = window.setTimeout(() => {
        void loadDashboard()
      }, 0)
      return () => window.clearTimeout(timer)
    }
  }, [api, authenticated, loadDashboard])

  useEffect(() => {
    if (!api) {
      return
    }
    if (prevSectionRef.current === null) {
      prevSectionRef.current = section
      return
    }
    if (prevSectionRef.current !== section && CONTENT_REFRESH_SECTIONS.includes(section)) {
      void loadDashboard({ silent: true })
    }
    prevSectionRef.current = section
  }, [api, section, loadDashboard])

  useEffect(() => {
    if (!api || !CONTENT_REFRESH_SECTIONS.includes(section)) {
      return
    }
    const seconds = settings.scheduler.pollIntervalSeconds ?? 15
    const intervalMs = Math.min(60_000, Math.max(5_000, seconds * 1000))
    const id = window.setInterval(() => {
      void loadDashboard({ silent: true })
    }, intervalMs)
    return () => window.clearInterval(id)
  }, [api, section, loadDashboard, settings.scheduler.pollIntervalSeconds])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const inviteToken = params.get('invite')
    if (!inviteToken || !api) {
      return
    }
    let cancelled = false
    void (async () => {
      try {
        await api.acceptTeamInvitation({ token: inviteToken })
        if (!cancelled) {
          setStatusMessage(t('status.invitationAccepted'))
          setError(null)
          params.delete('invite')
          const next = `${window.location.pathname}${params.toString() ? `?${params.toString()}` : ''}${window.location.hash}`
          window.history.replaceState({}, document.title, next)
          await loadDashboard({ silent: true })
        }
      } catch (cause) {
        if (!cancelled) {
          setError(translateApiError(cause instanceof Error ? cause.message : t('status.failedAcceptInvitation'), t))
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [api, loadDashboard])

  useEffect(() => {
    if (!api || section !== 'admin' || principalUser?.globalRole !== 'admin') {
      return
    }
    let cancelled = false
    setAdminMetricsLoading(true)
    setAdminSyncLoading(true)
    void Promise.all([api.adminMetrics(), api.runtimeConfig(), api.adminSyncStatus()])
      .then(([m, r, sync]) => {
        if (!cancelled) {
          setAdminMetrics(m)
          setAdminRuntime(toRuntimeConfigRecord(r))
          setAdminSyncStatus(sync)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setAdminMetrics(null)
          setAdminRuntime(null)
          setAdminSyncStatus(null)
        }
      })
      .finally(() => {
        if (!cancelled) {
          setAdminMetricsLoading(false)
          setAdminSyncLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [api, section, principalUser?.globalRole])

  useEffect(() => {
    if (!api || section !== 'settings') {
      return
    }
    let cancelled = false
    setApiTokensLoading(true)
    void api
      .listMyApiTokens()
      .then((r) => {
        if (!cancelled) {
          setApiTokens(r.items ?? [])
        }
      })
      .finally(() => {
        if (!cancelled) {
          setApiTokensLoading(false)
        }
      })
    return () => {
      cancelled = true
    }
  }, [api, section])

  const effectiveSelectedTeamId = selectedTeamId || teams[0]?.id || ''
  const { count: reviewQueueCount, overdueCount: reviewQueueOverdueCount } = useReviewQueueCount(
    principalUser && effectiveSelectedTeamId ? effectiveSelectedTeamId : '',
  )
  const selectedTeam = useMemo(
    () => teams.find((team) => team.id === effectiveSelectedTeamId) ?? null,
    [effectiveSelectedTeamId, teams],
  )

  const teamAccounts = useMemo(
    () =>
      selectedTeam
        ? accounts.filter((account) => selectedTeam.accountIds.includes(account.id)).sort((left, right) => left.name.localeCompare(right.name))
        : [],
    [accounts, selectedTeam],
  )

  const teamPosts = useMemo(() => postsForTeam(posts, effectiveSelectedTeamId), [effectiveSelectedTeamId, posts])

  const upcomingPosts = useMemo(() => {
    const baseline = startOfDay(currentDate)
    return teamPosts.filter(
      (post) =>
        (post.status === 'scheduled' || post.status === 'draft') && parseISO(post.scheduledAt) >= baseline,
    )
  }, [currentDate, teamPosts])

  const archivedPosts = useMemo(
    () => [...teamPosts].filter((post) => post.status === 'posted').sort((left, right) => parseISO(right.scheduledAt).getTime() - parseISO(left.scheduledAt).getTime()),
    [teamPosts],
  )

  const plannedPostsForContentCalendar = useMemo(
    () => teamPosts.filter((post) => post.status === 'scheduled' || post.status === 'draft'),
    [teamPosts],
  )

  const dashboardUpcomingPosts = useMemo(
    () =>
      [...teamPosts]
        .filter((post) => post.status === 'scheduled' || post.status === 'draft')
        .sort((left, right) => parseISO(left.scheduledAt).getTime() - parseISO(right.scheduledAt).getTime())
        .slice(0, 10),
    [teamPosts],
  )

  // Stable chart fetchers: inline arrows would change identity on every render and
  // make DashboardView refetch + flicker on each parent re-render (e.g. the poll).
  const fetchDashboardSeries = useCallback(
    (metric: string) =>
      api && selectedTeamId
        ? api.getTeamAnalyticsChart(selectedTeamId, { metric, days: 7 })
        : Promise.resolve({ metric, days: 7, series: [] }),
    [api, selectedTeamId],
  )
  const fetchDashboardGrowth = useCallback(
    (accountID: string, opts?: { days?: number }) =>
      api && selectedTeamId
        ? api.getTeamAccountGrowth(selectedTeamId, accountID, opts)
        : Promise.resolve({ days: opts?.days ?? 7, account: accountID, series: [] }),
    [api, selectedTeamId],
  )

  const contentCalendarCells = useMemo(
    () => calendarCellsForMonth(contentCalendarMonth, plannedPostsForContentCalendar),
    [contentCalendarMonth, plannedPostsForContentCalendar],
  )

  const showPreviewColumn = (section === 'calendar' || section === 'archive' || section === 'contentCalendar' || section === 'composer') && !isMobile

  const isComposer = section === 'composer'
  const pullToRefreshDisabled = !isMobile || !CONTENT_REFRESH_SECTIONS.includes(section)

  useEffect(() => {
    if (section !== 'composer') {
      return
    }
    const main = document.querySelector('.app-main')
    if (main instanceof HTMLElement) {
      main.scrollTop = 0
    }
  }, [section])

  const myRoleInSelectedTeam = useMemo((): TeamRole | null => {
    if (!selectedTeam || !principalUser) {
      return null
    }
    return selectedTeam.members.find((m) => m.userId === principalUser.id)?.role ?? null
  }, [principalUser, selectedTeam])

  const canEditTeamAccounts = myRoleInSelectedTeam === 'owner' || myRoleInSelectedTeam === 'editor'
  const canEditScheduledPosts = canEditTeamAccounts

  useEffect(() => {
    if (!selectedTeam) {
      setTeamSettingsName('')
      setTeamSettingsDescription('')
      setTeamAiEnabled(false)
      setExternalPostMonitorEnabled(false)
      setMemberRoleEdits({})
      setTeamBrandColor('')
      return
    }
    setTeamSettingsName(selectedTeam.name)
    setTeamSettingsDescription(selectedTeam.description ?? '')
    setTeamAiEnabled(selectedTeam.isAiEnabled ?? false)
    setMemberRoleEdits(Object.fromEntries(selectedTeam.members.map((member) => [member.userId, member.role])))
    setTeamBrandColor(selectedTeam.brandColor ?? '')
  }, [selectedTeam])

  // Apply the team brand color globally; CSS accent tones derive from --brand-primary.
  useEffect(() => {
    if (typeof document === 'undefined') {
      return
    }
    const color = (selectedTeam?.brandColor ?? '').trim()
    if (/^#[0-9a-fA-F]{6}$/.test(color)) {
      document.documentElement.style.setProperty('--brand-primary', color)
    } else {
      document.documentElement.style.removeProperty('--brand-primary')
    }
  }, [selectedTeam?.brandColor])

  useEffect(() => {
    if (!api || !selectedTeam || myRoleInSelectedTeam !== 'owner') {
      setExternalPostMonitorEnabled(false)
      return
    }
    let cancelled = false
    void api.getExternalPostMonitor(selectedTeam.id).then((settings) => {
      if (!cancelled) {
        setExternalPostMonitorEnabled(settings.enabled)
      }
    }).catch(() => {
      if (!cancelled) {
        setExternalPostMonitorEnabled(false)
      }
    })
    return () => {
      cancelled = true
    }
  }, [api, selectedTeam, myRoleInSelectedTeam])

  const instancesForAccountConnect = useMemo(
    () => providerInstances.filter((p) => p.provider === accountDraft.provider),
    [accountDraft.provider, providerInstances],
  )

  const selectedPost = useMemo(() => teamPosts.find((post) => post.id === expandedPostId) ?? null, [expandedPostId, teamPosts])
  const previewPostForMetrics = useMemo(() => {
    const id = expandedPostId ?? mobilePreviewPostId
    if (!id) {
      return null
    }
    return posts.find((post) => post.id === id) ?? null
  }, [expandedPostId, mobilePreviewPostId, posts])
  const editTargetPost = useMemo(() => teamPosts.find((post) => post.id === editingPostId) ?? null, [editingPostId, teamPosts])

  const openPostPreview = useCallback(
    (postId: string) => {
      if (isMobile) {
        setMobilePreviewPostId(postId)
      } else {
        setExpandedPostId(postId)
        setMobilePreviewPostId(null)
      }
    },
    [isMobile],
  )

  useEffect(() => {
    if (isMobile || !mobilePreviewPostId) {
      return
    }
    setExpandedPostId(mobilePreviewPostId)
    setMobilePreviewPostId(null)
  }, [isMobile, mobilePreviewPostId])

  useEffect(() => {
    if (!api || !previewPostForMetrics || previewPostForMetrics.status !== 'posted') {
      setPreviewPostMetrics([])
      return
    }
    let cancelled = false
    void api
      .getPostAnalytics(previewPostForMetrics.teamId, previewPostForMetrics.id)
      .then((r) => {
        if (!cancelled) {
          setPreviewPostMetrics(r.items ?? [])
        }
      })
      .catch(() => {
        if (!cancelled) {
          setPreviewPostMetrics([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [api, previewPostForMetrics])

  async function runAction(work: () => Promise<void>, successMessage: string) {
    setSyncing(true)
    setError(null)
    setStatusMessage(null)
    try {
      await work()
      setStatusMessage(successMessage)
    } catch (cause) {
      const raw = cause instanceof Error ? cause.message : t('auth.signInFailed')
      setError(translateApiError(raw, t))
    } finally {
      setSyncing(false)
    }
  }

  function openCreateComposer() {
    prevSectionBeforeComposerRef.current = section === 'composer' ? prevSectionBeforeComposerRef.current : section
    setComposerMode('create')
    setEditingPostId(null)
    setEditorDraft(defaultEditorDraft(currentDate, teamAccounts))
    setMobilePreviewPostId(null)
    setSection('composer')
  }

  function openComposerFromGeneratedContent(payload: {
    title?: string
    content: string
    targetAccountIds: string[]
    accountContentOverride?: Record<string, string>
    scheduledAt?: string
  }) {
    const baseDraft = defaultEditorDraft(currentDate, teamAccounts)
    setEditorDraft({
      title: payload.title?.trim() ?? '',
      content: payload.content,
      scheduledAt: payload.scheduledAt ? toInputDateTime(parseISO(payload.scheduledAt)) : baseDraft.scheduledAt,
      targetAccountIds: payload.targetAccountIds,
      status: 'draft',
      accountContentOverride: payload.accountContentOverride ?? {},
      mediaIds: [],
      mediaExcludeByAccount: {},
    })
    setComposerMode('create')
    setEditingPostId(null)
    setMobilePreviewPostId(null)
    prevSectionBeforeComposerRef.current = section === 'composer' ? prevSectionBeforeComposerRef.current : section
    setSection('composer')
  }

  async function openEditor(postId: string) {
    const targetPost = teamPosts.find((post) => post.id === postId)
    if (!targetPost || targetPost.source === 'imported') {
      return
    }
    let accountContentOverride: Record<string, string> = {}
    if (api) {
      try {
        const res = await api.listPostVersions(targetPost.teamId, postId)
        for (const row of res.items ?? []) {
          if (row.content.trim() !== targetPost.content.trim()) {
            accountContentOverride[row.account_id] = row.content
          }
        }
      } catch {
        accountContentOverride = {}
      }
    }
    setEditorDraft({
      title: targetPost.title,
      content: targetPost.content,
      scheduledAt: toInputDateTime(parseISO(targetPost.scheduledAt)),
      targetAccountIds: targetPost.targetAccountIds ?? [],
      status: targetPost.status,
      accountContentOverride,
      mediaIds: targetPost.mediaIds ? [...targetPost.mediaIds] : [],
      mediaExcludeByAccount: targetPost.mediaExcludeByAccount ? { ...targetPost.mediaExcludeByAccount } : {},
    })
    setComposerMode('edit')
    setEditingPostId(postId)
    setExpandedPostId(postId)
    setMobilePreviewPostId(null)
    prevSectionBeforeComposerRef.current = section === 'composer' ? prevSectionBeforeComposerRef.current : section
    setSection('composer')
  }

  function closeComposer() {
    if (prevSectionBeforeComposerRef.current) {
      setSection(prevSectionBeforeComposerRef.current)
      prevSectionBeforeComposerRef.current = null
    } else if (section === 'composer') {
      setSection('calendar')
    }
  }

  async function duplicatePost(postId: string) {
    const targetPost = posts.find((post) => post.id === postId)
    if (!targetPost) return

    let accountContentOverride: Record<string, string> = {}
    if (api) {
      try {
        const res = await api.listPostVersions(targetPost.teamId, postId)
        for (const row of res.items ?? []) {
          if (row.content.trim() !== targetPost.content.trim()) {
            accountContentOverride[row.account_id] = row.content
          }
        }
      } catch {
        accountContentOverride = {}
      }
    }

    setEditorDraft({
      title: `${targetPost.title} (Copy)`,
      content: targetPost.content,
      scheduledAt: toInputDateTime(addHours(currentDate, 1)),
      targetAccountIds: [...(targetPost.targetAccountIds ?? [])],
      status: 'draft',
      accountContentOverride,
      mediaIds: targetPost.mediaIds ? [...targetPost.mediaIds] : [],
      mediaExcludeByAccount: targetPost.mediaExcludeByAccount ? { ...targetPost.mediaExcludeByAccount } : {},
    })
    setComposerMode('create')
    setEditingPostId(null)
    setMobilePreviewPostId(null)
    prevSectionBeforeComposerRef.current = section === 'composer' ? prevSectionBeforeComposerRef.current : section
    setSection('composer')
  }

  function connectBackend() {
    setActiveConnection({
      apiBaseUrl: settings.general.apiBaseUrl.trim(),
      bearerToken: settings.general.bearerToken.trim(),
    })
    setAuthTokenDraft(settings.general.bearerToken.trim())
    setStatusMessage(t('status.sessionSettingsApplied'))
  }

  async function handleToggleExternalPostMonitor(enabled: boolean) {
    if (!api || !selectedTeam || myRoleInSelectedTeam !== 'owner') {
      return
    }
    setExternalPostMonitorEnabled(enabled)
    await runAction(async () => {
      await api.upsertExternalPostMonitor(selectedTeam.id, { enabled })
    }, t('status.externalPostMonitorUpdated'))
  }

  async function handleImportOldPosts(accountIds: string[], limit: number, untilDate?: string): Promise<number> {
    if (!api || !selectedTeam) {
      throw new Error('No team selected')
    }
    const result = await api.importOldPosts(selectedTeam.id, {
      account_ids: accountIds,
      limit,
      until_date: untilDate,
    })
    return result.imported
  }

  async function handleSavePersonalAiSettings() {
    if (!api || !selectedTeam || !selectedTeam.isPersonal) {
      return
    }
    await runAction(async () => {
      await api.updateTeam(selectedTeam.id, {
        name: selectedTeam.name,
        description: selectedTeam.description ?? '',
        is_ai_enabled: teamAiEnabled,
      })
      if (teamAiEnabled && (teamAiApiKey.trim() || teamAiKeySet)) {
        const cfg = await api.upsertAIServiceConfig(selectedTeam.id, {
          provider: teamAiProvider,
          model: teamAiModel.trim(),
          base_url: teamAiBaseUrl.trim(),
          api_key: teamAiApiKey.trim(),
          description: 'team llm',
        })
        setTeamAiApiKey('')
        setTeamAiKeySet(Boolean(cfg.api_key_set))
      }
      await loadDashboard({ silent: true })
    }, t('status.teamSettingsUpdated'))
  }

  async function handleUpdateTeam() {
    if (!api || !selectedTeam || selectedTeam.isPersonal) {
      return
    }
    const name = teamSettingsName.trim()
    if (!name) {
      setError(t('status.teamNameRequired'))
      return
    }
    await runAction(async () => {
      await api.updateTeam(selectedTeam.id, {
        name,
        description: teamSettingsDescription.trim(),
        is_ai_enabled: teamAiEnabled,
      })
      await loadDashboard({ silent: true })
    }, t('status.teamSettingsUpdated'))
  }

  async function handleSaveTeamBrandColor(color: string) {
    if (!api || !selectedTeam || (!selectedTeam.isPersonal && myRoleInSelectedTeam !== 'owner')) {
      return
    }
    await runAction(async () => {
      // Name/description are resolved server-side from the current team when left empty.
      await api.updateTeam(selectedTeam.id, { name: '', description: '', brand_color: color })
      await loadDashboard({ silent: true })
    }, t('status.brandColorUpdated'))
  }

  async function handleSaveAiServiceConfig() {
    if (!api || !selectedTeam) return
    await runAction(async () => {
      const cfg = await api.upsertAIServiceConfig(selectedTeam.id, {
        provider: teamAiProvider,
        model: teamAiModel.trim(),
        base_url: teamAiBaseUrl.trim(),
        api_key: teamAiApiKey.trim(),
        description: 'team llm',
      })
      setTeamAiApiKey('')
      setTeamAiKeySet(Boolean(cfg.api_key_set))
      setStatusMessage(t('status.saved'))
    }, t('status.saved'))
  }

  useEffect(() => {
    if (!api || !selectedTeamId) return
    setTeamAiProvider('openai')
    setTeamAiModel('')
    setTeamAiBaseUrl('')
    setTeamAiApiKey('')
    setTeamAiKeySet(false)
    api.getAIServiceConfig(selectedTeamId)
      .then((cfg) => {
        setTeamAiProvider(cfg.provider || 'openai')
        setTeamAiModel(cfg.model ?? '')
        setTeamAiBaseUrl(cfg.base_url ?? '')
        setTeamAiKeySet(Boolean(cfg.api_key_set))
      })
      .catch(() => {})
  }, [api, selectedTeamId])

  async function handleLoadAdminAIEnabledTeams() {
    if (!api) {
      return
    }
    setAdminAIEnabledTeamsLoading(true)
    try {
      const res = await api.listAIEnabledTeams()
      setAdminAIEnabledTeams(res.items ?? [])
    } catch {
      setAdminAIEnabledTeams([])
    } finally {
      setAdminAIEnabledTeamsLoading(false)
    }
  }

  async function handleAddTeamMember() {
    if (!api || !selectedTeam || !addMemberUserId.trim()) {
      return
    }
    await runAction(async () => {
      await api.addTeamMember(selectedTeam.id, { user_id: addMemberUserId.trim(), role: 'editor' })
      setAddMemberUserId('')
      await loadDashboard({ silent: true })
    }, t('status.memberAdded'))
  }

  async function handleRemoveTeamMember(userId: string) {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      await api.removeTeamMember(selectedTeam.id, userId)
      await loadDashboard({ silent: true })
    }, t('status.memberRemoved'))
  }

  async function handleChangeTeamMemberRole(userId: string) {
    if (!api || !selectedTeam || !memberRoleEdits[userId]) {
      return
    }
    const role = memberRoleEdits[userId]
    if (!role) {
      return
    }
    await runAction(async () => {
      await api.addTeamMember(selectedTeam.id, { user_id: userId, role })
      await loadDashboard({ silent: true })
    }, t('status.memberAccessUpdated'))
  }

  async function handleSaveAdminProvider() {
    if (!api) {
      return
    }
    const d = adminProviderDraft
    const scopes = d.scopes.split(',').map((s) => s.trim()).filter(Boolean)
    const payload = {
      provider: d.provider,
      name: d.name.trim(),
      instance_url: d.instanceUrl.trim(),
      client_id: d.clientId.trim() || undefined,
      client_secret: d.clientSecret.trim() || undefined,
      scopes: scopes.length ? scopes : undefined,
      authorization_endpoint: d.authorizationEndpoint.trim() || undefined,
      token_endpoint: d.tokenEndpoint.trim() || undefined,
    }
    if (!payload.name || !payload.instance_url) {
      setError(t('status.providerNameUrlRequired'))
      return
    }
    await runAction(async () => {
      if (editingProviderId) {
        await api.updateProviderInstance(editingProviderId, {
          provider: payload.provider,
          name: payload.name,
          instance_url: payload.instance_url,
          client_id: payload.client_id,
          client_secret: payload.client_secret,
          scopes: payload.scopes,
          authorization_endpoint: payload.authorization_endpoint,
          token_endpoint: payload.token_endpoint,
        })
        setEditingProviderId(null)
      } else {
        await api.createProviderInstance(payload)
      }
      setAdminProviderDraft(defaultAdminProviderDraft())
      setShowAdminProviderAdvanced(false)
      await loadDashboard({ silent: true })
    }, editingProviderId ? t('status.providerUpdated') : t('status.providerRegistered'))
  }

  async function handleDeleteProviderInstance(instanceId: string) {
    if (!api) {
      return
    }
    const linked = accounts.filter((a) => a.providerInstanceId === instanceId).length
    if (linked > 0) {
      setError(t('status.disconnectAccountsBeforeRemove'))
      return
    }
    if (!window.confirm(t('status.confirmRemoveProviderInstance'))) {
      return
    }
    await runAction(async () => {
      await api.deleteProviderInstance(instanceId)
      if (editingProviderId === instanceId) {
        setEditingProviderId(null)
        setAdminProviderDraft(defaultAdminProviderDraft())
        setShowAdminProviderAdvanced(false)
      }
      await loadDashboard({ silent: true })
    }, t('status.providerInstanceRemoved'))
  }

  async function handleAdminSyncMetrics() {
    if (!api) {
      return
    }
    await runAction(async () => {
      const result = await api.adminSyncMetrics()
      const sync = await api.adminSyncStatus()
      setAdminSyncStatus(sync)
      await loadDashboard({ silent: true })
      setStatusMessage(result.message)
    }, t('status.metricsSyncStarted'))
  }

  async function handleDeleteTeamAccount(accountId: string) {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      await api.deleteAccount(selectedTeam.id, accountId)
      await loadDashboard({ silent: true })
    }, t('status.accountDisconnected'))
  }

  async function handleUpdateTeamAccount(
    accountId: string,
    payload: { name?: string; max_chars_override?: number; access_token?: string; refresh_token?: string },
  ) {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      await api.updateAccount(selectedTeam.id, accountId, payload)
      await loadDashboard({ silent: true })
    }, t('status.accountUpdated'))
  }

  async function handleConnectSocialAccount() {
    if (!api || !selectedTeam || !canEditTeamAccounts) {
      return
    }
    const d = accountDraft
    const hasInst = Boolean(d.providerInstanceId.trim())
    const instanceUrl = d.instanceUrl.trim()
    const basePayload = {
      provider: d.provider,
      ...(hasInst ? { provider_instance_id: d.providerInstanceId.trim() } : {}),
      ...(!hasInst && instanceUrl ? { instance_url: instanceUrl } : {}),
    }

    if (d.provider === 'mastodon') {
      if (!d.accessToken.trim()) {
        setError(t('status.mastodonTokenRequired'))
        return
      }
      if (!hasInst && !instanceUrl) {
        setError(t('status.selectInstanceOrUrl'))
        return
      }
      await runAction(async () => {
        await api.createAccount(selectedTeam.id, {
          ...basePayload,
          access_token: d.accessToken.trim(),
          refresh_token: d.refreshToken.trim() || undefined,
        })
        setAccountDraft(defaultAccountConnectDraft())
        await loadDashboard({ silent: true })
      }, t('status.mastodonConnected'))
      return
    }

    if (d.provider === 'friendica') {
      if (!d.accessToken.trim() || !d.identifier.trim()) {
        setError(t('status.friendicaCredentialsRequired'))
        return
      }
      if (!hasInst && !instanceUrl) {
        setError(t('status.selectInstanceOrFriendicaUrl'))
        return
      }
      await runAction(async () => {
        await api.createAccount(selectedTeam.id, {
          ...basePayload,
          username: d.identifier.trim(),
          access_token: d.accessToken.trim(),
        })
        setAccountDraft(defaultAccountConnectDraft())
        await loadDashboard({ silent: true })
      }, t('status.friendicaConnected'))
      return
    }

    if (d.provider === 'bluesky') {
      if (d.blueskyAuthMode === 'app_password') {
        if (!d.identifier.trim() || !d.appPassword.trim()) {
          setError(t('status.blueskyCredentialsRequired'))
          return
        }
        await runAction(async () => {
          await api.createAccount(selectedTeam.id, {
            ...basePayload,
            identifier: d.identifier.trim(),
            app_password: d.appPassword.trim(),
          })
          setAccountDraft(defaultAccountConnectDraft())
          await loadDashboard({ silent: true })
        }, t('status.blueskyConnected'))
        return
      }
      if (!d.accessToken.trim()) {
        setError(t('status.blueskyJwtRequired'))
        return
      }
      await runAction(async () => {
        await api.createAccount(selectedTeam.id, {
          ...basePayload,
          access_token: d.accessToken.trim(),
          refresh_token: d.refreshToken.trim() || undefined,
        })
        setAccountDraft(defaultAccountConnectDraft())
        await loadDashboard({ silent: true })
      }, t('status.blueskyConnected'))
    }
  }

  async function handleMastodonOAuthConnect() {
    if (!api || !selectedTeam || !canEditTeamAccounts || !accountDraft.providerInstanceId.trim()) {
      setError(t('status.selectMastodonForOAuth'))
      return
    }
    setSyncing(true)
    setError(null)
    setStatusMessage(null)
    try {
      const returnTo = new URL(window.location.href)
      returnTo.hash = ''
      returnTo.searchParams.set('section', 'accounts')
      const res = await api.startMastodonOAuth(selectedTeam.id, {
        provider_instance_id: accountDraft.providerInstanceId.trim(),
        return_to: returnTo.toString(),
      })
      window.location.assign(res.authorization_url)
    } catch (cause) {
      setSyncing(false)
      setError(translateApiError(cause instanceof Error ? cause.message : t('status.mastodonOAuthStartFailed'), t))
    }
  }

  async function handleCreateApiToken(payload: {
    name: string
    description?: string
    expires_at?: string
    scopes?: string[]
    team_id?: string
  }): Promise<string> {
    if (!api) {
      throw new Error(t('common.actionFailed'))
    }
    const res = await api.createMyApiToken(payload)
    const list = await api.listMyApiTokens()
    setApiTokens(list.items ?? [])
    setStatusMessage(t('status.apiTokenCreated'))
    return res.token
  }

  async function handleRemoveApiToken(tokenID: string, expired: boolean) {
    if (!api) {
      return
    }
    await runAction(async () => {
      await api.revokeMyApiToken(tokenID)
      const list = await api.listMyApiTokens()
      setApiTokens(list.items ?? [])
    }, expired ? t('status.tokenDeleted') : t('status.apiTokenRevoked'))
  }

  async function handleSavePost() {
    if (!api || !selectedTeam) {
      return
    }
    const defaultContent = editorDraft.content.trim()
    if (editorDraft.targetAccountIds.length === 0 || !editorDraft.scheduledAt || defaultContent.length === 0) {
      return
    }
    for (const id of editorDraft.targetAccountIds) {
      const acc = teamAccounts.find((a) => a.id === id)
      if (!acc) {
        continue
      }
      if (isAccountOverCharLimit(editorDraft, acc)) {
        return
      }
    }

    await runAction(async () => {
      const mediaExclude = buildMediaExcludePayload(editorDraft.mediaExcludeByAccount, editorDraft.targetAccountIds, editorDraft.mediaIds)
      const payload = {
        title: editorDraft.title.trim(),
        content: defaultContent,
        scheduled_at: new Date(editorDraft.scheduledAt).toISOString(),
        target_accounts: editorDraft.targetAccountIds,
        media_ids: editorDraft.mediaIds.length > 0 ? editorDraft.mediaIds : undefined,
        media_exclude_by_account: mediaExclude,
        account_content_override: accountContentOverrideForSave(editorDraft),
        draft: false,
      }

      if (composerMode === 'edit' && editTargetPost) {
        await api.updatePost(selectedTeam.id, editTargetPost.id, payload)
      } else {
        await api.createPost(selectedTeam.id, payload)
      }

      closeComposer()
      await loadDashboard({ silent: true })
    }, composerMode === 'edit' ? t('status.postUpdated') : t('status.postScheduled'))
  }

  async function handleSaveDraft() {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      const defaultContent = editorDraft.content
      const mediaExclude = buildMediaExcludePayload(editorDraft.mediaExcludeByAccount, editorDraft.targetAccountIds, editorDraft.mediaIds)
      const payload = {
        title: editorDraft.title.trim(),
        content: defaultContent.trim(),
        scheduled_at: new Date(editorDraft.scheduledAt || Date.now()).toISOString(),
        target_accounts: editorDraft.targetAccountIds,
        media_ids: editorDraft.mediaIds.length > 0 ? editorDraft.mediaIds : undefined,
        media_exclude_by_account: mediaExclude,
        account_content_override: accountContentOverrideForSave(editorDraft),
        draft: true,
      }

      if (composerMode === 'edit' && editTargetPost) {
        await api.updatePost(selectedTeam.id, editTargetPost.id, payload)
      } else {
        await api.createPost(selectedTeam.id, payload)
      }

      closeComposer()
      await loadDashboard({ silent: true })
    }, t('status.draftSaved'))
  }

  async function deletePost(postId: string) {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      await api.deletePost(selectedTeam.id, postId)
      setExpandedPostId((current) => (current === postId ? null : current))
      setEditingPostId((current) => (current === postId ? null : current))
      closeComposer()
      await loadDashboard({ silent: true })
    }, t('status.postDeleted'))
  }

  async function handleCalendarPostDrop(postId: string, targetDay: Date) {
    if (!api || !selectedTeam || !canEditScheduledPosts) {
      return
    }
    const post = posts.find((p) => p.id === postId)
    if (!post || post.status !== 'scheduled' || post.teamId !== selectedTeam.id || post.source === 'imported') {
      return
    }

    const prev = parseISO(post.scheduledAt)
    const newScheduled = set(targetDay, {
      hours: prev.getHours(),
      minutes: prev.getMinutes(),
      seconds: 0,
      milliseconds: 0,
    })
    if (newScheduled.getTime() === prev.getTime()) {
      return
    }

    const scheduledForTeam = teamPosts.filter((p) => p.status === 'scheduled')
    const updated: PostRecord = { ...post, scheduledAt: newScheduled.toISOString() }
    const resolved = resolveScheduleChange(scheduledForTeam, updated)

    const changed = resolved.filter((p) => {
      const original = posts.find((x) => x.id === p.id)
      return original && original.status === 'scheduled' && original.scheduledAt !== p.scheduledAt
    })
    if (changed.length === 0) {
      return
    }

    const resolvedById = new Map(resolved.map((p) => [p.id, p]))
    setPosts((current) =>
      current.map((p) => {
        const next = resolvedById.get(p.id)
        return next ? { ...p, scheduledAt: next.scheduledAt } : p
      }),
    )

    await runAction(async () => {
      for (const p of changed) {
        await api.updatePost(selectedTeam.id, p.id, {
          scheduled_at: p.scheduledAt,
        })
      }
      await loadDashboard({ silent: true })
    }, changed.length > 1 ? t('status.postsRescheduledOverlap') : t('status.postMovedCalendar'))
  }

  if (!authProbed && !authenticated) {
    return (
      <AuthShell theme={resolvedTheme}>
        <AuthPanel
          view="login"
          authStatus={null}
          authTokenDraft={authTokenDraft}
          authError={authError}
          authSubmitting={true}
          recoveryMode={recoveryMode}
          onViewChange={setAuthView}
          onTokenChange={setAuthTokenDraft}
          onSubmit={() => undefined}
          onStartOIDCLogin={() => undefined}
          onUseRecovery={enterRecoveryMode}
        />
      </AuthShell>
    )
  }

  if (!authenticated) {
    return (
      <AuthShell theme={resolvedTheme}>
        <AuthPanel
          view={authView}
          authStatus={authStatus}
          authTokenDraft={authTokenDraft}
          authError={authError}
          authSubmitting={authSubmitting}
          recoveryMode={recoveryMode}
          onViewChange={setAuthView}
          onTokenChange={setAuthTokenDraft}
          onSubmit={(mode) => void authenticateWithToken(mode)}
          onStartOIDCLogin={() => void startOIDCLogin()}
          onUseRecovery={enterRecoveryMode}
        />
      </AuthShell>
    )
  }

  return (
    <AppShell
      section={section}
      setSection={setSection}
      teams={teams}
      selectedTeamId={effectiveSelectedTeamId}
      onSelectTeam={setSelectedTeamId}
      user={principalUser}
      onSignOut={() => {
        void api.logout().catch(() => undefined)
        clearAuthenticatedState(t('status.signedOut'))
      }}
      openComposer={openCreateComposer}
      resolvedTheme={resolvedTheme}
      showPreviewColumn={showPreviewColumn}
      isComposer={isComposer}
      onRefresh={loadDashboard}
      pullToRefreshDisabled={pullToRefreshDisabled}
      reviewQueueCount={reviewQueueCount}
      reviewQueueOverdueCount={reviewQueueOverdueCount}
      previewColumn={
        section === 'composer' && !isMobile ? (
          <div className="preview-content">
            <ComposerPreviews
              draft={editorDraft}
              teamAccounts={teamAccounts}
              teamId={selectedTeam?.id}
              api={api}
              authHeader={activeConnection.bearerToken.trim() ? `Bearer ${activeConnection.bearerToken.trim()}` : undefined}
              theme={resolvedTheme}
              libraryItems={[]}
            />
          </div>
        ) : (
          <>
            <div className="preview-header">
              <div className="preview-header__top">
                <div>
                  <p className="eyebrow">{t('preview.livePreview')}</p>
                  <h3 data-testid="live-preview-title">
                    {selectedPost ? selectedPost.title || t('common.untitledPost') : t('preview.noPostSelected')}
                  </h3>
                </div>
                {selectedPost && canEditScheduledPosts ? (
                  <div className="preview-header__actions flex-row--center gap-2">
                    <button
                      type="button"
                      className="btn btn--ghost"
                      data-testid="preview-delete-button"
                      style={{ color: 'var(--danger)' }}
                      onClick={() => {
                        if (window.confirm(t('common.confirmDeletePost'))) {
                          void deletePost(selectedPost.id)
                        }
                      }}
                    >
                      <Trash2 size={16} />
                      <span>{t('common.delete')}</span>
                    </button>
                    {/* Archived posts can't be edited in place; offer a repost (duplicate to draft). */}
                    {selectedPost.status === 'posted' ? (
                      <button
                        type="button"
                        className="btn btn--ghost preview-header__edit"
                        data-testid="preview-duplicate-button"
                        onClick={() => void duplicatePost(selectedPost.id)}
                      >
                        <Copy size={16} />
                        <span>{t('common.duplicate')}</span>
                      </button>
                    ) : selectedPost.source !== 'imported' ? (
                      <button
                        type="button"
                        className="btn btn--ghost preview-header__edit"
                        data-testid="preview-edit-button"
                        onClick={() => void openEditor(selectedPost.id)}
                      >
                        <Edit size={16} />
                        <span>{t('common.edit')}</span>
                      </button>
                    ) : null}
                  </div>
                ) : null}
              </div>
            </div>
            <div className="preview-content">
              {selectedPost ? (
                sharedAccountLabels(selectedPost, accounts).map((account) => (
                    <SocialPreview
                      key={account.id}
                      account={account}
                      content={
                        postVersions.find((v) => v.postId === selectedPost.id && v.accountId === account.id)?.content ||
                        selectedPost.content
                      }
                      scheduledAt={selectedPost.scheduledAt}
                    theme={resolvedTheme}
                    publishedPostUrl={
                      selectedPost.status === 'posted' ? selectedPost.publishedLinks?.[account.id] : undefined
                    }
                    engagement={
                      selectedPost.status === 'posted'
                        ? engagementForAccount(previewPostMetrics, account.id)
                        : null
                    }
                  />
                ))
              ) : (
                <div className="empty-state">
                  <p className="hint">{t('common.selectPostPreview')}</p>
                </div>
              )}
            </div>
          </>
        )
      }
    >
      {section !== 'composer' ? (
        <header className="page-header">
          <div>
            <p className="eyebrow">{section === 'dashboard' ? t('eyebrow.workspace') : t('eyebrow.socialPublishing')}</p>
            <h1>{sectionHeading(t, section)}</h1>
          </div>

          <button
            type="button"
            className="btn btn--ghost page-header__toggle"
            onClick={() =>
              setSettings((current) => ({
                ...current,
                ui: { ...current.ui, colorScheme: resolvedTheme === 'dark' ? 'light' : 'dark' },
              }))
            }
          >
            {resolvedTheme === 'dark' ? <Sun size={20} /> : <Moon size={20} />}
          </button>
        </header>
      ) : null}

      {(error || statusMessage || loading) && dismissedNoticeKey !== noticeKey ? (
        <section className="glass-panel status-banner-panel">
          <div>
            {loading ? <span className="hint">{t('common.loadingData')}</span> : null}
            {statusMessage ? <span className="status-banner__success">{statusMessage}</span> : null}
            {error ? <span className="status-banner__error">{error}</span> : null}
          </div>
          <button className="btn btn--ghost btn--xs" onClick={() => setDismissedNoticeKey(noticeKey)}>
            <X size={16} />
          </button>
        </section>
      ) : null}

      <div className="pb-section">
        {section === 'composer' ? (
          <PostComposer
            open={true}
            mode={composerMode}
            isMobile={isMobile}
            theme={resolvedTheme}
            teamAccounts={teamAccounts}
            draft={editorDraft}
            setDraft={setEditorDraft}
            syncing={syncing}
            onSave={() => void handleSavePost()}
            onSaveDraft={() => void handleSaveDraft()}
            onClose={closeComposer}
            teamId={selectedTeam?.id}
            api={api ?? undefined}
            authHeader={activeConnection.bearerToken.trim() ? `Bearer ${activeConnection.bearerToken.trim()}` : undefined}
            onMediaUpload={
              api && selectedTeam
                ? async (file) => {
                    const item = await api.uploadTeamMediaToLibrary(selectedTeam.id, file)
                    return item.id
                  }
                : undefined
            }
            schedulingPreferences={selectedTeam?.schedulingPreferences}
            isAiEnabled={selectedTeam?.isAiEnabled ?? false}
            standalone
            previewColumnExternal={!isMobile}
          />
        ) : null}

        {isMobile && mobilePreviewPostId && section !== 'composer' && (() => {
          const previewPost = posts.find((p) => p.id === mobilePreviewPostId)
          if (!previewPost) return null
          return (
            <MobilePreviewOverlay
              post={previewPost}
              accounts={accounts}
              postVersions={postVersions}
              previewPostMetrics={previewPostMetrics}
              theme={resolvedTheme}
              onClose={() => setMobilePreviewPostId(null)}
              onDelete={(id) => void deletePost(id)}
              onEdit={(id) => void openEditor(id)}
              onDuplicate={(id) => void duplicatePost(id)}
            />
          )
        })()}

        {section === 'dashboard' && selectedTeam && api ? (
          <DashboardView
            teamName={selectedTeam.name}
            upcomingPosts={dashboardUpcomingPosts}
            accounts={teamAccounts}
            fetchSeries={fetchDashboardSeries}
            fetchGrowth={fetchDashboardGrowth}
            onOpenPreview={openPostPreview}
            onEditPost={openEditor}
            onOpenSchedule={() => setSection('calendar')}
            onOpenAccounts={() => setSection('accounts')}
          />
        ) : section === 'dashboard' ? (
          <p className="hint">{t('common.selectTeamDashboard')}</p>
        ) : null}

        {section === 'calendar' && (
          <ScheduleView
            upcomingPosts={upcomingPosts}
            deletePost={deletePost}
            onPreview={openPostPreview}
            accounts={accounts}
          />
        )}

        {section === 'contentCalendar' && (
          <ContentCalendarView
            isMobile={isMobile}
            contentCalendarMonth={contentCalendarMonth}
            setContentCalendarMonth={setContentCalendarMonth}
            contentCalendarCells={contentCalendarCells}
            plannedPostsForContentCalendar={plannedPostsForContentCalendar}
            canEditScheduledPosts={canEditScheduledPosts}
            calendarDragOverKey={calendarDragOverKey}
            setCalendarDragOverKey={setCalendarDragOverKey}
            deletePost={deletePost}
            onPreview={openPostPreview}
            accounts={accounts}
            handleCalendarPostDrop={handleCalendarPostDrop}
          />
        )}

        {section === 'archive' && (
          <ArchiveView
            archivedPosts={archivedPosts}
            deletePost={deletePost}
            onPreview={openPostPreview}
            accounts={accounts}
          />
        )}

        {section === 'automation' && selectedTeam && api ? (
          <AutomationView
            team={selectedTeam}
            api={api}
            accounts={teamAccounts}
            canEdit={canEditScheduledPosts}
            onStatus={(msg) => setStatusMessage(msg)}
            onPostsRefresh={() => loadDashboard({ silent: true })}
          />
        ) : null}

        {section === 'reviewQueue' && selectedTeam && api ? (
          <ReviewQueueView
            team={selectedTeam}
            accounts={teamAccounts}
            canEdit={canEditScheduledPosts}
            onEdit={openEditor}
            onPublishNow={async (item) => {
              await api.updatePost(selectedTeam.id, item.id, {
                title: item.title,
                content: item.content,
                scheduled_at: new Date().toISOString(),
                target_accounts: item.targetAccountIds,
                draft: false,
              })
              await loadDashboard({ silent: true })
              setStatusMessage(t('review.publishNow'))
            }}
            onSchedule={async (item, scheduledAt) => {
              await api.updatePost(selectedTeam.id, item.id, {
                title: item.title,
                content: item.content,
                scheduled_at: scheduledAt,
                target_accounts: item.targetAccountIds,
                draft: false,
              })
              await loadDashboard({ silent: true })
              setStatusMessage(t('review.schedule'))
            }}
            onDiscard={async (postId) => {
              await deletePost(postId)
            }}
          />
        ) : null}

        {section === 'analytics' && api && effectiveSelectedTeamId ? (
          <AnalyticsView
            teamId={effectiveSelectedTeamId}
            accounts={teamAccounts}
            fetchSummary={(opts) => api.getTeamAnalyticsSummary(effectiveSelectedTeamId, opts)}
            fetchPosts={(opts) => api.getTeamAnalyticsPosts(effectiveSelectedTeamId, opts)}
            fetchChart={(opts) => api.getTeamAnalyticsChart(effectiveSelectedTeamId, opts)}
            fetchAccountGrowth={(accountID, opts) => api.getTeamAccountGrowth(effectiveSelectedTeamId, accountID, opts)}
            fetchPostMetrics={(postID) => api.getPostAnalytics(effectiveSelectedTeamId, postID)}
            fetchHashtags={(opts) => api.getTeamHashtagPerformance(effectiveSelectedTeamId, opts)}
            fetchHeatmap={(opts) => api.getTeamEngagementHeatmap(effectiveSelectedTeamId, opts)}
          />
        ) : null}

        {section === 'mediaLibrary' && selectedTeam && api && (
          <MediaLibraryView
            teamId={selectedTeam.id}
            teamName={selectedTeam.name}
            api={api}
            onError={(msg) => setError(msg)}
          />
        )}

        {section === 'teams' && selectedTeam && (
          <TeamSettingsView
            api={api}
            selectedTeam={selectedTeam}
            myRoleInSelectedTeam={myRoleInSelectedTeam}
            principalUser={principalUser}
            directoryUsers={directoryUsers}
            syncing={syncing}
            teamSettingsName={teamSettingsName}
            setTeamSettingsName={setTeamSettingsName}
            teamSettingsDescription={teamSettingsDescription}
            setTeamSettingsDescription={setTeamSettingsDescription}
            teamAiEnabled={teamAiEnabled}
            setTeamAiEnabled={setTeamAiEnabled}
            teamAiProvider={teamAiProvider}
            setTeamAiProvider={setTeamAiProvider}
            teamAiModel={teamAiModel}
            setTeamAiModel={setTeamAiModel}
            teamAiApiKey={teamAiApiKey}
            setTeamAiApiKey={setTeamAiApiKey}
            teamAiBaseUrl={teamAiBaseUrl}
            setTeamAiBaseUrl={setTeamAiBaseUrl}
            teamAiKeySet={teamAiKeySet}
            teamBrandColor={teamBrandColor}
            setTeamBrandColor={setTeamBrandColor}
            externalPostMonitorEnabled={externalPostMonitorEnabled}
            onToggleExternalPostMonitor={(next) => void handleToggleExternalPostMonitor(next)}
            onOpenImportOldPosts={() => setImportOldPostsOpen(true)}
            memberRoleEdits={memberRoleEdits}
            setMemberRoleEdits={setMemberRoleEdits}
            addMemberUserId={addMemberUserId}
            setAddMemberUserId={setAddMemberUserId}
            onSavePersonalAiSettings={() => void handleSavePersonalAiSettings()}
            onUpdateTeam={() => void handleUpdateTeam()}
            onSaveAiServiceConfig={() => void handleSaveAiServiceConfig()}
            onSaveBrandColor={(color) => void handleSaveTeamBrandColor(color)}
            onAddTeamMember={() => void handleAddTeamMember()}
            onRemoveTeamMember={(userId) => void handleRemoveTeamMember(userId)}
            onChangeTeamMemberRole={(userId) => void handleChangeTeamMemberRole(userId)}
          />
        )}

        {section === 'accounts' && (
          <AccountsView
            selectedTeam={selectedTeam}
            teamAccounts={teamAccounts}
            canEditTeamAccounts={canEditTeamAccounts}
            syncing={syncing}
            accountDraft={accountDraft}
            setAccountDraft={setAccountDraft}
            instancesForAccountConnect={instancesForAccountConnect}
            onDeleteTeamAccount={handleDeleteTeamAccount}
            onConnectSocialAccount={handleConnectSocialAccount}
            onMastodonOAuthConnect={handleMastodonOAuthConnect}
            onUpdateTeamAccount={handleUpdateTeamAccount}
            editingAccountId={editingAccountId}
            setEditingAccountId={setEditingAccountId}
          />
        )}

        {section === 'settings' && (
          <SettingsView
            settings={settings}
            setSettings={setSettings}
            updateAPIBaseURL={updateAPIBaseURL}
            connectBackend={connectBackend}
            loadDashboard={loadDashboard}
            apiPresent={Boolean(api)}
            syncing={syncing}
            teams={teams.map((team) => ({ id: team.id, name: team.name }))}
            createApiToken={handleCreateApiToken}
            onRemoveApiToken={handleRemoveApiToken}
            apiTokens={apiTokens}
            apiTokensLoading={apiTokensLoading}
            currentTokenId={currentTokenId}
            showBrowserSession={authStatus?.appEnv === 'development' && principalUser?.globalRole === 'admin'}
          />
        )}

        {section === 'aiProfile' && selectedTeam && (
          <BrandProfileView team={selectedTeam} />
        )}

        {section === 'aiGenerator' && selectedTeam && (
          <AIGeneratorView
            team={selectedTeam}
            accounts={teamAccounts}
            onEditInComposer={openComposerFromGeneratedContent}
          />
        )}

        {section === 'aiCampaigns' && selectedTeam && (
          <CampaignFormatView team={selectedTeam} />
        )}

        {section === 'admin' && principalUser?.globalRole === 'admin' && (
          <AdminView
            api={api}
            adminMetrics={adminMetrics}
            adminMetricsLoading={adminMetricsLoading}
            adminRuntime={adminRuntime}
            adminSyncStatus={adminSyncStatus}
            adminSyncLoading={adminSyncLoading}
            onTriggerMetricsSync={handleAdminSyncMetrics}
            directoryUsers={directoryUsers}
            providerInstances={providerInstances}
            accounts={accounts}
            adminProviderDraft={adminProviderDraft}
            setAdminProviderDraft={setAdminProviderDraft}
            editingProviderId={editingProviderId}
            setEditingProviderId={setEditingProviderId}
            showAdminProviderAdvanced={showAdminProviderAdvanced}
            setShowAdminProviderAdvanced={setShowAdminProviderAdvanced}
            syncing={syncing}
            onSaveAdminProvider={handleSaveAdminProvider}
            onDeleteProviderInstance={handleDeleteProviderInstance}
            adminAIEnabledTeams={adminAIEnabledTeams}
            adminAIEnabledTeamsLoading={adminAIEnabledTeamsLoading}
            onLoadAdminAIEnabledTeams={handleLoadAdminAIEnabledTeams}
          />
        )}
      </div>

      {selectedTeam && (
        <ImportOldPostsDialog
          open={importOldPostsOpen}
          onOpenChange={setImportOldPostsOpen}
          accounts={teamAccounts}
          onImport={handleImportOldPosts}
        />
      )}

      {api && selectedTeam?.isAiEnabled && (
        <AIChatWidget
          key={selectedTeam.id}
          api={api}
          teamId={selectedTeam.id}
          onOpenInComposer={openComposerFromGeneratedContent}
          onApplyToComposer={(content) => {
            setEditorDraft((current) => ({ ...current, content }))
            prevSectionBeforeComposerRef.current = section === 'composer' ? prevSectionBeforeComposerRef.current : section
            setSection('composer')
          }}
        />
      )}

    </AppShell>
  )
}

export default App
