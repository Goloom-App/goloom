import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { addDays, format, isValid, parseISO, set, startOfDay, startOfMonth } from 'date-fns'

import { AnalyticsView } from './views/Analytics/AnalyticsView'
import { ArchiveView } from './views/calendar/ArchiveView'
import { calendarCellsForMonth } from './views/calendar/calendarUtils'
import { ContentCalendarView } from './views/calendar/ContentCalendarView'
import { ScheduleView } from './views/calendar/ScheduleView'
import { ApiError, createApiClient, requestAuthStatus, requestStartOIDCLogin, type BackendAPIToken, type BackendAdminMetrics } from './api'
import { initialSettings } from './data'
import { AppSidebar } from './components/Sidebar/AppSidebar'
import { defaultEditorDraft, toInputDateTime } from './components/Composer/editorDraft'
import type { EditorDraftState } from './components/Composer/types'
import { DestinationAvatar } from './components/post/DestinationAvatar'
import { SocialPreview } from './components/post/SocialPreview'
import { PostComposer } from './components/Composer/PostComposer'
import { Icon } from './icons'
import type { IconName } from './icons'
import { toAccountRecord, toAuthStatusRecord, toPostRecord, toProviderInstanceRecord, toRuntimeConfigRecord, toTeamMemberRecord, toTeamRecord, toUserRecord } from './mappers'
import { postsForTeam, resolveScheduleChange, sharedAccountLabels } from './schedule'
import type { AccountRecord, AppSection, AuthStatusRecord, PostRecord, ProviderInstanceRecord, ProviderName, RuntimeConfigRecord, SettingsState, TeamRecord, TeamRole, UserRecord } from './types'

const SETTINGS_STORAGE_KEY = 'goloom-ui-settings'
/** Survives React Strict Mode remounts: hash is promoted here, then one reload applies the session. */
const OIDC_PENDING_SESSION_KEY = 'goloom.oidc.pending_session_v1'

const SECTION_HEADINGS: Record<AppSection, string> = {
  calendar: 'Schedule',
  contentCalendar: 'Content calendar',
  archive: 'Archive',
  analytics: 'Analytics',
  teams: 'Teams',
  accounts: 'Accounts',
  settings: 'Settings',
  admin: 'Admin',
}

const CONTENT_REFRESH_SECTIONS: AppSection[] = ['calendar', 'archive', 'contentCalendar', 'analytics']

type AdminProviderDraft = {
  provider: ProviderName
  name: string
  instanceUrl: string
  clientId: string
  clientSecret: string
  scopes: string
  authorizationEndpoint: string
  tokenEndpoint: string
}

function defaultAdminProviderDraft(): AdminProviderDraft {
  return {
    provider: 'mastodon',
    name: '',
    instanceUrl: '',
    clientId: '',
    clientSecret: '',
    scopes: 'read,write',
    authorizationEndpoint: '',
    tokenEndpoint: '',
  }
}

type AccountConnectDraft = {
  provider: ProviderName
  providerInstanceId: string
  instanceUrl: string
  accessToken: string
  refreshToken: string
  identifier: string
  appPassword: string
  blueskyAuthMode: 'app_password' | 'access_token'
}

function defaultAccountConnectDraft(): AccountConnectDraft {
  return {
    provider: 'mastodon',
    providerInstanceId: '',
    instanceUrl: '',
    accessToken: '',
    refreshToken: '',
    identifier: '',
    appPassword: '',
    blueskyAuthMode: 'app_password',
  }
}

function App() {
  const [section, setSection] = useState<AppSection>('calendar')
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
  const [selectedTeamId, setSelectedTeamId] = useState('')
  const [expandedPostId, setExpandedPostId] = useState<string | null>(null)
  const [editingPostId, setEditingPostId] = useState<string | null>(null)
  const [composerMode, setComposerMode] = useState<'create' | 'edit'>('create')
  const [composerOpen, setComposerOpen] = useState(false)
  const [editorDraft, setEditorDraft] = useState<EditorDraftState>(() => defaultEditorDraft(currentDate, []))
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [dismissedNoticeKey, setDismissedNoticeKey] = useState<string | null>(null)
  const [teamModalOpen, setTeamModalOpen] = useState(false)
  const [newApiTokenExpiresYmd, setNewApiTokenExpiresYmd] = useState(() => format(addDays(new Date(), 90), 'yyyy-MM-dd'))
  const [directoryUsers, setDirectoryUsers] = useState<UserRecord[]>([])
  const [providerInstances, setProviderInstances] = useState<ProviderInstanceRecord[]>([])
  const [adminMetrics, setAdminMetrics] = useState<BackendAdminMetrics | null>(null)
  const [adminMetricsLoading, setAdminMetricsLoading] = useState(false)
  const [adminRuntime, setAdminRuntime] = useState<RuntimeConfigRecord | null>(null)
  const [apiTokens, setApiTokens] = useState<BackendAPIToken[]>([])
  const [apiTokensLoading, setApiTokensLoading] = useState(false)
  const [newTokenPlaintext, setNewTokenPlaintext] = useState<string | null>(null)
  const [newTeamName, setNewTeamName] = useState('')
  const [newTeamDescription, setNewTeamDescription] = useState('')
  const [addMemberUserId, setAddMemberUserId] = useState('')
  const [addMemberRole, setAddMemberRole] = useState<'editor' | 'viewer'>('editor')
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
    } else {
      setError(message)
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
      { id: 'calendar', label: 'Schedule', icon: 'calendar' },
      { id: 'contentCalendar', label: 'Calendar', icon: 'calendarGrid' },
      { id: 'archive', label: 'Archive', icon: 'archive' },
      { id: 'analytics', label: 'Analytics', icon: 'chart' },
    ],
    [],
  )

  const sidebarWorkspaceNav: { id: AppSection; label: string; icon: IconName }[] = useMemo(
    () => [
      { id: 'teams', label: 'Teams', icon: 'teams' },
      { id: 'accounts', label: 'Accounts', icon: 'channels' },
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
    return teamPosts.filter((post) => post.status === 'scheduled' && parseISO(post.scheduledAt) >= baseline)
  }, [currentDate, teamPosts])

  const archivedPosts = useMemo(
    () => [...teamPosts].filter((post) => post.status === 'posted').sort((left, right) => parseISO(right.scheduledAt).getTime() - parseISO(left.scheduledAt).getTime()),
    [teamPosts],
  )

  const plannedPostsForContentCalendar = useMemo(
    () => teamPosts.filter((post) => post.status === 'scheduled'),
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

  const instancesForAccountConnect = useMemo(
    () => providerInstances.filter((p) => p.provider === accountDraft.provider),
    [accountDraft.provider, providerInstances],
  )

  const selectedPost = useMemo(() => posts.find((post) => post.id === expandedPostId) ?? null, [expandedPostId, posts])
  const editTargetPost = useMemo(() => posts.find((post) => post.id === editingPostId) ?? null, [editingPostId, posts])
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
    setSection('calendar')
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
    })
    setComposerMode('edit')
    setEditingPostId(postId)
    setExpandedPostId(postId)
    setComposerOpen(true)
    setSection(
      targetPost.status === 'posted' ? 'archive' : section === 'contentCalendar' ? 'contentCalendar' : 'calendar',
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

  async function handleCreateTeam() {
    if (!api || !newTeamName.trim()) {
      return
    }
    await runAction(async () => {
      const created = await api.createTeam({ name: newTeamName.trim(), description: newTeamDescription.trim() })
      setNewTeamName('')
      setNewTeamDescription('')
      setSelectedTeamId(created.id)
      await loadDashboard({ silent: true })
    }, 'Team created')
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
      const payload = {
        title: editorDraft.title.trim(),
        content: defaultContent,
        scheduled_at: new Date(editorDraft.scheduledAt).toISOString(),
        target_accounts: editorDraft.targetAccountIds,
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
        principalUser={principalUser}
        selectedTeam={selectedTeam}
        syncing={syncing}
        selectedTeamPresent={Boolean(selectedTeam)}
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
              <p className="eyebrow">Social publishing</p>
              <h1>{SECTION_HEADINGS[section]}</h1>
            </div>

            <div className="inline-cluster page-header__toolbar">
              {section === 'teams' ? (
                <button
                  type="button"
                  className="button button--secondary page-header__icon-btn"
                  onClick={() => setTeamModalOpen(true)}
                  aria-label="Open team workspace"
                  title="Create or manage teams"
                >
                  <Icon name="plus" className="inline-icon" />
                </button>
              ) : null}
              <select className="team-select" value={effectiveSelectedTeamId} onChange={(event) => setSelectedTeamId(event.target.value)} disabled={teams.length === 0}>
                {teams.length === 0 ? <option value="">No team loaded</option> : teams.map((team) => <option key={team.id} value={team.id}>{team.name}</option>)}
              </select>
            </div>
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
            <AnalyticsView teamId={effectiveSelectedTeamId} fetchAnalytics={(opts) => api.getTeamAnalytics(effectiveSelectedTeamId, opts)} />
          ) : section === 'analytics' ? (
            <p className="hint">Connect to the API and select a team to view analytics.</p>
          ) : null}

          {section === 'teams' && (
            <div className="teams-view two-column-detail">
              <div className="glass-panel">
                <h2 className="section-card__title">Your teams</h2>
                <p className="hint">Each card shows members, connected accounts, and post activity for that workspace.</p>
                <div className="team-grid">
                  {teams.map((team) => {
                    const teamPosts = posts.filter((p) => p.teamId === team.id)
                    const plannedCount = teamPosts.filter((p) => p.status === 'scheduled').length
                    const publishedCount = teamPosts.filter((p) => p.status === 'posted').length
                    return (
                      <button
                        key={team.id}
                        type="button"
                        className={`team-card ${team.id === effectiveSelectedTeamId ? 'team-card--active' : ''}`}
                        onClick={() => setSelectedTeamId(team.id)}
                      >
                        <strong>{team.name}{team.isPersonal ? ' · Personal' : ''}</strong>
                        <small>{team.members.length} members · {team.accountIds.length} accounts</small>
                        <div className="team-card__stats">
                          <span>{plannedCount} planned</span>
                          <span>{publishedCount} published</span>
                        </div>
                      </button>
                    )
                  })}
                </div>
              </div>

              {selectedTeam ? (
                <div className="glass-panel">
                  <h2 className="section-card__title">{selectedTeam.name}</h2>
                  <p className="hint">{selectedTeam.description || 'No description'}</p>
                  {(() => {
                    const teamPosts = posts.filter((p) => p.teamId === selectedTeam.id)
                    const plannedCount = teamPosts.filter((p) => p.status === 'scheduled').length
                    const publishedCount = teamPosts.filter((p) => p.status === 'posted').length
                    return (
                      <div className="stat-grid" style={{ marginTop: '1rem' }}>
                        <div className="stat-tile">
                          <span className="stat-tile__label">Planned posts</span>
                          <span className="stat-tile__value">{plannedCount}</span>
                        </div>
                        <div className="stat-tile">
                          <span className="stat-tile__label">Published</span>
                          <span className="stat-tile__value">{publishedCount}</span>
                        </div>
                        <div className="stat-tile">
                          <span className="stat-tile__label">Members</span>
                          <span className="stat-tile__value">{selectedTeam.members.length}</span>
                        </div>
                        <div className="stat-tile">
                          <span className="stat-tile__label">Accounts</span>
                          <span className="stat-tile__value">{selectedTeam.accountIds.length}</span>
                        </div>
                      </div>
                    )
                  })()}
                  {selectedTeam.isPersonal ? (
                    <p className="hint" style={{ marginTop: '1rem' }}>This is your personal workspace. Invite other users from a shared team instead.</p>
                  ) : (
                    <p className="hint" style={{ marginTop: '1rem' }}>
                      Use the <strong>+</strong> button next to the team selector to create a shared team or manage members and invitations.
                    </p>
                  )}
                </div>
              ) : null}
            </div>
          )}

          {section === 'accounts' && (
            <div className="accounts-view two-column-detail">
              <div className="glass-panel">
                <h2 className="section-card__title">Connected accounts</h2>
                <p className="hint">
                  Social accounts are attached to the workspace selected in the header — including your personal workspace. Use them as post destinations in the composer.
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
                          </div>
                        </div>
                        {canEditTeamAccounts ? (
                          <button type="button" className="button button--secondary" onClick={() => void handleDeleteTeamAccount(account.id)} disabled={syncing}>
                            <Icon name="trash" className="inline-icon" />
                            <span>Remove</span>
                          </button>
                        ) : null}
                      </li>
                    ))}
                  </ul>
                )}
                {!canEditTeamAccounts && selectedTeam ? (
                  <p className="hint">View-only members cannot connect or remove accounts.</p>
                ) : null}
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
                      <p className="hint">No {accountDraft.provider} instance is registered yet. Ask an administrator to add one under Admin, or use the instance URL field when the provider allows it.</p>
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
                        <div className="inline-cluster" style={{ flexWrap: 'wrap', marginTop: '0.5rem' }}>
                          <button
                            type="button"
                            className="button button--primary"
                            onClick={() => void handleMastodonOAuthConnect()}
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
                        <button type="button" className="button button--secondary" onClick={() => void handleConnectSocialAccount()} disabled={syncing}>
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
                        <button type="button" className="button button--primary" onClick={() => void handleConnectSocialAccount()} disabled={syncing}>
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
                        <button type="button" className="button button--primary" onClick={() => void handleConnectSocialAccount()} disabled={syncing}>
                          Connect Bluesky
                        </button>
                      </>
                    ) : null}
                  </>
                )}
              </div>
            </div>
          )}

          {section === 'settings' && (
            <div className="settings-view two-column-detail">
              <div className="glass-panel">
                <SettingsCard title="Browser session">
                  <label className="field">
                    <span>API base URL (optional)</span>
                    <input value={settings.general.apiBaseUrl} onChange={(event) => updateAPIBaseURL(event.target.value)} />
                  </label>
                  <label className="field">
                    <span>Bearer token (OIDC ID token, bootstrap, or API token)</span>
                    <input type="password" value={settings.general.bearerToken} onChange={(event) => setSettings((current) => ({ ...current, general: { ...current.general, bearerToken: event.target.value } }))} />
                  </label>
                  <div className="inline-cluster" style={{ marginTop: '1rem' }}>
                    <button type="button" className="button button--primary" onClick={connectBackend}>
                      Apply session
                    </button>
                    <button type="button" className="button button--secondary" onClick={() => void loadDashboard()} disabled={!api || syncing}>
                      Refresh data
                    </button>
                  </div>
                </SettingsCard>
              </div>

              <div className="glass-panel">
                <h2 className="section-card__title">API tokens</h2>
                <p className="hint">
                  Tokens authenticate as <strong>you</strong>, not a team. Team access follows your memberships. Use <code className="inline-code">Authorization: Bearer &lt;token&gt;</code> on every request.
                  Create automation tokens here; each value is shown only once.
                </p>
                {newTokenPlaintext ? (
                  <div className="token-reveal">
                    <p className="hint">Copy this secret now:</p>
                    <code className="token-reveal__value">{newTokenPlaintext}</code>
                    <button type="button" className="button button--secondary" onClick={() => setNewTokenPlaintext(null)}>
                      Dismiss
                    </button>
                  </div>
                ) : null}
                <div className="inline-cluster" style={{ flexWrap: 'wrap', marginTop: '1rem', alignItems: 'flex-end' }}>
                  <label className="field" style={{ minWidth: '12rem' }}>
                    <span>Label</span>
                    <input
                      value={newApiTokenName}
                      onChange={(event) => setNewApiTokenName(event.target.value)}
                      placeholder="e.g. CI, laptop"
                    />
                  </label>
                  <label className="field" style={{ minWidth: '11rem' }}>
                    <span>Expires (UTC end of day)</span>
                    <input type="date" value={newApiTokenExpiresYmd} onChange={(event) => setNewApiTokenExpiresYmd(event.target.value)} />
                  </label>
                  <button type="button" className="button button--primary" onClick={() => void handleCreateApiToken()} disabled={syncing || !newApiTokenName.trim()}>
                    Create token
                  </button>
                </div>
                <p className="hint">Expiry uses end of the selected calendar day in UTC (default picker value is 90 days ahead).</p>
                {apiTokensLoading ? <p className="hint">Loading tokens…</p> : null}
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Created</th>
                      <th>Expires</th>
                      <th>Last used</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {apiTokens.map((t) => (
                      <tr key={t.id}>
                        <td>{t.name}</td>
                        <td>{format(parseISO(t.created_at), 'PPp')}</td>
                        <td>{t.expires_at && isValid(parseISO(t.expires_at)) ? format(parseISO(t.expires_at), 'PPp') : '—'}</td>
                        <td>{t.last_used_at ? format(parseISO(t.last_used_at), 'PPp') : '—'}</td>
                        <td>
                          <button type="button" className="button button--secondary" onClick={() => void handleRevokeApiToken(t.id)} disabled={syncing}>
                            Revoke
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {apiTokens.length === 0 && !apiTokensLoading ? <p className="hint">No API tokens yet.</p> : null}
              </div>
            </div>
          )}

          {section === 'admin' && principalUser?.globalRole === 'admin' ? (
            <div className="admin-view two-column-detail">
              <div className="glass-panel">
                <h2 className="section-card__title">Overview</h2>
                {adminMetricsLoading ? <p className="hint">Loading metrics…</p> : null}
                {adminMetrics ? (
                  <div className="stat-grid">
                    <div className="stat-tile">
                      <span className="stat-tile__label">Users</span>
                      <span className="stat-tile__value">{adminMetrics.users_count}</span>
                    </div>
                    <div className="stat-tile">
                      <span className="stat-tile__label">Teams</span>
                      <span className="stat-tile__value">{adminMetrics.teams_count}</span>
                    </div>
                    <div className="stat-tile">
                      <span className="stat-tile__label">Provider instances</span>
                      <span className="stat-tile__value">{adminMetrics.provider_instances_count}</span>
                    </div>
                    <div className="stat-tile">
                      <span className="stat-tile__label">Queued / pending</span>
                      <span className="stat-tile__value">{adminMetrics.posts_pending}</span>
                    </div>
                    <div className="stat-tile">
                      <span className="stat-tile__label">Publishing</span>
                      <span className="stat-tile__value">{adminMetrics.posts_processing}</span>
                    </div>
                    <div className="stat-tile">
                      <span className="stat-tile__label">Posted</span>
                      <span className="stat-tile__value">{adminMetrics.posts_posted}</span>
                    </div>
                    <div className="stat-tile stat-tile--warn">
                      <span className="stat-tile__label">Failed</span>
                      <span className="stat-tile__value">{adminMetrics.posts_failed}</span>
                    </div>
                    <div className="stat-tile">
                      <span className="stat-tile__label">Cancelled</span>
                      <span className="stat-tile__value">{adminMetrics.posts_cancelled}</span>
                    </div>
                  </div>
                ) : null}
              </div>

              <div className="glass-panel">
                <h2 className="section-card__title">Registered users</h2>
                <p className="hint">Everyone who can sign in to this deployment.</p>
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Email</th>
                      <th>Access</th>
                      <th>Registered</th>
                    </tr>
                  </thead>
                  <tbody>
                    {[...directoryUsers]
                      .sort((left, right) => left.name.localeCompare(right.name))
                      .map((user) => (
                        <tr key={user.id}>
                          <td>{user.name}</td>
                          <td>{user.email}</td>
                          <td>{user.globalRole === 'admin' ? 'Administrator' : 'Member'}</td>
                          <td>
                            {user.createdAt && isValid(parseISO(user.createdAt))
                              ? format(parseISO(user.createdAt), 'PP')
                              : '—'}
                          </td>
                        </tr>
                      ))}
                  </tbody>
                </table>
                {directoryUsers.length === 0 ? <p className="hint">No users returned from the directory.</p> : null}
              </div>

              {adminRuntime ? (
                <div className="glass-panel">
                  <h2 className="section-card__title">Scheduler &amp; server</h2>
                  <dl className="kv-list">
                    <dt>Worker processes</dt>
                    <dd>{adminRuntime.scheduler.workers}</dd>
                    <dt>Poll interval</dt>
                    <dd>{adminRuntime.scheduler.pollInterval}</dd>
                    <dt>HTTP listen</dt>
                    <dd><code className="inline-code">{adminRuntime.general.httpAddr}</code></dd>
                    <dt>Rate limit / min</dt>
                    <dd>{adminRuntime.security.rateLimitPerMinute}</dd>
                  </dl>
                </div>
              ) : null}

              <div className="glass-panel">
                <h2 className="section-card__title">Provider onboarding</h2>
                <p className="hint">Register Mastodon, Friendica, or Bluesky instances so teams can connect accounts. Mastodon can auto-discover OAuth endpoints from the instance URL when credentials are omitted.</p>

                <div className="inline-cluster" style={{ marginBottom: '1rem' }}>
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
                    <option value="mastodon">Mastodon</option>
                    <option value="friendica">Friendica</option>
                    <option value="bluesky">Bluesky</option>
                  </select>
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
                      Cancel edit
                    </button>
                  ) : null}
                </div>

                <label className="field">
                  <span>Display name</span>
                  <input value={adminProviderDraft.name} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, name: event.target.value }))} placeholder="My instance" />
                </label>
                <label className="field">
                  <span>Instance URL</span>
                  <input value={adminProviderDraft.instanceUrl} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, instanceUrl: event.target.value }))} placeholder="https://mastodon.social" />
                </label>

                <details
                  className="advanced-config"
                  open={showAdminProviderAdvanced}
                  onToggle={(event) => setShowAdminProviderAdvanced(event.currentTarget.open)}
                >
                  <summary className="advanced-config__summary">Advanced configuration</summary>
                  <p className="hint" style={{ marginTop: '0.75rem' }}>
                    OAuth client credentials, scopes, and token endpoints. Mastodon can auto-register an app when these are left empty; set them manually for custom apps or strict instances.
                  </p>
                  <label className="field">
                    <span>Client ID</span>
                    <input value={adminProviderDraft.clientId} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, clientId: event.target.value }))} placeholder="Optional for Mastodon auto-register" />
                  </label>
                  <label className="field">
                    <span>Client secret</span>
                    <input type="password" value={adminProviderDraft.clientSecret} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, clientSecret: event.target.value }))} placeholder="Leave blank to keep existing on update" />
                  </label>
                  <label className="field">
                    <span>Scopes (comma-separated)</span>
                    <input value={adminProviderDraft.scopes} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, scopes: event.target.value }))} />
                  </label>
                  <label className="field">
                    <span>Authorization endpoint</span>
                    <input value={adminProviderDraft.authorizationEndpoint} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, authorizationEndpoint: event.target.value }))} />
                  </label>
                  <label className="field">
                    <span>Token endpoint</span>
                    <input value={adminProviderDraft.tokenEndpoint} onChange={(event) => setAdminProviderDraft((c) => ({ ...c, tokenEndpoint: event.target.value }))} />
                  </label>
                </details>

                <button type="button" className="button button--primary" onClick={() => void handleSaveAdminProvider()} disabled={syncing} style={{ marginTop: '1rem' }}>
                  {editingProviderId ? 'Update provider' : 'Register provider'}
                </button>

                <h3 className="subsection-title" style={{ marginTop: '1.5rem' }}>Registered instances</h3>
                <ul className="provider-admin-list">
                  {providerInstances.map((p) => {
                    const onboarded = accounts.filter((a) => a.providerInstanceId === p.id).length
                    return (
                      <li key={p.id}>
                        <div>
                          <strong>{p.name}</strong>
                          <span className="hint"> {p.provider} · {p.instanceUrl}</span>
                          <span className="provider-admin-list__count">
                            {onboarded} account{onboarded === 1 ? '' : 's'} onboarded
                          </span>
                        </div>
                        <div className="inline-cluster" style={{ flexShrink: 0 }}>
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
                            Edit
                          </button>
                          <button
                            type="button"
                            className="button button--secondary"
                            onClick={() => void handleDeleteProviderInstance(p.id)}
                            disabled={syncing || onboarded > 0}
                            title={onboarded > 0 ? 'Disconnect every account using this instance first' : 'Remove provider instance'}
                          >
                            <Icon name="trash" className="inline-icon" />
                            <span>Remove</span>
                          </button>
                        </div>
                      </li>
                    )
                  })}
                </ul>
                {providerInstances.length === 0 ? <p className="hint">No provider instances yet.</p> : null}
              </div>
            </div>
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
              {selectedPost && selectedPost.status === 'scheduled' && canEditScheduledPosts ? (
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

      {teamModalOpen ? (
        <div className="modal-backdrop" role="presentation" onClick={() => setTeamModalOpen(false)}>
          <div className="composer-container team-workspace-modal" onClick={(event) => event.stopPropagation()}>
            <div className="composer-main">
              <header className="team-workspace-modal__head">
                <div>
                  <p className="eyebrow">Team workspace</p>
                  <h2>Create or manage teams</h2>
                </div>
                <button type="button" className="button button--secondary" onClick={() => setTeamModalOpen(false)} aria-label="Close">
                  <Icon name="close" className="inline-icon" />
                </button>
              </header>

              <h3 className="subsection-title">Create team</h3>
              <p className="hint">New teams you create are owned by you. Personal workspaces cannot receive members.</p>
              <label className="field">
                <span>Name</span>
                <input value={newTeamName} onChange={(event) => setNewTeamName(event.target.value)} placeholder="Marketing" />
              </label>
              <label className="field">
                <span>Description</span>
                <input value={newTeamDescription} onChange={(event) => setNewTeamDescription(event.target.value)} placeholder="Optional" />
              </label>
              <button type="button" className="button button--primary" onClick={() => void handleCreateTeam()} disabled={syncing || !newTeamName.trim()}>
                Create team
              </button>

              <div className="divider" style={{ margin: '1.5rem 0' }} />

              <h3 className="subsection-title">Members · {selectedTeam?.name ?? '—'}</h3>
              {!selectedTeam ? (
                <p className="hint">Select a team in the header to manage members.</p>
              ) : selectedTeam.isPersonal ? (
                <p className="hint">Personal workspace has no shared members.</p>
              ) : myRoleInSelectedTeam === 'owner' ? (
                <>
                  <ul className="member-list">
                    {selectedTeam.members.map((m) => (
                      <li key={m.userId} className="member-list__row">
                        <div>
                          <strong>{directoryUserLabel(m.userId)}</strong>
                          <span className="member-list__role">{m.role}</span>
                        </div>
                        {m.userId !== principalUser?.id ? (
                          <button type="button" className="button button--secondary" onClick={() => void handleRemoveTeamMember(m.userId)} disabled={syncing}>
                            Remove
                          </button>
                        ) : null}
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
          </div>
        </div>
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
        onClose={closeComposer}
      />

      <nav className="mobile-nav">
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
        <button type="button" className={section === 'accounts' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'} onClick={() => setSection('accounts')}>
          <Icon name="channels" className="inline-icon" />
        </button>
        <button type="button" className={section === 'settings' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'} onClick={() => setSection('settings')}>
          <Icon name="settings" className="inline-icon" />
        </button>
      </nav>
    </div>
  )
}

function AuthShell({
  theme,
  children,
}: {
  theme: 'dark' | 'light'
  children: ReactNode
}) {
  return (
    <div className="app-shell auth-shell" data-theme={theme}>
      <div className="auth-screen-center">
        <section className="auth-card">
          <div className="auth-card__hero auth-card__hero--compact">
            <div className="nav-rail__brand auth-card__brand" title="goloom">
              <span>G</span>
            </div>
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

function AuthPanel({
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
          <button
            type="button"
            className={view === 'login' ? 'button button--prominent' : 'button button--secondary'}
            onClick={() => onViewChange('login')}
          >
            Sign in
          </button>
          <button
            type="button"
            className={view === 'bootstrap' ? 'button button--prominent' : 'button button--secondary'}
            onClick={() => onViewChange('bootstrap')}
          >
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

            {authStatus.oidcOAuthEnabled && (initial || !isBootstrap) ? (
              <p className="hint auth-form__divider-label">or use a token</p>
            ) : null}

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

            {showDevHints ? (
              <p className="hint">API base URL can be changed later under Settings if this UI is not served from the same host as the API.</p>
            ) : null}
          </div>
        ) : null}

        {authStatus.hasUsers && !authStatus.oidcOAuthEnabled && !authStatus.bootstrapRecoveryEnabled ? (
          <p className="hint">OIDC browser login is not configured. Use an API token or ask an admin to set OIDC_ISSUER_URL, OIDC_CLIENT_ID, OIDC_REDIRECT_URI, and PUBLIC_BASE_URL.</p>
        ) : null}
      </div>
    </div>
  )
}

function SettingsCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="subpanel">
      <h3>{title}</h3>
      {children}
    </section>
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

export default App
