import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { addMinutes, format, parseISO, set, startOfDay } from 'date-fns'

import { ApiError, createApiClient, requestAuthStatus, requestStartOIDCLogin, type BackendAPIToken, type BackendAdminMetrics } from './api'
import { initialSettings } from './data'
import { Icon } from './icons'
import type { IconName } from './icons'
import { toAccountRecord, toAuthStatusRecord, toPostRecord, toProviderInstanceRecord, toRuntimeConfigRecord, toTeamMemberRecord, toTeamRecord, toUserRecord } from './mappers'
import { postsForTeam, sharedAccountLabels, SLOT_MINUTES } from './schedule'
import type { AccountRecord, AppSection, AuthStatusRecord, PostRecord, ProviderInstanceRecord, ProviderName, RuntimeConfigRecord, SettingsState, TeamRecord, TeamRole, UserRecord } from './types'

const SETTINGS_STORAGE_KEY = 'goloom-ui-settings'

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
  const [theme, setTheme] = useState<'dark' | 'light'>(() => getSystemTheme())
  const [currentDate] = useState<Date>(new Date())
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
  const [editorDraft, setEditorDraft] = useState(defaultEditorDraft(currentDate, []))
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)
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

  useEffect(() => {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    const syncTheme = (event?: MediaQueryListEvent) => {
      const isDark = event ? event.matches : mediaQuery.matches
      setTheme(isDark ? 'dark' : 'light')
    }

    syncTheme()
    mediaQuery.addEventListener('change', syncTheme)
    return () => mediaQuery.removeEventListener('change', syncTheme)
  }, [])

  useEffect(() => {
    let cancelled = false
    requestAuthStatus(settings.general.apiBaseUrl.trim())
      .then((status) => {
        if (cancelled) {
          return
        }
        const mapped = toAuthStatusRecord(status)
        setAuthStatus(mapped)
        if (!activeConnection.bearerToken.trim()) {
          setAuthView(mapped.bootstrapEnabled ? 'bootstrap' : 'login')
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
    window.history.replaceState({}, document.title, `${window.location.pathname}${window.location.search}`)

    void (async () => {
      setAuthSubmitting(true)
      setAuthError(null)
      setStatusMessage(null)
      const baseUrl = loadStoredSettings().general.apiBaseUrl.trim()
      try {
        const meResponse = await createApiClient({ baseUrl, token }).me()
        setActiveConnection({ apiBaseUrl: baseUrl, bearerToken: token })
        setSettings((current) => ({
          ...current,
          general: { ...current.general, apiBaseUrl: baseUrl, bearerToken: token },
        }))
        setAuthTokenDraft(token)
        setPrincipalUser(toUserRecord(meResponse.user))
        setStatusMessage('Signed in with OpenID Connect')
      } catch (cause) {
        if (cause instanceof ApiError && cause.status === 401) {
          setAuthError('OpenID Connect sign-in was rejected.')
        } else {
          setAuthError(cause instanceof Error ? cause.message : 'Sign-in failed')
        }
      } finally {
        setAuthSubmitting(false)
      }
    })()
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
      const baseUrl = settings.general.apiBaseUrl.trim()
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

  const loadDashboard = useCallback(async () => {
    if (!api) {
      return
    }

    setLoading(true)
    setError(null)

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
      setError(cause instanceof Error ? cause.message : 'Failed to load dashboard data')
    } finally {
      setLoading(false)
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
          await loadDashboard()
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
      { id: 'archive', label: 'Archive', icon: 'archive' },
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

  const showPreviewColumn = section === 'calendar' || section === 'archive'

  const myRoleInSelectedTeam = useMemo((): TeamRole | null => {
    if (!selectedTeam || !principalUser) {
      return null
    }
    return selectedTeam.members.find((m) => m.userId === principalUser.id)?.role ?? null
  }, [principalUser, selectedTeam])

  const canEditTeamAccounts = myRoleInSelectedTeam === 'owner' || myRoleInSelectedTeam === 'editor'

  const instancesForAccountConnect = useMemo(
    () => providerInstances.filter((p) => p.provider === accountDraft.provider),
    [accountDraft.provider, providerInstances],
  )

  const selectedPost = useMemo(() => posts.find((post) => post.id === expandedPostId) ?? null, [expandedPostId, posts])
  const editTargetPost = useMemo(() => posts.find((post) => post.id === editingPostId) ?? null, [editingPostId, posts])
  const selectedComposerAccounts = useMemo(() => teamAccounts.filter((account) => editorDraft.targetAccountIds.includes(account.id)), [editorDraft.targetAccountIds, teamAccounts])

  const maxChars = useMemo(() => {
    if (selectedComposerAccounts.length === 0) {
      return 0
    }
    return selectedComposerAccounts.reduce((lowest, account) => Math.min(lowest, account.maxChars), selectedComposerAccounts[0].maxChars)
  }, [selectedComposerAccounts])

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

  function openEditor(postId: string) {
    const targetPost = posts.find((post) => post.id === postId)
    if (!targetPost) {
      return
    }
    setEditorDraft({
      title: targetPost.title,
      content: targetPost.content,
      scheduledAt: toInputDateTime(parseISO(targetPost.scheduledAt)),
      targetAccountIds: targetPost.targetAccountIds,
      status: targetPost.status,
    })
    setComposerMode('edit')
    setEditingPostId(postId)
    setExpandedPostId(postId)
    setComposerOpen(true)
    setSection(targetPost.status === 'posted' ? 'archive' : 'calendar')
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
      await api.createTeam({ name: newTeamName.trim(), description: newTeamDescription.trim() })
      setNewTeamName('')
      setNewTeamDescription('')
      await loadDashboard()
    }, 'Team created')
  }

  async function handleAddTeamMember() {
    if (!api || !selectedTeam || !addMemberUserId.trim()) {
      return
    }
    await runAction(async () => {
      await api.addTeamMember(selectedTeam.id, { user_id: addMemberUserId.trim(), role: addMemberRole })
      setAddMemberUserId('')
      await loadDashboard()
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
      await loadDashboard()
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
      await loadDashboard()
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
      await loadDashboard()
    }, editingProviderId ? 'Provider updated' : 'Provider registered')
  }

  async function handleDeleteTeamAccount(accountId: string) {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      await api.deleteAccount(selectedTeam.id, accountId)
      await loadDashboard()
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
        await loadDashboard()
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
        await loadDashboard()
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
          await loadDashboard()
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
        await loadDashboard()
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
    await runAction(async () => {
      const res = await api.createMyApiToken(newApiTokenName.trim())
      setNewTokenPlaintext(res.token)
      setNewApiTokenName('')
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
    if (editorDraft.targetAccountIds.length === 0 || !editorDraft.scheduledAt || editorDraft.content.trim().length === 0 || (maxChars > 0 && editorDraft.content.length > maxChars)) {
      return
    }

    await runAction(async () => {
      const payload = {
        title: editorDraft.title.trim(),
        content: editorDraft.content.trim(),
        scheduled_at: new Date(editorDraft.scheduledAt).toISOString(),
        target_accounts: editorDraft.targetAccountIds,
      }

      if (composerMode === 'edit' && editTargetPost) {
        await api.updatePost(selectedTeam.id, editTargetPost.id, payload)
      } else {
        await api.createPost(selectedTeam.id, payload)
      }
      setComposerOpen(false)
      await loadDashboard()
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
      await loadDashboard()
    }, 'Post deleted')
  }

  if (authStatusLoading && !activeConnection.bearerToken.trim()) {
    return (
      <AuthShell theme={theme}>
        <AuthPanel
          view="login"
          authStatus={null}
          authTokenDraft={authTokenDraft}
          authError={authError}
          authSubmitting={true}
          apiBaseUrl={settings.general.apiBaseUrl}
          onViewChange={setAuthView}
          onTokenChange={setAuthTokenDraft}
          onAPIBaseURLChange={updateAPIBaseURL}
          onSubmit={() => undefined}
          onStartOIDCLogin={() => undefined}
        />
      </AuthShell>
    )
  }

  if (!activeConnection.bearerToken.trim()) {
    return (
      <AuthShell theme={theme}>
        <AuthPanel
          view={authView}
          authStatus={authStatus}
          authTokenDraft={authTokenDraft}
          authError={authError}
          authSubmitting={authSubmitting}
          apiBaseUrl={settings.general.apiBaseUrl}
          onViewChange={setAuthView}
          onTokenChange={setAuthTokenDraft}
          onAPIBaseURLChange={updateAPIBaseURL}
          onSubmit={() => void authenticateWithToken(authView)}
          onStartOIDCLogin={() => void startOIDCLogin()}
        />
      </AuthShell>
    )
  }

  return (
    <div className={`app-shell ${showPreviewColumn ? 'app-shell--triple' : 'app-shell--double'}`} data-theme={theme}>
      <aside className="app-sidebar" aria-label="Main navigation">
        <div className="app-sidebar__header">
          <div className="app-sidebar__logo" title="goloom" aria-hidden="true">
            <span className="app-sidebar__logo-layer app-sidebar__logo-layer--a" />
            <span className="app-sidebar__logo-layer app-sidebar__logo-layer--b" />
            <span className="app-sidebar__logo-layer app-sidebar__logo-layer--c" />
          </div>
          <span className="app-sidebar__title">goloom</span>
        </div>

        <button
          type="button"
          className="app-sidebar__cta"
          onClick={openCreateComposer}
          disabled={!selectedTeam || syncing}
        >
          <span className="app-sidebar__cta-icon" aria-hidden="true">
            <Icon name="plus" className="inline-icon" />
          </span>
          <span>Create post</span>
        </button>

        <nav className="app-sidebar__nav" aria-label="Sections">
          <div className="app-sidebar__nav-group">
            <p className="app-sidebar__nav-heading">Content</p>
            <ul className="app-sidebar__nav-list">
              {sidebarContentNav.map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    className={`app-sidebar__link ${section === item.id ? 'app-sidebar__link--active' : ''}`}
                    onClick={() => setSection(item.id)}
                  >
                    <Icon name={item.icon} className="app-sidebar__link-icon" />
                    <span>{item.label}</span>
                  </button>
                </li>
              ))}
            </ul>
          </div>

          <div className="app-sidebar__divider" role="presentation" />

          <div className="app-sidebar__nav-group">
            <p className="app-sidebar__nav-heading">Workspace</p>
            <ul className="app-sidebar__nav-list">
              {sidebarWorkspaceNav.map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    className={`app-sidebar__link ${section === item.id ? 'app-sidebar__link--active' : ''}`}
                    onClick={() => setSection(item.id)}
                  >
                    <Icon name={item.icon} className="app-sidebar__link-icon" />
                    <span>{item.label}</span>
                  </button>
                </li>
              ))}
            </ul>
          </div>

          <div className="app-sidebar__divider" role="presentation" />

          <div className="app-sidebar__nav-group">
            <p className="app-sidebar__nav-heading">Configuration</p>
            <ul className="app-sidebar__nav-list">
              {sidebarConfigNav.map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    className={`app-sidebar__link ${section === item.id ? 'app-sidebar__link--active' : ''}`}
                    onClick={() => setSection(item.id)}
                  >
                    <Icon name={item.icon} className="app-sidebar__link-icon" />
                    <span>{item.label}</span>
                  </button>
                </li>
              ))}
            </ul>
          </div>
        </nav>

        <div className="app-sidebar__footer">
          <div className="app-sidebar__user">
            <div className="app-sidebar__avatar" aria-hidden="true">
              {initialsFromName(principalUser?.name ?? '')}
            </div>
            <div className="app-sidebar__user-text">
              <span className="app-sidebar__user-name">{principalUser?.name ?? 'Signed in'}</span>
              <span className="app-sidebar__user-meta">{selectedTeam?.name ?? principalUser?.email ?? '—'}</span>
            </div>
          </div>
          <button
            type="button"
            className="app-sidebar__theme"
            onClick={() => setTheme((current) => (current === 'dark' ? 'light' : 'dark'))}
            title={theme === 'dark' ? 'Light mode' : 'Dark mode'}
            aria-label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
          >
            <Icon name={theme === 'dark' ? 'sun' : 'moon'} className="inline-icon" />
          </button>
        </div>
      </aside>

      <main className="app-main">
          <header className="page-header">
            <div>
              <p className="eyebrow">Social publishing</p>
              <h1>{section.charAt(0).toUpperCase() + section.slice(1)}</h1>
            </div>

            <div className="inline-cluster">
              <select className="team-select" value={effectiveSelectedTeamId} onChange={(event) => setSelectedTeamId(event.target.value)} disabled={teams.length === 0}>
                {teams.length === 0 ? <option value="">No team loaded</option> : teams.map((team) => <option key={team.id} value={team.id}>{team.name}</option>)}
              </select>
            </div>
          </header>

          {(error || statusMessage || loading) && (
            <section className="glass-panel">
              {loading ? <span className="hint">Loading backend data…</span> : null}
              {statusMessage ? <span className="status-banner__success">{statusMessage}</span> : null}
              {error ? <span className="status-banner__error">{error}</span> : null}
            </section>
          )}

          {section === 'calendar' && (
            <div className="timeline-view">
              {upcomingPosts.length === 0 ? (
                <div className="empty-state">
                  <h3>No upcoming posts</h3>
                  <p className="hint">Create a post to start building your publishing timeline.</p>
                </div>
              ) : (
                groupPostsByDay(upcomingPosts).map((group) => (
                  <section key={group.key} className="timeline-day-section">
                    <p className="eyebrow" style={{ marginBottom: '1rem' }}>{format(parseISO(group.posts[0].scheduledAt), 'EEEE, d MMMM')}</p>
                    <div className="posts-grid">
                      {group.posts.map((post) => (
                        <PostCard
                          key={post.id}
                          post={post}
                          active={expandedPostId === post.id}
                          onClick={() => setExpandedPostId(post.id)}
                          onEdit={() => openEditor(post.id)}
                          onDelete={() => void deletePost(post.id)}
                          accounts={accounts}
                        />
                      ))}
                    </div>
                  </section>
                ))
              )}
            </div>
          )}

          {section === 'archive' && (
            <div className="archive-view">
              {archivedPosts.map((post) => (
                <PostCard
                  key={post.id}
                  post={post}
                  active={expandedPostId === post.id}
                  onClick={() => setExpandedPostId(post.id)}
                  onEdit={() => openEditor(post.id)}
                  onDelete={() => void deletePost(post.id)}
                  accounts={accounts}
                  isArchived
                />
              ))}
            </div>
          )}

          {section === 'teams' && (
            <div className="teams-view two-column-detail">
              <div className="glass-panel">
                <h2 className="section-card__title">Create team</h2>
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
              </div>

              <div className="glass-panel">
                <h2 className="section-card__title">Your teams</h2>
                <div className="team-grid">
                  {teams.map((team) => (
                    <button
                      key={team.id}
                      type="button"
                      className={`team-card ${team.id === selectedTeamId ? 'team-card--active' : ''}`}
                      onClick={() => setSelectedTeamId(team.id)}
                    >
                      <strong>{team.name}{team.isPersonal ? ' · Personal' : ''}</strong>
                      <small>{team.members.length} members · {team.accountIds.length} accounts</small>
                    </button>
                  ))}
                </div>
              </div>

              {selectedTeam ? (
                <div className="glass-panel">
                  <h2 className="section-card__title">{selectedTeam.name}</h2>
                  <p className="hint">{selectedTeam.description || 'No description'}</p>

                  {selectedTeam.isPersonal ? (
                    <p className="hint">This is your personal workspace. Invite other users from a shared team instead.</p>
                  ) : myRoleInSelectedTeam === 'owner' ? (
                    <>
                      <h3 className="subsection-title">Members</h3>
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

                      <h3 className="subsection-title">Add member</h3>
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

                      <h3 className="subsection-title">Invite by email</h3>
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
                      <h3 className="subsection-title">Members</h3>
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
                <div className="inline-cluster" style={{ flexWrap: 'wrap', marginTop: '1rem' }}>
                  <input
                    value={newApiTokenName}
                    onChange={(event) => setNewApiTokenName(event.target.value)}
                    placeholder="Token label (e.g. CI, laptop)"
                  />
                  <button type="button" className="button button--primary" onClick={() => void handleCreateApiToken()} disabled={syncing || !newApiTokenName.trim()}>
                    Create token
                  </button>
                </div>
                {apiTokensLoading ? <p className="hint">Loading tokens…</p> : null}
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Name</th>
                      <th>Created</th>
                      <th>Last used</th>
                      <th />
                    </tr>
                  </thead>
                  <tbody>
                    {apiTokens.map((t) => (
                      <tr key={t.id}>
                        <td>{t.name}</td>
                        <td>{format(parseISO(t.created_at), 'PPp')}</td>
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
                  {providerInstances.map((p) => (
                    <li key={p.id}>
                      <div>
                        <strong>{p.name}</strong>
                        <span className="hint"> {p.provider} · {p.instanceUrl}</span>
                      </div>
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
                    </li>
                  ))}
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
            <p className="eyebrow">Live Preview</p>
            <h3>{selectedPost ? selectedPost.title || 'Untitled Post' : 'No post selected'}</h3>
          </div>
          <div className="preview-content">
            {selectedPost ? (
              sharedAccountLabels(selectedPost, accounts).map((account) => (
                <SocialPreview
                  key={account.id}
                  account={account}
                  content={selectedPost.content}
                  scheduledAt={selectedPost.scheduledAt}
                  theme={theme}
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

      {composerOpen && (
        <div className="modal-backdrop" onClick={closeComposer}>
          <div className="composer-container" onClick={(event) => event.stopPropagation()}>
            <div className="composer-main">
              <header>
                <p className="eyebrow">Composer</p>
                <h2>{composerMode === 'edit' ? 'Edit post' : 'Create post'}</h2>
              </header>

              <label className="field">
                <span>Title</span>
                <input value={editorDraft.title} onChange={(event) => setEditorDraft((current) => ({ ...current, title: event.target.value }))} placeholder="Post title for internal reference" />
              </label>

              <label className="field">
                <span>Message</span>
                <textarea rows={10} value={editorDraft.content} onChange={(event) => setEditorDraft((current) => ({ ...current, content: event.target.value }))} placeholder="What's on your mind?" />
              </label>

              <div className="inline-cluster">
                 <div className={`char-counter ${charCounterClass(editorDraft.content.length, maxChars)}`}>
                    <strong>{editorDraft.content.length}</strong>
                    <span>/ {maxChars || '—'}</span>
                  </div>
                  <DestinationStack accounts={selectedComposerAccounts} />
              </div>

              <label className="field">
                <span>Scheduled at</span>
                <input type="datetime-local" value={editorDraft.scheduledAt} onChange={(event) => setEditorDraft((current) => ({ ...current, scheduledAt: event.target.value }))} />
              </label>

              <footer className="inline-cluster" style={{ marginTop: 'auto', paddingTop: '2rem' }}>
                <button type="button" className="button button--primary" disabled={syncing || editorDraft.targetAccountIds.length === 0 || (maxChars > 0 && editorDraft.content.length > maxChars)} onClick={() => void handleSavePost()}>
                  <Icon name="calendar" className="inline-icon" />
                  <span>{composerMode === 'edit' ? 'Save changes' : 'Schedule post'}</span>
                </button>
                <button type="button" className="button button--secondary" onClick={closeComposer}>
                  Cancel
                </button>
              </footer>
            </div>

            <aside className="composer-sidebar">
              <p className="eyebrow">Destinations</p>
              <div className="checkbox-grid">
                {teamAccounts.map((account) => (
                  <label key={account.id} className="checkbox-card glass-panel" style={{ padding: '0.75rem' }}>
                    <input
                      type="checkbox"
                      checked={editorDraft.targetAccountIds.includes(account.id)}
                      onChange={(event) =>
                        setEditorDraft((current) => ({
                          ...current,
                          targetAccountIds: event.target.checked
                            ? [...current.targetAccountIds, account.id]
                            : current.targetAccountIds.filter((id) => id !== account.id),
                        }))
                      }
                    />
                    <div className="inline-cluster">
                      <DestinationAvatar account={account} />
                      <div>
                        <strong>{account.name}</strong>
                        <small>{account.provider}</small>
                      </div>
                    </div>
                  </label>
                ))}
              </div>

              <div className="divider" />
              <p className="eyebrow">Preview</p>
              {selectedComposerAccounts.length > 0 ? (
                <SocialPreview
                  account={selectedComposerAccounts[0]}
                  content={editorDraft.content}
                  scheduledAt={editorDraft.scheduledAt}
                  theme={theme}
                />
              ) : (
                <p className="hint">Select a destination to see a preview.</p>
              )}
            </aside>
          </div>
        </div>
      )}

      <nav className="mobile-nav">
        <button type="button" className={section === 'calendar' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'} onClick={() => setSection('calendar')}>
          <Icon name="calendar" className="inline-icon" />
        </button>
        <button type="button" className="mobile-nav__item mobile-nav__item--primary" onClick={openCreateComposer}>
          <Icon name="edit" className="inline-icon" />
        </button>
        <button type="button" className={section === 'archive' ? 'mobile-nav__item mobile-nav__item--active' : 'mobile-nav__item'} onClick={() => setSection('archive')}>
          <Icon name="archive" className="inline-icon" />
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
      <section className="auth-card">
        <div className="auth-card__hero">
          <div className="nav-rail__brand auth-card__brand" title="goloom">
            <span>G</span>
          </div>
          <div className="auth-card__copy">
            <p className="eyebrow">Single-binary deployment</p>
            <h1>Welcome to goloom</h1>
            <p className="hint">The UI and API are served by the same application. Sign in to finish setup and start scheduling posts.</p>
          </div>
        </div>
        {children}
      </section>
    </div>
  )
}

function AuthPanel({
  view,
  authStatus,
  authTokenDraft,
  authError,
  authSubmitting,
  apiBaseUrl,
  onViewChange,
  onTokenChange,
  onAPIBaseURLChange,
  onSubmit,
  onStartOIDCLogin,
}: {
  view: 'bootstrap' | 'login'
  authStatus: AuthStatusRecord | null
  authTokenDraft: string
  authError: string | null
  authSubmitting: boolean
  apiBaseUrl: string
  onViewChange: (view: 'bootstrap' | 'login') => void
  onTokenChange: (value: string) => void
  onAPIBaseURLChange: (value: string) => void
  onSubmit: () => void
  onStartOIDCLogin: () => void
}) {
  const isBootstrap = view === 'bootstrap'
  const title = isBootstrap ? 'Bootstrap onboarding' : 'Sign in'
  const description = isBootstrap
    ? 'Use the bootstrap admin token configured on the server for the first administrator session.'
    : authStatus?.oidcOAuthEnabled
      ? 'Sign in with OpenID Connect, or use an API token in the field below.'
      : 'Enter a bearer token to open the embedded dashboard.'

  return (
    <div className="auth-panel">
      {authStatus?.bootstrapEnabled ? (
        <div className="auth-tabs" role="tablist" aria-label="Authentication mode">
          <button
            type="button"
            className={view === 'bootstrap' ? 'button button--prominent' : 'button button--secondary'}
            onClick={() => onViewChange('bootstrap')}
          >
            Bootstrap
          </button>
          <button
            type="button"
            className={view === 'login' ? 'button button--prominent' : 'button button--secondary'}
            onClick={() => onViewChange('login')}
          >
            Login
          </button>
        </div>
      ) : null}

      <div className="auth-panel__content">
        <div className="auth-panel__header">
          <div>
            <p className="eyebrow">{isBootstrap ? 'Initial setup' : 'Authentication'}</p>
            <h2>{title}</h2>
            <p className="hint">{description}</p>
          </div>
          <div className="auth-badges">
            <span className="pill">API token</span>
            {authStatus?.bootstrapEnabled ? <span className="pill">Bootstrap enabled</span> : null}
            {authStatus?.oidcEnabled ? <span className="pill">OIDC available</span> : null}
            {authStatus?.oidcOAuthEnabled ? <span className="pill">OIDC redirect</span> : null}
          </div>
        </div>

        {authStatus && !authStatus.bootstrapEnabled && isBootstrap ? (
          <div className="empty-state">
            <h3>Bootstrap mode is unavailable</h3>
            <p className="hint">Set `BOOTSTRAP_ADMIN_TOKEN` on the server to enable bootstrap onboarding, or switch to the regular login screen.</p>
          </div>
        ) : (
          <div className="auth-form">
            <label className="field">
              <span>API base URL (optional)</span>
              <input
                value={apiBaseUrl}
                onChange={(event) => onAPIBaseURLChange(event.target.value)}
                placeholder="Leave empty to use this server"
              />
            </label>
            {authStatus?.oidcOAuthEnabled ? (
              <div className="inline-cluster">
                <button
                  type="button"
                  className="button button--prominent"
                  onClick={onStartOIDCLogin}
                  disabled={authSubmitting}
                >
                  {authSubmitting ? 'Redirecting…' : 'Sign in with OpenID Connect'}
                </button>
              </div>
            ) : null}
            {authStatus?.oidcOAuthEnabled ? (
              <p className="hint">Manual token entry remains available if your IdP or network blocks redirects.</p>
            ) : null}
            <label className="field">
              <span>{isBootstrap ? 'Bootstrap admin token' : 'Bearer token'}</span>
              <input
                type="password"
                value={authTokenDraft}
                onChange={(event) => onTokenChange(event.target.value)}
                placeholder={isBootstrap ? 'Enter bootstrap token' : 'Enter bearer token'}
              />
            </label>
            {authError ? <div className="status-banner"><span className="status-banner__error">{authError}</span></div> : null}
            <div className="inline-cluster">
              <button type="button" className="button button--prominent" onClick={onSubmit} disabled={authSubmitting}>
                {authSubmitting ? 'Signing in...' : isBootstrap ? 'Start bootstrap session' : 'Sign in'}
              </button>
              {authStatus?.bootstrapEnabled ? (
                <button
                  type="button"
                  className="button button--secondary"
                  onClick={() => onViewChange(isBootstrap ? 'login' : 'bootstrap')}
                  disabled={authSubmitting}
                >
                  {isBootstrap ? 'Use regular login instead' : 'Use bootstrap onboarding instead'}
                </button>
              ) : null}
            </div>
          </div>
        )}

        {authStatus && !authStatus.hasUsers && !authStatus.bootstrapEnabled ? (
          <div className="empty-state">
            <h3>No initial access method is configured</h3>
            <p className="hint">Configure `BOOTSTRAP_ADMIN_TOKEN` or OIDC on the server, then reload this page.</p>
          </div>
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

function DestinationAvatar({ account, compact = false }: { account: AccountRecord; compact?: boolean }) {
  return (
    <div className={`destination-avatar ${compact ? 'destination-avatar--compact' : ''}`}>
      <div className="destination-avatar__image" style={{ background: avatarBackground(account.color) }} aria-hidden="true">
        {account.username.replace('@', '').slice(0, 2).toUpperCase()}
      </div>
      <img className="destination-avatar__platform" src={`/icons/platforms/${account.provider}.svg`} alt={account.provider} />
    </div>
  )
}

function DestinationStack({ accounts }: { accounts: AccountRecord[] }) {
  return (
    <div className="destination-stack">
      {accounts.map((account) => (
        <DestinationAvatar key={account.id} account={account} compact />
      ))}
    </div>
  )
}

function defaultEditorDraft(date: Date, teamAccounts: AccountRecord[]) {
  const roundedDate = roundToNextSlot(date)
  return {
    title: '',
    content: '',
    scheduledAt: toInputDateTime(roundedDate),
    targetAccountIds: teamAccounts[0] ? [teamAccounts[0].id] : [],
    status: 'scheduled' as PostRecord['status'],
  }
}

function roundToNextSlot(date: Date) {
  const minutes = date.getMinutes()
  const remainder = minutes % SLOT_MINUTES
  if (remainder === 0) {
    return set(date, { seconds: 0, milliseconds: 0 })
  }
  return set(addMinutes(date, SLOT_MINUTES - remainder), { seconds: 0, milliseconds: 0 })
}

function toInputDateTime(date: Date) {
  return format(date, "yyyy-MM-dd'T'HH:mm")
}

function avatarBackground(color: string) {
  return `linear-gradient(135deg, ${color}, color-mix(in srgb, ${color} 40%, white))`
}

function charCounterClass(length: number, maxChars: number) {
  if (maxChars === 0) {
    return 'char-counter--idle'
  }
  const usage = length / maxChars
  if (usage >= 1) {
    return 'char-counter--danger'
  }
  if (usage >= 0.85) {
    return 'char-counter--warning'
  }
  return 'char-counter--good'
}

function groupPostsByDay(posts: PostRecord[]) {
  const groups = new Map<string, PostRecord[]>()
  for (const post of posts) {
    const key = format(parseISO(post.scheduledAt), 'yyyy-MM-dd')
    groups.set(key, [...(groups.get(key) ?? []), post])
  }
  return Array.from(groups.entries()).map(([key, value]) => ({ key, posts: value }))
}

function initialsFromName(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return '?'
  }
  if (parts.length === 1) {
    return parts[0]!.slice(0, 2).toUpperCase()
  }
  return `${parts[0]![0] ?? ''}${parts[parts.length - 1]![0] ?? ''}`.toUpperCase() || '?'
}

function getSystemTheme(): 'dark' | 'light' {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return 'dark'
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
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

function PostCard({
  post,
  active,
  onClick,
  onEdit,
  onDelete,
  accounts,
  isArchived = false,
}: {
  post: PostRecord
  active: boolean
  onClick: () => void
  onEdit: () => void
  onDelete: () => void
  accounts: AccountRecord[]
  isArchived?: boolean
}) {
  return (
    <article className={`post-card ${active ? 'post-card--active' : ''}`} onClick={onClick}>
      <div className="post-card__header">
        <span className="post-card__meta">{format(parseISO(post.scheduledAt), 'HH:mm')}</span>
        <DestinationStack accounts={sharedAccountLabels(post, accounts)} />
      </div>
      <h3 className="post-card__title">{post.title || 'Untitled Post'}</h3>
      <p className="post-card__content">{post.content}</p>
      {active && (
        <div className="inline-cluster" style={{ marginTop: '1rem' }}>
          <button type="button" className="button button--secondary" onClick={(e) => { e.stopPropagation(); onEdit(); }}>
            <Icon name="edit" className="inline-icon" />
            <span>Edit</span>
          </button>
          {!isArchived && (
            <button type="button" className="button button--secondary" onClick={(e) => { e.stopPropagation(); onDelete(); }}>
              <Icon name="trash" className="inline-icon" />
              <span>Delete</span>
            </button>
          )}
        </div>
      )}
    </article>
  )
}

function SocialPreview({
  account,
  content,
  scheduledAt,
  theme,
}: {
  account: AccountRecord
  content: string
  scheduledAt: string
  theme: 'dark' | 'light'
}) {
  return (
    <div className={`social-preview ${theme === 'dark' ? 'social-preview--dark' : ''}`}>
      <div className="social-preview__header">
        <div className="social-preview__avatar" style={{ background: avatarBackground(account.color) }} />
        <div className="social-preview__meta">
          <span className="social-preview__name">{account.name}</span>
          <span className="social-preview__handle">{account.username}</span>
        </div>
        <img
          src={`/icons/platforms/${account.provider}.svg`}
          alt={account.provider}
          style={{ marginLeft: 'auto', width: '20px', height: '20px' }}
        />
      </div>
      <div className="social-preview__body">
        {content || <span className="hint">Post content will appear here...</span>}
      </div>
      <div style={{ marginTop: '1rem', fontSize: '0.75rem', color: 'var(--text-dim)' }}>
        {scheduledAt ? format(parseISO(scheduledAt), 'PPpp') : 'Not scheduled'}
      </div>
    </div>
  )
}

export default App
