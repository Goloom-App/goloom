import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { addDays, format, parseISO, set, startOfDay, startOfMonth } from 'date-fns'

import { AuthPanel, AuthShell } from './components/auth/AuthViews'
import { PostComposer } from './components/Composer/PostComposer'
import { buildMediaExcludePayload, defaultEditorDraft, toInputDateTime } from './components/Composer/editorDraft'
import type { EditorDraftState } from './components/Composer/types'
import { SocialPreview } from './components/post/SocialPreview'
import { AppSidebar } from './components/Sidebar/AppSidebar'
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
import {
  ApiError,
  createApiClient,
  requestAuthStatus,
  requestStartOIDCLogin,
  type BackendAPIToken,
  type BackendAdminMetrics,
  type BackendPostMetric,
} from './api'
import { initialSettings } from './data'
import { Icon } from './icons'
import type { IconName } from './icons'
import { toAccountRecord, toAuthStatusRecord, toPostRecord, toProviderInstanceRecord, toRuntimeConfigRecord, toTeamMemberRecord, toTeamRecord, toUserRecord } from './mappers'
import { engagementForAccount } from './postMetrics'
import { postsForTeam, resolveScheduleChange, sharedAccountLabels } from './schedule'
import type { AccountRecord, AppSection, AuthStatusRecord, PostRecord, ProviderInstanceRecord, RuntimeConfigRecord, SettingsState, TeamRecord, TeamRole, UserRecord } from './types'

const SETTINGS_STORAGE_KEY = 'goloom-ui-settings'
/** Survives React Strict Mode remounts: hash is promoted here, then one reload applies the session. */
const OIDC_PENDING_SESSION_KEY = 'goloom.oidc.pending_session_v1'
const LAST_SECTION_STORAGE_KEY = 'goloom.last_section.v1'
const LAST_TEAM_STORAGE_KEY = 'goloom.last_team.v1'

const SECTION_HEADINGS: Record<AppSection, string> = {
  dashboard: 'Dashboard',
  calendar: 'Schedule',
  contentCalendar: 'Content calendar',
  archive: 'Archive',
  analytics: 'Analytics',
  mediaLibrary: 'Media library',
  teams: 'Team settings',
  accounts: 'Accounts',
  settings: 'Settings',
  admin: 'Admin',
}

const CONTENT_REFRESH_SECTIONS: AppSection[] = ['dashboard', 'calendar', 'archive', 'contentCalendar', 'analytics']

function App() {
  const [section, setSection] = useState<AppSection>(() => loadInitialSection())
  const [systemIsDark, setSystemIsDark] = useState(() =>
    typeof window !== 'undefined' && typeof window.matchMedia === 'function'
      ? window.matchMedia('(prefers-color-scheme: dark)').matches
      : true,
  )
  const [currentDate] = useState<Date>(new Date())
  const [contentCalendarMonth, setContentCalendarMonth] = useState(() => startOfMonth(new Date()))
  const [calendarDragOverKey, setCalendarDragOverKey] = useState<string | null>(null)
  const prevSectionRef = useRef<AppSection | null>(null)
  const [settings, setSettings] = useState<SettingsState>(() => loadStoredSettings())
  const [activeConnection, setActiveConnection] = useState(() => ({
    apiBaseUrl: loadStoredSettings().general.apiBaseUrl,
    bearerToken: loadStoredSettings().general.bearerToken,
  }))
  const [authStatus, setAuthStatus] = useState<AuthStatusRecord | null>(null)
  const [authStatusLoading, setAuthStatusLoading] = useState(true)
  const [authView, setAuthView] = useState<'bootstrap' | 'login'>('login')
  const [authTokenDraft, setAuthTokenDraft] = useState(() => loadStoredSettings().general.bearerToken)
  const [principalUser, setPrincipalUser] = useState<UserRecord | null>(null)
  const [authError, setAuthError] = useState<string | null>(null)
  const [authSubmitting, setAuthSubmitting] = useState(false)
  const [teams, setTeams] = useState<TeamRecord[]>([])
  const [accounts, setAccounts] = useState<AccountRecord[]>([])
  const [posts, setPosts] = useState<PostRecord[]>([])
  const [selectedTeamId, setSelectedTeamId] = useState(() => loadInitialTeamId())
  const [expandedPostId, setExpandedPostId] = useState<string | null>(null)
  const [archivePreviewMetrics, setArchivePreviewMetrics] = useState<BackendPostMetric[]>([])
  const [editingPostId, setEditingPostId] = useState<string | null>(null)
  const [composerMode, setComposerMode] = useState<'create' | 'edit'>('create')
  const [composerOpen, setComposerOpen] = useState(false)
  const [editorDraft, setEditorDraft] = useState<EditorDraftState>(() => defaultEditorDraft(currentDate, []))
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [dismissedNoticeKey, setDismissedNoticeKey] = useState<string | null>(null)
  const [newApiTokenExpiresYmd, setNewApiTokenExpiresYmd] = useState(() => format(addDays(new Date(), 90), 'yyyy-MM-dd'))
  const [directoryUsers, setDirectoryUsers] = useState<UserRecord[]>([])
  const [providerInstances, setProviderInstances] = useState<ProviderInstanceRecord[]>([])
  const [adminMetrics, setAdminMetrics] = useState<BackendAdminMetrics | null>(null)
  const [adminMetricsLoading, setAdminMetricsLoading] = useState(false)
  const [adminRuntime, setAdminRuntime] = useState<RuntimeConfigRecord | null>(null)
  const [apiTokens, setApiTokens] = useState<BackendAPIToken[]>([])
  const [apiTokensLoading, setApiTokensLoading] = useState(false)
  const [newTokenPlaintext, setNewTokenPlaintext] = useState<string | null>(null)
  const [teamSettingsName, setTeamSettingsName] = useState('')
  const [teamSettingsDescription, setTeamSettingsDescription] = useState('')
  const [addMemberUserId, setAddMemberUserId] = useState('')
  const [addMemberRole, setAddMemberRole] = useState<'editor' | 'viewer'>('editor')
  const [memberRoleEdits, setMemberRoleEdits] = useState<Record<string, TeamRole>>({})
  const [newApiTokenName, setNewApiTokenName] = useState('')
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState<'editor' | 'viewer'>('editor')
  const [adminProviderDraft, setAdminProviderDraft] = useState<AdminProviderDraft>(() => defaultAdminProviderDraft())
  const [editingProviderId, setEditingProviderId] = useState<string | null>(null)
  const [showAdminProviderAdvanced, setShowAdminProviderAdvanced] = useState(false)
  const [accountDraft, setAccountDraft] = useState<AccountConnectDraft>(() => defaultAccountConnectDraft())

  const api = useMemo(() => {
    const token = activeConnection.bearerToken.trim()
    if (!token) {
      return null
    }
    return createApiClient({ baseUrl: activeConnection.apiBaseUrl.trim(), token })
  }, [activeConnection.apiBaseUrl, activeConnection.bearerToken])

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

  const resolvedTheme = useMemo((): 'dark' | 'light' => {
    const scheme = settings.ui.colorScheme
    if (scheme === 'dark') {
      return 'dark'
    }
    if (scheme === 'light') {
      return 'light'
    }
    return systemIsDark ? 'dark' : 'light'
  }, [settings.ui.colorScheme, systemIsDark])

  useEffect(() => {
    if (settings.ui.colorScheme !== 'system') {
      return
    }
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    const syncTheme = (event?: MediaQueryListEvent) => {
      setSystemIsDark(event ? event.matches : mediaQuery.matches)
    }
    syncTheme()
    mediaQuery.addEventListener('change', syncTheme)
    return () => mediaQuery.removeEventListener('change', syncTheme)
  }, [settings.ui.colorScheme])

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
    const message = params.get('oauth_message') || (oauthStatus === 'success'
      ? `Connected ${provider} account`
      : `${provider} oauth failed`)

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
  }, [])

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
      setAuthError('Invalid OpenID Connect hand-off data.')
      return
    }
    const token = typeof payload.token === 'string' ? payload.token.trim() : ''
    const baseUrl = typeof payload.baseUrl === 'string' ? payload.baseUrl.trim() : ''
    if (!token) {
      setAuthError('OpenID Connect returned an empty token.')
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
        setStatusMessage('Signed in with OpenID Connect')
      } catch (cause) {
        if (cause instanceof ApiError && cause.status === 401) {
          setAuthError('OpenID Connect sign-in was rejected (check server time, IdP audience, and PUBLIC_BASE_URL / ALLOWED_ORIGINS).')
        } else {
          setAuthError(cause instanceof Error ? cause.message : 'Sign-in failed')
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
      setAuthError('Invalid sign-in response.')
      window.history.replaceState({}, document.title, `${window.location.pathname}${window.location.search}`)
      return
    }
    if (!token.trim()) {
      setAuthError('Invalid sign-in response.')
      window.history.replaceState({}, document.title, `${window.location.pathname}${window.location.search}`)
      return
    }
    const baseUrl = loadStoredSettings().general.apiBaseUrl.trim() || window.location.origin
    try {
      sessionStorage.setItem(OIDC_PENDING_SESSION_KEY, JSON.stringify({ token, baseUrl }))
    } catch {
      setAuthError('Could not store sign-in data (private browsing?).')
      return
    }
    window.history.replaceState({}, document.title, `${window.location.pathname}${window.location.search}`)
    window.location.reload()
  }, [])
  const clearAuthenticatedState = useCallback((message?: string) => {
    setActiveConnection((current) => ({ ...current, bearerToken: '' }))
    setSettings((current) => ({
      ...current,
      general: { ...current.general, bearerToken: '' },
    }))
    setAuthTokenDraft('')
    setPrincipalUser(null)
    setTeams([])
    setAccounts([])
    setPosts([])
    setSelectedTeamId('')
    setExpandedPostId(null)
    setEditingPostId(null)
    setComposerOpen(false)
    setLoading(false)
    if (message) {
      setStatusMessage(message)
    }
  }, [])

  async function authenticateWithToken(mode: 'bootstrap' | 'login') {
    const token = authTokenDraft.trim()
    if (!token) {
      setAuthError(mode === 'bootstrap' ? 'Enter the bootstrap token.' : 'Enter a bearer token.')
      return
    }

    setAuthSubmitting(true)
    setAuthError(null)
    setStatusMessage(null)

    try {
      const baseUrl = settings.general.apiBaseUrl.trim()
      const meResponse = await createApiClient({ baseUrl, token }).me()
      setActiveConnection({ apiBaseUrl: baseUrl, bearerToken: token })
      setSettings((current) => ({
        ...current,
        general: { ...current.general, apiBaseUrl: baseUrl, bearerToken: token },
      }))
      setPrincipalUser(toUserRecord(meResponse.user))
      setStatusMessage(mode === 'bootstrap' ? 'Bootstrap administrator signed in' : 'Signed in')
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        setAuthError(mode === 'bootstrap' ? 'The bootstrap token was rejected.' : 'The bearer token was rejected.')
      } else {
        setAuthError(cause instanceof Error ? cause.message : 'Sign-in failed')
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
      setAuthError(cause instanceof Error ? cause.message : 'Failed to start OpenID Connect login')
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
          const [membersResponse, accountsResponse, postsResponse] = await Promise.all([
            api.listTeamMembers(team.id),
            api.listAccounts(team.id),
            api.listPosts(team.id),
          ])

          const mappedAccounts = (accountsResponse.items ?? []).map((account) => toAccountRecord(account, mappedProviderInstances))
          const mappedPosts = (postsResponse.items ?? []).map(toPostRecord)
          const mappedMembers = (membersResponse.items ?? []).map(toTeamMemberRecord)

          return {
            team: toTeamRecord(team, mappedMembers, mappedAccounts.map((account) => account.id)),
            accounts: mappedAccounts,
            posts: mappedPosts,
          }
        }),
      )

      setTeams(teamPayloads.map((payload) => payload.team))
      setAccounts(teamPayloads.flatMap((payload) => payload.accounts))
      setPosts(teamPayloads.flatMap((payload) => payload.posts))
      setExpandedPostId((current) => (current && teamPayloads.flatMap((payload) => payload.posts).some((post) => post.id === current) ? current : null))
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        clearAuthenticatedState('Session expired')
        setAuthError('Session expired. Sign in again to continue.')
        return
      }
      if (!silent) {
        setError(cause instanceof Error ? cause.message : 'Failed to load dashboard data')
      }
    } finally {
      if (!silent) {
        setLoading(false)
      }
    }
  }, [api, clearAuthenticatedState])

  useEffect(() => {
    if (api) {
      const timer = window.setTimeout(() => {
        void loadDashboard()
      }, 0)
      return () => window.clearTimeout(timer)
    }
  }, [api, loadDashboard])

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
          setStatusMessage('Invitation accepted — you have been added to the team.')
          setError(null)
          params.delete('invite')
          const next = `${window.location.pathname}${params.toString() ? `?${params.toString()}` : ''}${window.location.hash}`
          window.history.replaceState({}, document.title, next)
          await loadDashboard({ silent: true })
        }
      } catch (cause) {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Failed to accept invitation')
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
    void Promise.all([api.adminMetrics(), api.runtimeConfig()])
      .then(([m, r]) => {
        if (!cancelled) {
          setAdminMetrics(m)
          setAdminRuntime(toRuntimeConfigRecord(r))
        }
      })
      .catch(() => {
        if (!cancelled) {
          setAdminMetrics(null)
          setAdminRuntime(null)
        }
      })
      .finally(() => {
        if (!cancelled) {
          setAdminMetricsLoading(false)
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

  const sidebarContentNav: { id: AppSection; label: string; icon: IconName }[] = useMemo(
    () => [
      { id: 'dashboard', label: 'Dashboard', icon: 'home' },
      { id: 'calendar', label: 'Schedule', icon: 'calendar' },
      { id: 'contentCalendar', label: 'Calendar', icon: 'calendarGrid' },
      { id: 'archive', label: 'Archive', icon: 'archive' },
      { id: 'analytics', label: 'Analytics', icon: 'chart' },
      { id: 'mediaLibrary', label: 'Media', icon: 'image' },
    ],
    [],
  )

  const sidebarWorkspaceNav: { id: AppSection; label: string; icon: IconName }[] = useMemo(
    () => [
      { id: 'teams', label: 'Team settings', icon: 'teams' },
      { id: 'accounts', label: 'Accounts', icon: 'share' },
    ],
    [],
  )

  const sidebarConfigNav: { id: AppSection; label: string; icon: IconName }[] = useMemo(() => {
    const items: { id: AppSection; label: string; icon: IconName }[] = [{ id: 'settings', label: 'Settings', icon: 'settings' }]
    if (principalUser?.globalRole === 'admin') {
      items.push({ id: 'admin', label: 'Admin', icon: 'admin' })
    }
    return items
  }, [principalUser])

  const effectiveSelectedTeamId = selectedTeamId || teams[0]?.id || ''
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

  const contentCalendarCells = useMemo(
    () => calendarCellsForMonth(contentCalendarMonth, plannedPostsForContentCalendar),
    [contentCalendarMonth, plannedPostsForContentCalendar],
  )

  const showPreviewColumn = section === 'calendar' || section === 'archive' || section === 'contentCalendar'

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
      setMemberRoleEdits({})
      return
    }
    setTeamSettingsName(selectedTeam.name)
    setTeamSettingsDescription(selectedTeam.description ?? '')
    setMemberRoleEdits(Object.fromEntries(selectedTeam.members.map((member) => [member.userId, member.role])))
  }, [selectedTeam])

  const instancesForAccountConnect = useMemo(
    () => providerInstances.filter((p) => p.provider === accountDraft.provider),
    [accountDraft.provider, providerInstances],
  )

  const selectedPost = useMemo(() => posts.find((post) => post.id === expandedPostId) ?? null, [expandedPostId, posts])
  const editTargetPost = useMemo(() => posts.find((post) => post.id === editingPostId) ?? null, [editingPostId, posts])

  useEffect(() => {
    if (!api || !selectedPost || selectedPost.status !== 'posted' || section !== 'archive') {
      setArchivePreviewMetrics([])
      return
    }
    let cancelled = false
    void api
      .getPostAnalytics(selectedPost.teamId, selectedPost.id)
      .then((r) => {
        if (!cancelled) {
          setArchivePreviewMetrics(r.items ?? [])
        }
      })
      .catch(() => {
        if (!cancelled) {
          setArchivePreviewMetrics([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [api, section, selectedPost])

  async function runAction(work: () => Promise<void>, successMessage: string) {
    setSyncing(true)
    setError(null)
    setStatusMessage(null)
    try {
      await work()
      setStatusMessage(successMessage)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Action failed')
    } finally {
      setSyncing(false)
    }
  }

  function openCreateComposer() {
    setComposerMode('create')
    setEditingPostId(null)
    setEditorDraft(defaultEditorDraft(currentDate, teamAccounts))
    setComposerOpen(true)
    setSection('dashboard')
  }

  async function openEditor(postId: string) {
    const targetPost = posts.find((post) => post.id === postId)
    if (!targetPost) {
      return
    }
    let accountContentOverride: Record<string, string> = {}
    if (api) {
      try {
        const res = await api.listPostVersions(targetPost.teamId, postId)
        for (const row of res.items ?? []) {
          accountContentOverride[row.account_id] = row.content
        }
      } catch {
        accountContentOverride = {}
      }
    }
    setEditorDraft({
      title: targetPost.title,
      content: targetPost.content,
      scheduledAt: toInputDateTime(parseISO(targetPost.scheduledAt)),
      targetAccountIds: targetPost.targetAccountIds,
      status: targetPost.status,
      accountContentOverride,
      mediaIds: targetPost.mediaIds ? [...targetPost.mediaIds] : [],
      mediaExcludeByAccount: targetPost.mediaExcludeByAccount ? { ...targetPost.mediaExcludeByAccount } : {},
    })
    setComposerMode('edit')
    setEditingPostId(postId)
    setExpandedPostId(postId)
    setComposerOpen(true)
    setSection(
      targetPost.status === 'posted'
        ? 'archive'
        : section === 'contentCalendar'
          ? 'contentCalendar'
          : section === 'dashboard'
            ? 'dashboard'
            : 'calendar',
    )
  }

  function closeComposer() {
    setComposerOpen(false)
  }

  function connectBackend() {
    setActiveConnection({
      apiBaseUrl: settings.general.apiBaseUrl.trim(),
      bearerToken: settings.general.bearerToken.trim(),
    })
    setAuthTokenDraft(settings.general.bearerToken.trim())
    setStatusMessage('Session settings applied')
  }

  function directoryUserLabel(userId: string) {
    const user = directoryUsers.find((u) => u.id === userId)
    return user ? `${user.name} · ${user.email}` : userId
  }

  async function handleCreateTeamFromSelector() {
    if (!api) {
      return
    }
    const newTeamName = (window.prompt('New team name') ?? '').trim()
    if (!newTeamName) {
      return
    }
    const newTeamDescription = (window.prompt('Team description (optional)') ?? '').trim()
    await runAction(async () => {
      const created = await api.createTeam({ name: newTeamName.trim(), description: newTeamDescription.trim() })
      setSelectedTeamId(created.id)
      await loadDashboard({ silent: true })
      setSection('teams')
    }, 'Team created')
  }

  async function handleUpdateTeam() {
    if (!api || !selectedTeam || selectedTeam.isPersonal) {
      return
    }
    const name = teamSettingsName.trim()
    if (!name) {
      setError('Team name is required.')
      return
    }
    await runAction(async () => {
      await api.updateTeam(selectedTeam.id, {
        name,
        description: teamSettingsDescription.trim(),
      })
      await loadDashboard({ silent: true })
    }, 'Team settings updated')
  }

  async function handleAddTeamMember() {
    if (!api || !selectedTeam || !addMemberUserId.trim()) {
      return
    }
    await runAction(async () => {
      await api.addTeamMember(selectedTeam.id, { user_id: addMemberUserId.trim(), role: addMemberRole })
      setAddMemberUserId('')
      await loadDashboard({ silent: true })
    }, 'Member added')
  }

  async function handleInviteToTeam() {
    if (!api || !selectedTeam || !inviteEmail.trim()) {
      return
    }
    setSyncing(true)
    setError(null)
    setStatusMessage(null)
    try {
      const res = await api.createTeamInvitation(selectedTeam.id, { email: inviteEmail.trim(), role: inviteRole })
      setInviteEmail('')
      await loadDashboard({ silent: true })
      setStatusMessage(`Invitation created. One-time acceptance token: ${res.token}`)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Invitation failed')
    } finally {
      setSyncing(false)
    }
  }

  async function handleRemoveTeamMember(userId: string) {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      await api.removeTeamMember(selectedTeam.id, userId)
      await loadDashboard({ silent: true })
    }, 'Member removed')
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
    }, 'Member access updated')
  }

  async function handleTransferOwnership(userId: string) {
    if (!api || !selectedTeam) {
      return
    }
    if (!window.confirm('Transfer ownership to this member? You can still stay owner unless you change your role.')) {
      return
    }
    await runAction(async () => {
      await api.addTeamMember(selectedTeam.id, { user_id: userId, role: 'owner' })
      await loadDashboard({ silent: true })
    }, 'Ownership transferred')
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
      setError('Provider name and instance URL are required.')
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
    }, editingProviderId ? 'Provider updated' : 'Provider registered')
  }

  async function handleDeleteProviderInstance(instanceId: string) {
    if (!api) {
      return
    }
    const linked = accounts.filter((a) => a.providerInstanceId === instanceId).length
    if (linked > 0) {
      setError('Disconnect all social accounts that use this instance before removing it.')
      return
    }
    if (!window.confirm('Remove this provider instance? It will no longer appear when teams connect accounts.')) {
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
    }, 'Provider instance removed')
  }

  async function handleDeleteTeamAccount(accountId: string) {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      await api.deleteAccount(selectedTeam.id, accountId)
      await loadDashboard({ silent: true })
    }, 'Account disconnected')
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
        setError('Mastodon access token is required for manual connection.')
        return
      }
      if (!hasInst && !instanceUrl) {
        setError('Select a registered instance or enter the instance URL.')
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
      }, 'Mastodon account connected')
      return
    }

    if (d.provider === 'friendica') {
      if (!d.accessToken.trim() || !d.identifier.trim()) {
        setError('Friendica username and access token are required.')
        return
      }
      if (!hasInst && !instanceUrl) {
        setError('Select a registered instance or enter the Friendica base URL.')
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
      }, 'Friendica account connected')
      return
    }

    if (d.provider === 'bluesky') {
      if (d.blueskyAuthMode === 'app_password') {
        if (!d.identifier.trim() || !d.appPassword.trim()) {
          setError('Bluesky handle and app password are required.')
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
        }, 'Bluesky account connected')
        return
      }
      if (!d.accessToken.trim()) {
        setError('Bluesky access token (JWT) is required.')
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
      }, 'Bluesky account connected')
    }
  }

  async function handleMastodonOAuthConnect() {
    if (!api || !selectedTeam || !canEditTeamAccounts || !accountDraft.providerInstanceId.trim()) {
      setError('Select a registered Mastodon instance for browser login.')
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
      setError(cause instanceof Error ? cause.message : 'Mastodon OAuth failed to start')
    }
  }

  async function handleCreateApiToken() {
    if (!api || !newApiTokenName.trim()) {
      return
    }
    const expEnd = new Date(`${newApiTokenExpiresYmd}T23:59:59.999Z`)
    if (!newApiTokenExpiresYmd.trim() || Number.isNaN(expEnd.getTime()) || expEnd.getTime() <= Date.now()) {
      setError('Choose an expiry date in the future (UTC end of day).')
      return
    }
    await runAction(async () => {
      const expiresAt = new Date(`${newApiTokenExpiresYmd}T23:59:59.999Z`).toISOString()
      const res = await api.createMyApiToken({ name: newApiTokenName.trim(), expires_at: expiresAt })
      setNewTokenPlaintext(res.token)
      setNewApiTokenName('')
      setNewApiTokenExpiresYmd(format(addDays(new Date(), 90), 'yyyy-MM-dd'))
      const list = await api.listMyApiTokens()
      setApiTokens(list.items ?? [])
    }, 'API token created. Copy the secret below; it is not stored in plain text.')
  }

  async function handleRevokeApiToken(tokenID: string) {
    if (!api) {
      return
    }
    await runAction(async () => {
      await api.revokeMyApiToken(tokenID)
      const list = await api.listMyApiTokens()
      setApiTokens(list.items ?? [])
    }, 'API token revoked')
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
      const body = (editorDraft.accountContentOverride[id] ?? editorDraft.content).trim()
      if (acc.maxChars > 0 && body.length > acc.maxChars) {
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
        draft: false,
      }

      let savedPostId: string
      if (composerMode === 'edit' && editTargetPost) {
        await api.updatePost(selectedTeam.id, editTargetPost.id, payload)
        savedPostId = editTargetPost.id
      } else {
        const created = await api.createPost(selectedTeam.id, payload)
        savedPostId = created.id
      }

      const versions = editorDraft.targetAccountIds.map((aid) => {
        const override = (editorDraft.accountContentOverride[aid] ?? '').trim()
        const content = override && override !== defaultContent ? override : ''
        return { account_id: aid, content }
      })
      await api.patchPostVersions(selectedTeam.id, savedPostId, { versions })

      setComposerOpen(false)
      await loadDashboard({ silent: true })
    }, composerMode === 'edit' ? 'Post updated' : 'Post scheduled')
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
        draft: true,
      }

      let savedPostId: string
      if (composerMode === 'edit' && editTargetPost) {
        await api.updatePost(selectedTeam.id, editTargetPost.id, payload)
        savedPostId = editTargetPost.id
      } else {
        const created = await api.createPost(selectedTeam.id, payload)
        savedPostId = created.id
      }

      const versions = editorDraft.targetAccountIds.map((aid) => {
        const override = (editorDraft.accountContentOverride[aid] ?? '').trim()
        const content = override && override !== defaultContent.trim() ? override : ''
        return { account_id: aid, content }
      })
      if (versions.some((v) => v.content.length > 0)) {
        await api.patchPostVersions(selectedTeam.id, savedPostId, { versions })
      }

      setComposerOpen(false)
      await loadDashboard({ silent: true })
    }, 'Draft saved')
  }

  async function deletePost(postId: string) {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      await api.deletePost(selectedTeam.id, postId)
      setExpandedPostId((current) => (current === postId ? null : current))
      setEditingPostId((current) => (current === postId ? null : current))
      setComposerOpen(false)
      await loadDashboard({ silent: true })
    }, 'Post deleted')
  }

  async function handleCalendarPostDrop(postId: string, targetDay: Date) {
    if (!api || !selectedTeam || !canEditScheduledPosts) {
      return
    }
    const post = posts.find((p) => p.id === postId)
    if (!post || post.status !== 'scheduled' || post.teamId !== selectedTeam.id) {
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

    await runAction(async () => {
      for (const p of changed) {
        const original = posts.find((x) => x.id === p.id)
        if (!original) {
          continue
        }
        await api.updatePost(selectedTeam.id, p.id, {
          title: original.title.trim(),
          content: original.content.trim(),
          scheduled_at: p.scheduledAt,
          target_accounts: original.targetAccountIds,
        })
      }
      await loadDashboard({ silent: true })
    }, changed.length > 1 ? 'Posts rescheduled to avoid overlap' : 'Post moved on calendar')
  }

  if (authStatusLoading && !activeConnection.bearerToken.trim()) {
    return (
      <AuthShell theme={resolvedTheme}>
        <AuthPanel
          view="login"
          authStatus={null}
          authTokenDraft={authTokenDraft}
          authError={authError}
          authSubmitting={true}
          onViewChange={setAuthView}
          onTokenChange={setAuthTokenDraft}
          onSubmit={() => undefined}
          onStartOIDCLogin={() => undefined}
        />
      </AuthShell>
    )
  }

  if (!activeConnection.bearerToken.trim()) {
    return (
      <AuthShell theme={resolvedTheme}>
        <AuthPanel
          view={authView}
          authStatus={authStatus}
          authTokenDraft={authTokenDraft}
          authError={authError}
          authSubmitting={authSubmitting}
          onViewChange={setAuthView}
          onTokenChange={setAuthTokenDraft}
          onSubmit={(mode) => void authenticateWithToken(mode)}
          onStartOIDCLogin={() => void startOIDCLogin()}
        />
      </AuthShell>
    )
  }

  return (
    <div className={`app-shell ${showPreviewColumn ? 'app-shell--triple' : 'app-shell--double'}`} data-theme={resolvedTheme}>
      <AppSidebar
        section={section}
        setSection={setSection}
        sidebarContentNav={sidebarContentNav}
        sidebarWorkspaceNav={sidebarWorkspaceNav}
        sidebarConfigNav={sidebarConfigNav}
        teams={teams}
        principalUser={principalUser}
        effectiveSelectedTeamId={effectiveSelectedTeamId}
        selectedTeam={selectedTeam}
        syncing={syncing}
        selectedTeamPresent={Boolean(selectedTeam)}
        onSelectTeam={setSelectedTeamId}
        onCreateTeamFromSelector={() => void handleCreateTeamFromSelector()}
        onCreatePost={openCreateComposer}
        onSignOut={() => clearAuthenticatedState('Signed out')}
      />

      <main className="app-main">
          <button
            type="button"
            className="theme-floating-toggle"
            onClick={() =>
              setSettings((current) => ({
                ...current,
                ui: { ...current.ui, colorScheme: resolvedTheme === 'dark' ? 'light' : 'dark' },
              }))
            }
            title={resolvedTheme === 'dark' ? 'Light mode' : 'Dark mode'}
            aria-label={resolvedTheme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
          >
            <Icon name={resolvedTheme === 'dark' ? 'sun' : 'moon'} className="inline-icon" />
          </button>
          <header className="page-header">
            <div>
              <p className="eyebrow">{section === 'dashboard' ? 'Workspace' : 'Social publishing'}</p>
              <h1>{SECTION_HEADINGS[section]}</h1>
            </div>

            <div className="inline-cluster page-header__toolbar" />
          </header>

          {(error || statusMessage || loading) && dismissedNoticeKey !== noticeKey ? (
            <section className="glass-panel status-banner-panel">
              <div className="status-banner-panel__body">
                {loading ? <span className="hint">Loading backend data…</span> : null}
                {statusMessage ? <span className="status-banner__success">{statusMessage}</span> : null}
                {error ? <span className="status-banner__error">{error}</span> : null}
              </div>
              <button
                type="button"
                className="status-banner-panel__close"
                onClick={() => setDismissedNoticeKey(noticeKey)}
                aria-label="Dismiss notification"
                title="Dismiss"
              >
                <Icon name="close" className="inline-icon" />
              </button>
            </section>
          ) : null}

          {section === 'dashboard' && selectedTeam && api ? (
            <DashboardView
              teamName={selectedTeam.name}
              upcomingPosts={dashboardUpcomingPosts}
              accounts={teamAccounts}
              fetchSeries={(metric) => api.getTeamAnalyticsChart(selectedTeam.id, { metric, days: 7 })}
              onOpenPost={(id) => void openEditor(id)}
              onOpenSchedule={() => setSection('calendar')}
              onOpenAccounts={() => setSection('accounts')}
            />
          ) : section === 'dashboard' ? (
            <p className="hint">Select a team and connect to the API to load the dashboard.</p>
          ) : null}

          {section === 'calendar' && (
            <ScheduleView
              upcomingPosts={upcomingPosts}
              expandedPostId={expandedPostId}
              setExpandedPostId={setExpandedPostId}
              openEditor={(id) => void openEditor(id)}
              deletePost={deletePost}
              accounts={accounts}
            />
          )}

          {section === 'contentCalendar' && (
            <ContentCalendarView
              contentCalendarMonth={contentCalendarMonth}
              setContentCalendarMonth={setContentCalendarMonth}
              contentCalendarCells={contentCalendarCells}
              plannedPostsForContentCalendar={plannedPostsForContentCalendar}
              canEditScheduledPosts={canEditScheduledPosts}
              calendarDragOverKey={calendarDragOverKey}
              setCalendarDragOverKey={setCalendarDragOverKey}
              setExpandedPostId={setExpandedPostId}
              openEditor={(id) => void openEditor(id)}
              handleCalendarPostDrop={handleCalendarPostDrop}
            />
          )}

          {section === 'archive' && (
            <ArchiveView
              archivedPosts={archivedPosts}
              expandedPostId={expandedPostId}
              setExpandedPostId={setExpandedPostId}
              openEditor={(id) => void openEditor(id)}
              deletePost={deletePost}
              accounts={accounts}
            />
          )}

          {section === 'analytics' && api && effectiveSelectedTeamId ? (
            <AnalyticsView
              teamId={effectiveSelectedTeamId}
              accounts={teamAccounts}
              fetchSummary={(opts) => api.getTeamAnalyticsSummary(effectiveSelectedTeamId, opts)}
              fetchPosts={(opts) => api.getTeamAnalyticsPosts(effectiveSelectedTeamId, opts)}
              fetchChart={(opts) => api.getTeamAnalyticsChart(effectiveSelectedTeamId, opts)}
              fetchAccountGrowth={(accountID, opts) => api.getTeamAccountGrowth(effectiveSelectedTeamId, accountID, opts)}
            />
          ) : section === 'analytics' ? (
            <p className="hint">Connect to the API and select a team to view analytics.</p>
          ) : null}

          {section === 'mediaLibrary' && selectedTeam && api ? (
            <MediaLibraryView
              teamId={selectedTeam.id}
              teamName={selectedTeam.name}
              api={api}
              onError={(msg) => setError(msg)}
            />
          ) : section === 'mediaLibrary' ? (
            <p className="hint">Select a team to browse the media workspace.</p>
          ) : null}

          {section === 'teams' && (
            <div className="glass-panel">
              <h2 className="section-card__title">Team settings</h2>
              <p className="hint">Manage members, update access, and transfer ownership for selected workspace.</p>

              <h3 className="subsection-title">Team settings · {selectedTeam?.name ?? '—'}</h3>
              {!selectedTeam ? (
                <p className="hint">Select a workspace from the sidebar to manage members.</p>
              ) : selectedTeam.isPersonal ? (
                <p className="hint">Personal workspace has no shared members.</p>
              ) : myRoleInSelectedTeam === 'owner' ? (
                <>
                  <h4 className="subsection-title" style={{ marginTop: '1rem' }}>General</h4>
                  <div className="inline-cluster" style={{ alignItems: 'flex-end', flexWrap: 'wrap' }}>
                    <label className="field" style={{ minWidth: '14rem', flex: '1 1 14rem' }}>
                      <span>Team name</span>
                      <input value={teamSettingsName} onChange={(event) => setTeamSettingsName(event.target.value)} />
                    </label>
                    <label className="field" style={{ minWidth: '16rem', flex: '1 1 16rem' }}>
                      <span>Description</span>
                      <input value={teamSettingsDescription} onChange={(event) => setTeamSettingsDescription(event.target.value)} placeholder="Optional" />
                    </label>
                    <button
                      type="button"
                      className="button button--primary"
                      onClick={() => void handleUpdateTeam()}
                      disabled={syncing || !teamSettingsName.trim()}
                    >
                      Save
                    </button>
                  </div>

                  <h4 className="subsection-title" style={{ marginTop: '1rem' }}>Members & access</h4>
                  <ul className="member-list">
                    {selectedTeam.members.map((m) => (
                      <li key={m.userId} className="member-list__row">
                        <div>
                          <strong>{directoryUserLabel(m.userId)}</strong>
                          <span className="member-list__role">{m.role}</span>
                        </div>
                        <div className="inline-cluster" style={{ flexWrap: 'wrap', justifyContent: 'flex-end' }}>
                          <select
                            value={memberRoleEdits[m.userId] ?? m.role}
                            onChange={(event) =>
                              setMemberRoleEdits((current) => ({ ...current, [m.userId]: event.target.value as TeamRole }))
                            }
                          >
                            <option value="owner">Owner</option>
                            <option value="editor">Editor</option>
                            <option value="viewer">Viewer</option>
                          </select>
                          <button
                            type="button"
                            className="button button--secondary"
                            onClick={() => void handleChangeTeamMemberRole(m.userId)}
                            disabled={syncing || (memberRoleEdits[m.userId] ?? m.role) === m.role}
                          >
                            Apply access
                          </button>
                          {m.userId !== principalUser?.id ? (
                            <>
                              <button
                                type="button"
                                className="button button--secondary"
                                onClick={() => void handleTransferOwnership(m.userId)}
                                disabled={syncing || m.role === 'owner'}
                              >
                                Make owner
                              </button>
                              <button type="button" className="button button--secondary" onClick={() => void handleRemoveTeamMember(m.userId)} disabled={syncing}>
                                Remove
                              </button>
                            </>
                          ) : null}
                        </div>
                      </li>
                    ))}
                  </ul>

                  <h4 className="subsection-title" style={{ marginTop: '1rem' }}>Add member</h4>
                  <p className="hint">Grant access to an existing user from the directory.</p>
                  <div className="inline-cluster" style={{ flexWrap: 'wrap' }}>
                    <select value={addMemberUserId} onChange={(event) => setAddMemberUserId(event.target.value)}>
                      <option value="">Select user…</option>
                      {directoryUsers
                        .filter((u) => !selectedTeam.members.some((m) => m.userId === u.id))
                        .map((u) => (
                          <option key={u.id} value={u.id}>{u.name} ({u.email})</option>
                        ))}
                    </select>
                    <select value={addMemberRole} onChange={(event) => setAddMemberRole(event.target.value as 'editor' | 'viewer')}>
                      <option value="editor">Editor</option>
                      <option value="viewer">Viewer</option>
                    </select>
                    <button type="button" className="button button--primary" onClick={() => void handleAddTeamMember()} disabled={syncing || !addMemberUserId}>
                      Add
                    </button>
                  </div>

                  <h4 className="subsection-title" style={{ marginTop: '1rem' }}>Invite by email</h4>
                  <p className="hint">Generates a one-time token the invitee uses with the invitation link flow.</p>
                  <div className="inline-cluster" style={{ flexWrap: 'wrap' }}>
                    <input type="email" value={inviteEmail} onChange={(event) => setInviteEmail(event.target.value)} placeholder="colleague@company.com" />
                    <select value={inviteRole} onChange={(event) => setInviteRole(event.target.value as 'editor' | 'viewer')}>
                      <option value="editor">Editor</option>
                      <option value="viewer">Viewer</option>
                    </select>
                    <button type="button" className="button button--secondary" onClick={() => void handleInviteToTeam()} disabled={syncing || !inviteEmail.trim()}>
                      Create invitation
                    </button>
                  </div>
                </>
              ) : myRoleInSelectedTeam ? (
                <>
                  <ul className="member-list">
                    {selectedTeam.members.map((m) => (
                      <li key={m.userId} className="member-list__row">
                        <div>
                          <strong>{directoryUserLabel(m.userId)}</strong>
                          <span className="member-list__role">{m.role}</span>
                        </div>
                      </li>
                    ))}
                  </ul>
                  <p className="hint">Only the team owner can add or remove people.</p>
                </>
              ) : (
                <p className="hint">You do not have access to this team.</p>
              )}
            </div>
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
              newTokenPlaintext={newTokenPlaintext}
              setNewTokenPlaintext={setNewTokenPlaintext}
              newApiTokenName={newApiTokenName}
              setNewApiTokenName={setNewApiTokenName}
              newApiTokenExpiresYmd={newApiTokenExpiresYmd}
              setNewApiTokenExpiresYmd={setNewApiTokenExpiresYmd}
              onCreateApiToken={handleCreateApiToken}
              onRevokeApiToken={handleRevokeApiToken}
              apiTokens={apiTokens}
              apiTokensLoading={apiTokensLoading}
            />
          )}

          {section === 'admin' && principalUser?.globalRole === 'admin' ? (
            <AdminView
              adminMetrics={adminMetrics}
              adminMetricsLoading={adminMetricsLoading}
              adminRuntime={adminRuntime}
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
            />
          ) : section === 'admin' ? (
            <div className="glass-panel">
              <p className="hint">Administrator access is required for this section.</p>
            </div>
          ) : null}
        </main>

        {showPreviewColumn ? (
        <aside className="preview-column">
          <div className="preview-header">
            <div className="preview-header__top">
              <div>
                <p className="eyebrow">Live Preview</p>
                <h3>{selectedPost ? selectedPost.title || 'Untitled Post' : 'No post selected'}</h3>
              </div>
              {selectedPost &&
              (selectedPost.status === 'scheduled' || selectedPost.status === 'draft') &&
              canEditScheduledPosts ? (
                <button type="button" className="button button--secondary preview-header__edit" onClick={() => openEditor(selectedPost.id)}>
                  <Icon name="edit" className="inline-icon" />
                  <span>Edit</span>
                </button>
              ) : null}
            </div>
          </div>
          <div className="preview-content">
            {selectedPost ? (
              sharedAccountLabels(selectedPost, accounts).map((account) => (
                <SocialPreview
                  key={account.id}
                  account={account}
                  content={selectedPost.content}
                  scheduledAt={selectedPost.scheduledAt}
                  theme={resolvedTheme}
                  publishedPostUrl={
                    selectedPost.status === 'posted' ? selectedPost.publishedLinks?.[account.id] : undefined
                  }
                  engagement={
                    section === 'archive' && selectedPost.status === 'posted'
                      ? engagementForAccount(archivePreviewMetrics, account.id)
                      : null
                  }
                />
              ))
            ) : (
              <div className="empty-state">
                <p className="hint">Select a post from the timeline to see how it will look on different platforms.</p>
              </div>
            )}
          </div>
        </aside>
        ) : null}

      <PostComposer
        open={composerOpen}
        mode={composerMode}
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
        authHeader={
          activeConnection.bearerToken.trim()
            ? `Bearer ${activeConnection.bearerToken.trim()}`
            : undefined
        }
        onMediaUpload={
          api && selectedTeam
            ? async (file) => {
                const item = await api.uploadTeamMediaToLibrary(selectedTeam.id, file)
                return item.id
              }
            : undefined
        }
      />

      <nav className="mobile-nav">
        <button
          type="button"
          className={section === 'dashboard' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'}
          onClick={() => setSection('dashboard')}
          aria-label="Dashboard"
        >
          <Icon name="home" className="inline-icon" />
        </button>
        <button type="button" className={section === 'calendar' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'} onClick={() => setSection('calendar')}>
          <Icon name="calendar" className="inline-icon" />
        </button>
        <button
          type="button"
          className={section === 'contentCalendar' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'}
          onClick={() => setSection('contentCalendar')}
          aria-label="Content calendar"
        >
          <Icon name="calendarGrid" className="inline-icon" />
        </button>
        <button type="button" className="mobile-nav__item mobile-nav__item--primary" onClick={openCreateComposer}>
          <Icon name="edit" className="inline-icon" />
        </button>
        <button type="button" className={section === 'archive' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'} onClick={() => setSection('archive')}>
          <Icon name="archive" className="inline-icon" />
        </button>
        <button
          type="button"
          className={section === 'analytics' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'}
          onClick={() => setSection('analytics')}
          aria-label="Analytics"
        >
          <Icon name="chart" className="inline-icon" />
        </button>
        <button
          type="button"
          className={section === 'mediaLibrary' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'}
          onClick={() => setSection('mediaLibrary')}
          aria-label="Media library"
        >
          <Icon name="image" className="inline-icon" />
        </button>
        <button type="button" className={section === 'accounts' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'} onClick={() => setSection('accounts')}>
          <Icon name="share" className="inline-icon" />
        </button>
        <button type="button" className={section === 'settings' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'} onClick={() => setSection('settings')}>
          <Icon name="settings" className="inline-icon" />
        </button>
      </nav>
    </div>
  )
}

function loadStoredSettings(): SettingsState {
  if (typeof window === 'undefined') {
    return initialSettings
  }
  const raw = window.localStorage.getItem(SETTINGS_STORAGE_KEY)
  if (!raw) {
    return initialSettings
  }
  try {
    const parsed = JSON.parse(raw) as Partial<SettingsState>
    return {
      ...initialSettings,
      ...parsed,
      ui: { ...initialSettings.ui, ...parsed.ui },
      general: { ...initialSettings.general, ...parsed.general },
      oidc: { ...initialSettings.oidc, ...parsed.oidc },
      security: { ...initialSettings.security, ...parsed.security },
      scheduler: { ...initialSettings.scheduler, ...parsed.scheduler },
      providers: { ...initialSettings.providers, ...parsed.providers },
    }
  } catch {
    return initialSettings
  }
}

function writeStoredSettings(settings: SettingsState) {
  if (typeof window === 'undefined') {
    return
  }
  window.localStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(settings))
}

function isAppSection(value: string): value is AppSection {
  return value in SECTION_HEADINGS
}

function loadInitialSection(): AppSection {
  if (typeof window === 'undefined') {
    return 'dashboard'
  }
  const sectionFromQuery = new URLSearchParams(window.location.search).get('section')?.trim() ?? ''
  if (sectionFromQuery && isAppSection(sectionFromQuery)) {
    return sectionFromQuery
  }
  const stored = window.localStorage.getItem(LAST_SECTION_STORAGE_KEY)?.trim() ?? ''
  if (stored && isAppSection(stored)) {
    return stored
  }
  return 'dashboard'
}

function loadInitialTeamId(): string {
  if (typeof window === 'undefined') {
    return ''
  }
  const teamFromQuery = new URLSearchParams(window.location.search).get('team')?.trim() ?? ''
  if (teamFromQuery) {
    return teamFromQuery
  }
  return window.localStorage.getItem(LAST_TEAM_STORAGE_KEY)?.trim() ?? ''
}

export default App
