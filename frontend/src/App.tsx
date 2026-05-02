import { useCallback, useEffect, useMemo, useState } from 'react'
import type { CSSProperties, ReactNode } from 'react'
import { addMinutes, format, parseISO, set, startOfDay } from 'date-fns'

import { ApiError, createApiClient, requestAuthStatus } from './api'
import { initialSettings } from './data'
import { Icon } from './icons'
import { colorForProvider, toAccountRecord, toAuthStatusRecord, toPostRecord, toProviderInstanceRecord, toRuntimeConfigRecord, toTeamMemberRecord, toTeamRecord, toUserRecord } from './mappers'
import { postsForTeam, sharedAccountLabels, SLOT_MINUTES } from './schedule'
import type { AccountRecord, AppSection, AuthStatusRecord, CalendarViewMode, PostRecord, ProviderInstanceRecord, ProviderName, RuntimeConfigRecord, SettingsState, TeamRecord, TeamRole, UserRecord } from './types'

const SETTINGS_STORAGE_KEY = 'goloom-ui-settings'

function defaultAccountDraft(provider: ProviderName = 'mastodon', providerInstanceId = '') {
  return {
    provider,
    providerInstanceId,
    username: '',
    identifier: '',
    accessToken: '',
    refreshToken: '',
    appPassword: '',
  }
}

function defaultProviderInstanceDraft(provider: ProviderName = 'mastodon') {
  return {
    id: '',
    provider,
    name: '',
    instanceUrl: provider === 'bluesky' ? 'https://bsky.social' : '',
    clientId: '',
    clientSecret: '',
    scopes: provider === 'mastodon' || provider === 'friendica' ? 'read,write' : '',
    authorizationEndpoint: '',
    tokenEndpoint: '',
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
  const [authError, setAuthError] = useState<string | null>(null)
  const [authSubmitting, setAuthSubmitting] = useState(false)
  const [users, setUsers] = useState<UserRecord[]>([])
  const [teams, setTeams] = useState<TeamRecord[]>([])
  const [accounts, setAccounts] = useState<AccountRecord[]>([])
  const [posts, setPosts] = useState<PostRecord[]>([])
  const [providerInstances, setProviderInstances] = useState<ProviderInstanceRecord[]>([])
  const [runtimeConfig, setRuntimeConfig] = useState<RuntimeConfigRecord | null>(null)
  const [selectedTeamId, setSelectedTeamId] = useState('')
  const [expandedPostId, setExpandedPostId] = useState<string | null>(null)
  const [editingPostId, setEditingPostId] = useState<string | null>(null)
  const [composerMode, setComposerMode] = useState<'create' | 'edit'>('create')
  const [composerOpen, setComposerOpen] = useState(false)
  const [newTeamName, setNewTeamName] = useState('')
  const [newTeamDescription, setNewTeamDescription] = useState('')
  const [memberUserId, setMemberUserId] = useState('')
  const [memberRole, setMemberRole] = useState<TeamRole>('editor')
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole] = useState<'editor' | 'viewer'>('editor')
  const [migrateTargetTeamId, setMigrateTargetTeamId] = useState('')
  const [accountDraft, setAccountDraft] = useState(() => defaultAccountDraft())
  const [providerInstanceDraft, setProviderInstanceDraft] = useState(() => defaultProviderInstanceDraft())
  const [showProviderAdvancedSettings, setShowProviderAdvancedSettings] = useState(false)
  const [editorDraft, setEditorDraft] = useState(defaultEditorDraft(currentDate, []))
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [principalUser, setPrincipalUser] = useState<UserRecord | null>(null)

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

  const clearAuthenticatedState = useCallback((message?: string) => {
    setActiveConnection((current) => ({ ...current, bearerToken: '' }))
    setSettings((current) => ({
      ...current,
      general: { ...current.general, bearerToken: '' },
    }))
    setAuthTokenDraft('')
    setPrincipalUser(null)
    setUsers([])
    setTeams([])
    setAccounts([])
    setPosts([])
    setProviderInstances([])
    setRuntimeConfig(null)
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
      const meUser = toUserRecord(meResponse.user)
      setPrincipalUser(meUser)

      const [teamsResponse, usersResponse, providerInstancesResponse, runtimeConfigResponse] = await Promise.all([
        api.listTeams(),
        api.listUsers(),
        api.listProviderInstances(),
        meResponse.user.is_admin ? api.runtimeConfig() : Promise.resolve(null),
      ])

      const mappedUsers = (usersResponse.items ?? []).map(toUserRecord)
      const mappedProviderInstances = (providerInstancesResponse.items ?? []).map(toProviderInstanceRecord)

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

      setUsers(mappedUsers)
      setProviderInstances(mappedProviderInstances)
      setTeams(teamPayloads.map((payload) => payload.team))
      setAccounts(teamPayloads.flatMap((payload) => payload.accounts))
      setPosts(teamPayloads.flatMap((payload) => payload.posts))
      setRuntimeConfig(runtimeConfigResponse ? toRuntimeConfigRecord(runtimeConfigResponse) : null)
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

  const navigationItems: { id: AppSection; label: string; icon: 'calendar' | 'archive' | 'teams' | 'settings' | 'admin' }[] = [
    { id: 'calendar', label: 'Schedule', icon: 'calendar' },
    { id: 'archive', label: 'Archive', icon: 'archive' },
    { id: 'teams', label: 'Teams', icon: 'teams' },
    { id: 'settings', label: 'Settings', icon: 'settings' },
    { id: 'admin', label: 'Admin', icon: 'admin' },
  ]

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

  const migrateTargetOptions = useMemo(
    () => teams.filter((t) => !t.isPersonal && t.id !== selectedTeam?.id),
    [teams, selectedTeam],
  )

  const upcomingPosts = useMemo(() => {
    const baseline = startOfDay(currentDate)
    return teamPosts.filter((post) => post.status === 'scheduled' && parseISO(post.scheduledAt) >= baseline)
  }, [currentDate, teamPosts])

  const archivedPosts = useMemo(
    () => [...teamPosts].filter((post) => post.status === 'posted').sort((left, right) => parseISO(right.scheduledAt).getTime() - parseISO(left.scheduledAt).getTime()),
    [teamPosts],
  )

  const selectedPost = useMemo(() => posts.find((post) => post.id === expandedPostId) ?? null, [expandedPostId, posts])
  const editTargetPost = useMemo(() => posts.find((post) => post.id === editingPostId) ?? null, [editingPostId, posts])
  const selectedComposerAccounts = useMemo(() => teamAccounts.filter((account) => editorDraft.targetAccountIds.includes(account.id)), [editorDraft.targetAccountIds, teamAccounts])
  const availableUsers = useMemo(() => users.filter((user) => !selectedTeam?.members.some((member) => member.userId === user.id)), [selectedTeam, users])
  const providerInstanceOptions = useMemo(() => providerInstances.filter((instance) => instance.provider === accountDraft.provider), [accountDraft.provider, providerInstances])
  const accountProviderHints = useMemo(() => {
    switch (accountDraft.provider) {
      case 'bluesky':
        return {
          supportsOAuthRedirect: false,
          showUsername: false,
          showIdentifier: true,
          showAccessToken: false,
          showAppPassword: true,
          accessTokenLabel: 'Access token',
          helperText: 'Use a Bluesky identifier and app password. The backend will derive the account handle automatically.',
        }
      case 'friendica':
        return {
          supportsOAuthRedirect: false,
          showUsername: true,
          showIdentifier: false,
          showAccessToken: true,
          showAppPassword: false,
          accessTokenLabel: 'Access token',
          helperText: 'Provide the Friendica username and a usable access token for that account.',
        }
      default:
        return {
          supportsOAuthRedirect: true,
          showUsername: false,
          showIdentifier: false,
          showAccessToken: true,
          showAppPassword: false,
          accessTokenLabel: 'OAuth access token',
          helperText: 'Use the browser OAuth flow to authorize Mastodon and return to this dashboard automatically. Manual token entry still works as a fallback.',
        }
    }
  }, [accountDraft.provider])
  const providerInstanceHints = useMemo(() => {
    switch (providerInstanceDraft.provider) {
      case 'bluesky':
        return {
          showClientCredentials: false,
          showScopes: false,
          helperText: 'Only a name and optional PDS URL are needed for Bluesky instance registration.',
          supportsAdvancedOverrides: false,
        }
      case 'friendica':
        return {
          showClientCredentials: true,
          showScopes: true,
          helperText: 'Friendica does not support portable automatic app registration here, so enter client credentials manually if your instance provides them.',
          supportsAdvancedOverrides: false,
        }
      default:
        return {
          showClientCredentials: false,
          showScopes: false,
          helperText: 'Mastodon app credentials are registered automatically from the instance URL using the backend defaults.',
          supportsAdvancedOverrides: true,
        }
    }
  }, [providerInstanceDraft.provider])
  const effectiveMemberUserId = memberUserId || availableUsers[0]?.id || ''
  const effectiveProviderInstanceId = providerInstanceOptions.some((instance) => instance.id === accountDraft.providerInstanceId)
    ? accountDraft.providerInstanceId
    : (providerInstanceOptions[0]?.id ?? '')

  const maxChars = useMemo(() => {
    if (selectedComposerAccounts.length === 0) {
      return 0
    }
    return selectedComposerAccounts.reduce((lowest, account) => Math.min(lowest, account.maxChars), selectedComposerAccounts[0].maxChars)
  }, [selectedComposerAccounts])

  const composerAccent = useMemo(() => {
    if (selectedComposerAccounts.length === 0) {
      return '#8B5CF6'
    }
    const firstProvider = selectedComposerAccounts[0].provider
    return selectedComposerAccounts.every((account) => account.provider === firstProvider) ? colorForProvider(firstProvider) : '#8B5CF6'
  }, [selectedComposerAccounts])

  const remainingChars = maxChars ? maxChars - editorDraft.content.length : 0

  const metrics = useMemo(() => {
    const queued = posts.filter((post) => post.status === 'scheduled').length
    const failed = posts.filter((post) => post.status === 'failed').length
    return [
      { label: 'Queued posts', value: String(queued), tone: 'default' as const },
      { label: 'Connected accounts', value: String(accounts.length), tone: 'success' as const },
      { label: 'Failed deliveries', value: String(failed), tone: failed > 0 ? ('warning' as const) : ('success' as const) },
      { label: 'Active teams', value: String(teams.length), tone: 'default' as const },
    ]
  }, [accounts.length, posts, teams.length])

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

  async function addTeam() {
    if (!api || !newTeamName.trim()) {
      return
    }
    await runAction(async () => {
      const team = await api.createTeam({ name: newTeamName.trim(), description: newTeamDescription.trim() })
      setNewTeamName('')
      setNewTeamDescription('')
      setSelectedTeamId(team.id)
      await loadDashboard()
      setSection('teams')
    }, 'Team created')
  }

  async function addMemberToTeam() {
    if (!api || !selectedTeam || !effectiveMemberUserId) {
      return
    }
    await runAction(async () => {
      await api.addTeamMember(selectedTeam.id, { user_id: effectiveMemberUserId, role: memberRole })
      await loadDashboard()
    }, 'Team member added')
  }

  async function removeMemberFromTeam(userID: string) {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      await api.removeTeamMember(selectedTeam.id, userID)
      await loadDashboard()
    }, 'Team member removed')
  }

  async function sendTeamInvite() {
    if (!api || !selectedTeam || !inviteEmail.trim()) {
      return
    }
    await runAction(async () => {
      const response = await api.createTeamInvitation(selectedTeam.id, { email: inviteEmail.trim(), role: inviteRole })
      const link = `${window.location.origin}${window.location.pathname}?invite=${encodeURIComponent(response.token)}`
      setStatusMessage(`Share this link with ${inviteEmail.trim()}: ${link}`)
      setInviteEmail('')
    }, 'Invitation created')
  }

  async function migrateAccountToTeam(accountId: string) {
    if (!api || !selectedTeam || !migrateTargetTeamId) {
      return
    }
    await runAction(async () => {
      await api.migrateAccount(selectedTeam.id, accountId, { target_team_id: migrateTargetTeamId })
      await loadDashboard()
    }, 'Account migrated to team')
  }

  async function addAccountToTeam() {
    if (!api || !selectedTeam || !effectiveProviderInstanceId) {
      return
    }
    const payload = {
      provider: accountDraft.provider,
      provider_instance_id: effectiveProviderInstanceId,
      username: accountDraft.username || undefined,
      identifier: accountDraft.identifier || undefined,
      access_token: accountDraft.accessToken || undefined,
      refresh_token: accountDraft.refreshToken || undefined,
      app_password: accountDraft.appPassword || undefined,
    }
    await runAction(async () => {
      await api.createAccount(selectedTeam.id, payload)
      setAccountDraft(defaultAccountDraft(accountDraft.provider, providerInstanceOptions[0]?.id ?? ''))
      await loadDashboard()
    }, 'Account connected')
  }

  async function startMastodonOAuth() {
    if (!api || !selectedTeam || !effectiveProviderInstanceId) {
      return
    }

    setSyncing(true)
    setError(null)
    setStatusMessage(null)

    try {
      const returnTo = new URL(window.location.href)
      returnTo.hash = ''
      returnTo.searchParams.set('team', selectedTeam.id)
      returnTo.searchParams.delete('oauth_status')
      returnTo.searchParams.delete('oauth_provider')
      returnTo.searchParams.delete('oauth_message')

      const response = await api.startMastodonOAuth(selectedTeam.id, {
        provider_instance_id: effectiveProviderInstanceId,
        return_to: returnTo.toString(),
      })
      window.location.assign(response.authorization_url)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to start Mastodon OAuth')
      setSyncing(false)
    }
  }

  async function removeAccountFromTeam(accountID: string) {
    if (!api || !selectedTeam) {
      return
    }
    await runAction(async () => {
      await api.deleteAccount(selectedTeam.id, accountID)
      await loadDashboard()
    }, 'Account removed')
  }

  async function saveProviderInstance() {
    if (!api || !principalUser || principalUser.globalRole !== 'admin') {
      return
    }
    const provider = providerInstanceDraft.provider
    const trimmedScopes = providerInstanceDraft.scopes.split(',').map((scope) => scope.trim()).filter(Boolean)
    const payload = {
      provider,
      name: providerInstanceDraft.name.trim(),
      instance_url: providerInstanceDraft.instanceUrl.trim(),
      client_id: provider === 'friendica' || (provider === 'mastodon' && showProviderAdvancedSettings)
        ? (providerInstanceDraft.clientId.trim() || undefined)
        : undefined,
      client_secret: provider === 'friendica' || (provider === 'mastodon' && showProviderAdvancedSettings)
        ? (providerInstanceDraft.clientSecret.trim() || undefined)
        : undefined,
      scopes: provider === 'friendica' || (provider === 'mastodon' && showProviderAdvancedSettings)
        ? trimmedScopes
        : undefined,
      authorization_endpoint: provider === 'mastodon' && showProviderAdvancedSettings
        ? (providerInstanceDraft.authorizationEndpoint.trim() || undefined)
        : undefined,
      token_endpoint: provider === 'mastodon' && showProviderAdvancedSettings
        ? (providerInstanceDraft.tokenEndpoint.trim() || undefined)
        : undefined,
    }
    await runAction(async () => {
      if (providerInstanceDraft.id) {
        await api.updateProviderInstance(providerInstanceDraft.id, payload)
      } else {
        await api.createProviderInstance(payload)
      }
      setProviderInstanceDraft(defaultProviderInstanceDraft())
      setShowProviderAdvancedSettings(false)
      await loadDashboard()
    }, providerInstanceDraft.id ? 'Provider instance updated' : 'Provider instance registered')
  }

  async function updateUserRole(userId: string, nextRole: UserRecord['globalRole']) {
    if (!api || !principalUser || principalUser.globalRole !== 'admin') {
      return
    }
    await runAction(async () => {
      await api.updateUser(userId, { is_admin: nextRole === 'admin' })
      await loadDashboard()
    }, 'User role updated')
  }

  const selectedTeamName = selectedTeam?.name ?? 'No team selected'
  const connectionReady = Boolean(api)

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
        />
      </AuthShell>
    )
  }

  return (
    <div className="app-shell" data-theme={theme}>
      <aside className="nav-rail">
        <div className="nav-rail__brand" title="goloom">
          <span>G</span>
        </div>

        <nav className="nav-rail__items">
          {navigationItems.map((item) => (
            <button
              key={item.id}
              type="button"
              className={`nav-rail__button ${section === item.id ? 'nav-rail__button--active' : ''}`}
              onClick={() => setSection(item.id)}
              title={item.label}
              aria-label={item.label}
            >
              <Icon name={item.icon} className="inline-icon" />
            </button>
          ))}
        </nav>

        <div className="nav-rail__footer">
          <button
            type="button"
            className="nav-rail__button"
            onClick={() => setTheme((current) => (current === 'dark' ? 'light' : 'dark'))}
            title={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
            aria-label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
          >
            <Icon name={theme === 'dark' ? 'sun' : 'moon'} className="inline-icon" />
          </button>
        </div>
      </aside>

      <main className="content">
        <header className="hero-shell">
          <div className="hero-shell__copy">
            <p className="eyebrow">Social publishing</p>
            <h1>Upcoming posts</h1>
            <p className="hint">
              {connectionReady
                ? `API-connected planning space for ${selectedTeamName}.`
                : 'Enter a bearer token in Settings to connect the dashboard.'}
            </p>
          </div>

          <div className="hero-shell__actions">
            <label className="team-picker">
              <span className="eyebrow">Team</span>
              <select value={effectiveSelectedTeamId} onChange={(event) => setSelectedTeamId(event.target.value)} disabled={teams.length === 0}>
                {teams.length === 0 ? <option value="">No team loaded</option> : teams.map((team) => <option key={team.id} value={team.id}>{team.name}</option>)}
              </select>
            </label>

            <button type="button" className="icon-button icon-button--primary icon-button--field" onClick={openCreateComposer} aria-label="Compose post" title="Compose post" disabled={!selectedTeam || syncing}>
              <Icon name="plus" className="button__icon" />
            </button>
          </div>
        </header>

        {(error || statusMessage || loading) && (
          <section className="status-banner">
            {loading ? <span className="hint">Loading backend data…</span> : null}
            {statusMessage ? <span className="status-banner__success">{statusMessage}</span> : null}
            {error ? <span className="status-banner__error">{error}</span> : null}
          </section>
        )}

        <section className="metric-strip">
          {metrics.map((metric) => (
            <article key={metric.label} className={`metric-card metric-card--${metric.tone}`}>
              <span>{metric.label}</span>
              <strong>{metric.value}</strong>
            </article>
          ))}
        </section>

        {section === 'calendar' && (
          <section className="timeline-feed">
            <div className="timeline-feed__header">
              <div>
                <p className="eyebrow">Timeline</p>
                <h2>Upcoming</h2>
              </div>
              <div className="inline-cluster">
                <DestinationStack accounts={teamAccounts.slice(0, 4)} />
                <span className="hint">{upcomingPosts.length} posts queued</span>
              </div>
            </div>

            {!connectionReady ? (
              <div className="empty-state">
                <h3>Backend connection required</h3>
                <p className="hint">Open Settings, add a bearer token, then connect. Leave the API base URL empty to use this server.</p>
              </div>
            ) : upcomingPosts.length === 0 ? (
              <div className="empty-state">
                <h3>No upcoming posts</h3>
                <p className="hint">Create a post to start building your publishing timeline.</p>
                <button type="button" className="button button--prominent" onClick={openCreateComposer} disabled={!selectedTeam}>
                  Compose post
                </button>
              </div>
            ) : (
              groupPostsByDay(upcomingPosts).map((group) => (
                <section key={group.key} className="timeline-day">
                  <div className="timeline-day__header">
                    <div>
                      <p className="eyebrow">{format(parseISO(group.posts[0].scheduledAt), 'EEEE')}</p>
                      <h3>{format(parseISO(group.posts[0].scheduledAt), 'd MMMM yyyy')}</h3>
                    </div>
                    <span className="timeline-day__count">{group.posts.length}</span>
                  </div>

                  <div className="timeline-day__posts">
                    {group.posts.map((post) => (
                      <article
                        key={post.id}
                        className={`timeline-post ${expandedPostId === post.id ? 'timeline-post--active' : ''}`}
                        onClick={() => setExpandedPostId((current) => (current === post.id ? null : post.id))}
                      >
                        <div className="timeline-post__time">
                          <span>{format(parseISO(post.scheduledAt), 'HH:mm')}</span>
                          <small>{post.durationMinutes} min</small>
                        </div>

                        <div className="timeline-post__body">
                          <div className="timeline-post__title">
                            <strong>{post.title}</strong>
                            <DestinationStack accounts={sharedAccountLabels(post, accounts)} />
                          </div>
                          <p>{post.content}</p>

                          <div className="timeline-post__expanded">
                            <div className="inline-cluster timeline-post__actions">
                              <button type="button" className="icon-button" onClick={(event) => {
                                event.stopPropagation()
                                openEditor(post.id)
                              }} aria-label="Edit post">
                                <Icon name="edit" className="button__icon" />
                              </button>
                              <button type="button" className="icon-button" onClick={(event) => {
                                event.stopPropagation()
                                void deletePost(post.id)
                              }} aria-label="Delete post">
                                <Icon name="trash" className="button__icon" />
                              </button>
                            </div>
                            {expandedPostId === post.id && (
                              <span className="hint">{sharedAccountLabels(post, accounts).map((account) => account.name).join(', ')}</span>
                            )}
                          </div>
                        </div>
                      </article>
                    ))}
                  </div>
                </section>
              ))
            )}

            {selectedPost && (
              <aside className="selected-post">
                <p className="eyebrow">Focused post</p>
                <h3>{selectedPost.title}</h3>
                <p>{selectedPost.content}</p>
                <div className="inline-cluster">
                  <span className="hint">{format(parseISO(selectedPost.scheduledAt), 'PPpp')}</span>
                  <DestinationStack accounts={sharedAccountLabels(selectedPost, accounts)} />
                </div>
              </aside>
            )}
          </section>
        )}

        {section === 'archive' && (
          <section className="timeline-feed">
            <div className="timeline-feed__header">
              <div>
                <p className="eyebrow">Published</p>
                <h2>Archive</h2>
              </div>
              <span className="hint">{archivedPosts.length} published posts</span>
            </div>

            {archivedPosts.length === 0 ? (
              <div className="empty-state">
                <h3>No published posts yet</h3>
                <p className="hint">Published content will appear here with direct platform links when available.</p>
              </div>
            ) : (
              <div className="archive-grid">
                {archivedPosts.map((post) => (
                  <article key={post.id} className="archive-card">
                    <div className="archive-card__header">
                      <div>
                        <p className="eyebrow">{format(parseISO(post.scheduledAt), 'd MMM yyyy')}</p>
                        <h3>{post.title}</h3>
                      </div>
                      <DestinationLinkStack post={post} accounts={sharedAccountLabels(post, accounts)} />
                    </div>
                    <p>{post.content}</p>
                    <div className="archive-card__footer">
                      <span className="hint">{format(parseISO(post.scheduledAt), 'PPpp')}</span>
                      <button type="button" className="button button--secondary button--icon-label" onClick={() => openEditor(post.id)}>
                        <Icon name="edit" className="inline-icon" />
                        <span>Edit post</span>
                      </button>
                    </div>
                  </article>
                ))}
              </div>
            )}
          </section>
        )}

        {section === 'teams' && (
          <section className="page-grid">
            <div className="panel">
              <div className="panel__header">
                <div>
                  <p className="eyebrow">Teams</p>
                  <h2>{selectedTeam?.name ?? 'No team selected'}</h2>
                </div>
                <button type="button" className="button button--secondary button--icon-label" onClick={() => void loadDashboard()} disabled={!api || syncing}>
                  <Icon name="chevron-right" className="inline-icon" />
                  <span>Refresh</span>
                </button>
              </div>

              <div className="team-grid">
                {teams.map((team) => (
                  <button
                    key={team.id}
                    type="button"
                    className={`team-card ${team.id === selectedTeamId ? 'team-card--active' : ''}`}
                    onClick={() => setSelectedTeamId(team.id)}
                  >
                    <strong>{team.name}{team.isPersonal ? ' · Personal' : ''}</strong>
                    <span>{team.description || 'No description'}</span>
                    <small>{team.members.length} members · {team.accountIds.length} accounts</small>
                  </button>
                ))}
              </div>

              <div className="split-grid">
                <section className="subpanel">
                  <h3>Create team</h3>
                  <label className="field">
                    <span>Name</span>
                    <input value={newTeamName} onChange={(event) => setNewTeamName(event.target.value)} />
                  </label>
                  <label className="field">
                    <span>Description</span>
                    <textarea rows={3} value={newTeamDescription} onChange={(event) => setNewTeamDescription(event.target.value)} />
                  </label>
                  <button type="button" className="button button--icon-label" onClick={() => void addTeam()} disabled={!api || syncing}>
                    <Icon name="plus" className="inline-icon" />
                    <span>Add team</span>
                  </button>
                </section>

                {!selectedTeam?.isPersonal ? (
                  <section className="subpanel">
                    <h3>Invite by email</h3>
                    <p className="hint">Creates a one-time link (7 days). Send it to someone who already has a goloom account with that email.</p>
                    <label className="field">
                      <span>Email</span>
                      <input type="email" value={inviteEmail} onChange={(event) => setInviteEmail(event.target.value)} placeholder="colleague@example.com" />
                    </label>
                    <label className="field">
                      <span>Role</span>
                      <select value={inviteRole} onChange={(event) => setInviteRole(event.target.value as 'editor' | 'viewer')}>
                        <option value="editor">editor</option>
                        <option value="viewer">viewer</option>
                      </select>
                    </label>
                    <button type="button" className="button button--icon-label" onClick={() => void sendTeamInvite()} disabled={!selectedTeam || !inviteEmail.trim() || syncing}>
                      <Icon name="plus" className="inline-icon" />
                      <span>Create invitation</span>
                    </button>
                  </section>
                ) : null}

                <section className="subpanel">
                  <h3>Members</h3>
                  {selectedTeam?.isPersonal ? (
                    <p className="hint">This is your personal workspace. Create a shared team to invite collaborators.</p>
                  ) : null}
                  <div className="entity-list">
                      {selectedTeam?.members.map((member) => {
                      const user = users.find((candidate) => candidate.id === member.userId)
                      if (!user) {
                        return null
                      }
                      return (
                        <article key={member.userId} className="entity-card">
                          <div>
                            <strong>{user.name}</strong>
                            <p>{user.email}</p>
                          </div>
                          <div className="inline-cluster">
                            <span className="pill">{member.role}</span>
                            {!selectedTeam?.isPersonal ? (
                              <button type="button" className="icon-button" onClick={() => void removeMemberFromTeam(member.userId)} aria-label="Remove member" disabled={syncing}>
                                <Icon name="trash" className="button__icon" />
                              </button>
                            ) : null}
                          </div>
                        </article>
                      )
                    })}
                  </div>
                  {!selectedTeam?.isPersonal ? (
                    <div className="inline-form">
                      <select value={effectiveMemberUserId} onChange={(event) => setMemberUserId(event.target.value)} disabled={availableUsers.length === 0}>
                        {availableUsers.length === 0 ? <option value="">No users available</option> : availableUsers.map((user) => <option key={user.id} value={user.id}>{user.name}</option>)}
                      </select>
                      <select value={memberRole} onChange={(event) => setMemberRole(event.target.value as TeamRole)}>
                        <option value="owner">owner</option>
                        <option value="editor">editor</option>
                        <option value="viewer">viewer</option>
                      </select>
                      <button type="button" className="button button--icon-label" onClick={() => void addMemberToTeam()} disabled={!selectedTeam || !effectiveMemberUserId || syncing}>
                        <Icon name="plus" className="inline-icon" />
                        <span>Add user</span>
                      </button>
                    </div>
                  ) : null}
                </section>
              </div>
            </div>

            <aside className="panel">
              <div className="panel__header">
                <div>
                  <p className="eyebrow">Destinations</p>
                  <h2>Connected accounts</h2>
                </div>
              </div>

              {selectedTeam?.isPersonal && migrateTargetOptions.length > 0 ? (
                <div className="field" style={{ marginBottom: '1rem' }}>
                  <span>Move account to team</span>
                  <div className="inline-cluster">
                    <select value={migrateTargetTeamId} onChange={(event) => setMigrateTargetTeamId(event.target.value)}>
                      <option value="">Select team…</option>
                      {migrateTargetOptions.map((t) => (
                        <option key={t.id} value={t.id}>{t.name}</option>
                      ))}
                    </select>
                  </div>
                  <p className="hint">Choose a destination, then use Move on each account. Scheduled posts that target only that account move with it.</p>
                </div>
              ) : null}

              <div className="entity-list">
                {teamAccounts.map((account) => (
                  <article key={account.id} className="entity-card">
                    <div className="inline-cluster">
                      <DestinationAvatar account={account} />
                      <div>
                        <strong>{account.name}</strong>
                        <p>{account.provider} · {account.username}</p>
                      </div>
                    </div>
                    <div className="inline-cluster">
                      {selectedTeam?.isPersonal && migrateTargetTeamId ? (
                        <button type="button" className="button button--secondary" onClick={() => void migrateAccountToTeam(account.id)} disabled={syncing}>
                          Move
                        </button>
                      ) : null}
                      <button type="button" className="icon-button" onClick={() => void removeAccountFromTeam(account.id)} aria-label="Remove account" disabled={syncing}>
                        <Icon name="trash" className="button__icon" />
                      </button>
                    </div>
                  </article>
                ))}
              </div>

              <div className="divider" />

              <div className="detail-list">
                <h3>Connect account</h3>
                <label className="field">
                  <span>Provider</span>
                  <select value={accountDraft.provider} onChange={(event) => {
                    const provider = event.target.value as ProviderName
                    setShowProviderAdvancedSettings(false)
                    setAccountDraft(defaultAccountDraft(provider, providerInstances.find((instance) => instance.provider === provider)?.id ?? ''))
                  }}>
                    <option value="bluesky">Bluesky</option>
                    <option value="friendica">Friendica</option>
                    <option value="mastodon">Mastodon</option>
                  </select>
                </label>
                <label className="field">
                  <span>Registered instance</span>
                  <select value={effectiveProviderInstanceId} onChange={(event) => setAccountDraft((current) => ({ ...current, providerInstanceId: event.target.value }))}>
                    <option value="">Select instance</option>
                    {providerInstanceOptions.map((instance) => (
                      <option key={instance.id} value={instance.id}>
                        {instance.name}
                      </option>
                    ))}
                  </select>
                </label>
                {accountProviderHints.supportsOAuthRedirect && (
                  <div className="field">
                    <span>OAuth flow</span>
                    <button
                      type="button"
                      className="button button--prominent button--icon-label"
                      onClick={() => void startMastodonOAuth()}
                      disabled={!selectedTeam || !effectiveProviderInstanceId || syncing}
                    >
                      <Icon name="plus" className="inline-icon" />
                      <span>Authorize with Mastodon</span>
                    </button>
                  </div>
                )}
                {accountProviderHints.showUsername && (
                  <label className="field">
                    <span>Username</span>
                    <input value={accountDraft.username} onChange={(event) => setAccountDraft((current) => ({ ...current, username: event.target.value }))} />
                  </label>
                )}
                {accountProviderHints.showIdentifier && (
                  <>
                    <label className="field">
                      <span>Identifier</span>
                      <input value={accountDraft.identifier} onChange={(event) => setAccountDraft((current) => ({ ...current, identifier: event.target.value }))} />
                    </label>
                  </>
                )}
                {accountProviderHints.showAppPassword && (
                  <>
                    <label className="field">
                      <span>App password</span>
                      <input type="password" value={accountDraft.appPassword} onChange={(event) => setAccountDraft((current) => ({ ...current, appPassword: event.target.value }))} />
                    </label>
                  </>
                )}
                {accountProviderHints.showAccessToken && (
                  <label className="field">
                    <span>{accountProviderHints.accessTokenLabel}</span>
                    <input type="password" value={accountDraft.accessToken} onChange={(event) => setAccountDraft((current) => ({ ...current, accessToken: event.target.value }))} />
                  </label>
                )}
                <p className="hint">{accountProviderHints.helperText}</p>
                <button
                  type="button"
                  className="button button--icon-label"
                  onClick={() => void addAccountToTeam()}
                  disabled={!selectedTeam || !effectiveProviderInstanceId || syncing}
                >
                  <Icon name="plus" className="inline-icon" />
                  <span>{accountProviderHints.supportsOAuthRedirect ? 'Connect with manual token' : 'Connect account'}</span>
                </button>
              </div>
            </aside>
          </section>
        )}

        {section === 'settings' && (
          <section className="page-grid">
            <div className="panel">
              <div className="settings-grid">
                <SettingsCard title="Connection">
                  <label className="field">
                    <span>API base URL (optional)</span>
                    <input value={settings.general.apiBaseUrl} onChange={(event) => updateAPIBaseURL(event.target.value)} />
                  </label>
                  <label className="field">
                    <span>Bearer token</span>
                    <input type="password" value={settings.general.bearerToken} onChange={(event) => setSettings((current) => ({ ...current, general: { ...current.general, bearerToken: event.target.value } }))} />
                  </label>
                  <p className="hint">Leave the API base URL empty to use this server. Update the token here only if you want to switch sessions manually.</p>
                  <div className="inline-cluster">
                    <button type="button" className="button button--prominent" onClick={connectBackend}>
                      Apply session
                    </button>
                    <button type="button" className="button button--secondary" onClick={() => void loadDashboard()} disabled={!api || syncing}>
                      Refresh data
                    </button>
                    <button type="button" className="button button--secondary" onClick={() => clearAuthenticatedState('Signed out')}>
                      Sign out
                    </button>
                  </div>
                </SettingsCard>

                <SettingsCard title="Workspace preferences">
                  <label className="field">
                    <span>Timezone</span>
                    <input value={settings.general.timezone} onChange={(event) => setSettings((current) => ({ ...current, general: { ...current.general, timezone: event.target.value } }))} />
                  </label>
                  <label className="field">
                    <span>Default view</span>
                    <select value={settings.general.defaultCalendarView} onChange={(event) => setSettings((current) => ({ ...current, general: { ...current.general, defaultCalendarView: event.target.value as CalendarViewMode } }))}>
                      <option value="month">month</option>
                      <option value="week">week</option>
                      <option value="day">day</option>
                    </select>
                  </label>
                  <label className="field">
                    <span>Slot minutes</span>
                    <input type="number" value={settings.general.slotMinutes} onChange={(event) => setSettings((current) => ({ ...current, general: { ...current.general, slotMinutes: Number(event.target.value) || 30 } }))} />
                  </label>
                </SettingsCard>

                <SettingsCard title="Provider overview">
                  {(Object.entries(settings.providers) as [ProviderName, SettingsState['providers'][ProviderName]][]).map(([provider, providerSetting]) => (
                    <div key={provider} className="provider-row">
                      <div className="inline-cluster">
                        <img className="platform-inline-icon" src={`/icons/platforms/${provider}.svg`} alt={provider} />
                        <div>
                          <strong>{provider}</strong>
                          <p>{providerInstances.filter((instance) => instance.provider === provider).length} registered instances</p>
                        </div>
                      </div>
                      <span className="pill">{providerSetting.defaultMaxChars} chars</span>
                    </div>
                  ))}
                </SettingsCard>
              </div>
            </div>

            <aside className="panel">
              <div className="detail-list">
                <p className="eyebrow">Runtime snapshot</p>
                <h2>Backend configuration</h2>
                {runtimeConfig ? (
                  <dl>
                    <div>
                      <dt>HTTP bind</dt>
                      <dd>{runtimeConfig.general.httpAddr}</dd>
                    </div>
                    <div>
                      <dt>Rate limit</dt>
                      <dd>{runtimeConfig.security.rateLimitPerMinute}/min</dd>
                    </div>
                    <div>
                      <dt>Scheduler</dt>
                      <dd>{runtimeConfig.scheduler.pollInterval} · {runtimeConfig.scheduler.workers} workers</dd>
                    </div>
                    <div>
                      <dt>OIDC</dt>
                      <dd>{runtimeConfig.oidc.enabled ? 'Enabled' : 'Disabled'}</dd>
                    </div>
                  </dl>
                ) : (
                  <p className="hint">Runtime configuration appears here for admin users after the backend connection is established.</p>
                )}
              </div>
            </aside>
          </section>
        )}

        {section === 'admin' && (
          <section className="page-grid">
            {principalUser?.globalRole !== 'admin' ? (
              <div className="empty-state">
                <h3>Administrator access required</h3>
                <p className="hint">This area manages provider instances, client credentials, and global user roles.</p>
              </div>
            ) : (
              <>
                <div className="panel">
                  <div className="split-grid">
                    <section className="subpanel">
                      <h3>Provider instances</h3>
                      <div className="entity-list">
                        {providerInstances.map((instance) => (
                          <article key={instance.id} className="entity-card">
                            <div>
                              <strong>{instance.name}</strong>
                              <p>{instance.provider} · {instance.instanceUrl}</p>
                            </div>
                            <button
                              type="button"
                              className="button button--secondary button--icon-label"
                              onClick={() => {
                                setProviderInstanceDraft({
                                  id: instance.id,
                                  provider: instance.provider,
                                  name: instance.name,
                                  instanceUrl: instance.instanceUrl,
                                  clientId: instance.clientId,
                                  clientSecret: '',
                                  scopes: instance.scopes.join(','),
                                  authorizationEndpoint: instance.authorizationEndpoint,
                                  tokenEndpoint: instance.tokenEndpoint,
                                })
                                setShowProviderAdvancedSettings(instance.provider === 'mastodon')
                              }}
                            >
                              <Icon name="edit" className="inline-icon" />
                              <span>Edit</span>
                            </button>
                          </article>
                        ))}
                      </div>
                    </section>

                    <section className="subpanel">
                      <h3>{providerInstanceDraft.id ? 'Edit provider instance' : 'Register provider instance'}</h3>
                      <label className="field">
                        <span>Provider</span>
                        <select value={providerInstanceDraft.provider} onChange={(event) => {
                          setShowProviderAdvancedSettings(false)
                          setProviderInstanceDraft(defaultProviderInstanceDraft(event.target.value as ProviderName))
                        }}>
                          <option value="bluesky">Bluesky</option>
                          <option value="friendica">Friendica</option>
                          <option value="mastodon">Mastodon</option>
                        </select>
                      </label>
                      <label className="field">
                        <span>Name</span>
                        <input value={providerInstanceDraft.name} onChange={(event) => setProviderInstanceDraft((current) => ({ ...current, name: event.target.value }))} />
                      </label>
                      <label className="field">
                        <span>Instance URL</span>
                        <input value={providerInstanceDraft.instanceUrl} onChange={(event) => setProviderInstanceDraft((current) => ({ ...current, instanceUrl: event.target.value }))} />
                      </label>
                      {providerInstanceHints.supportsAdvancedOverrides && (
                        <div className="inline-cluster">
                          <button
                            type="button"
                            className="button button--secondary"
                            onClick={() => setShowProviderAdvancedSettings((current) => !current)}
                          >
                            {showProviderAdvancedSettings ? 'Hide advanced settings' : 'Show advanced settings'}
                          </button>
                        </div>
                      )}
                      {(providerInstanceHints.showClientCredentials || (providerInstanceDraft.provider === 'mastodon' && showProviderAdvancedSettings)) && (
                        <>
                          <label className="field">
                            <span>Client ID</span>
                            <input value={providerInstanceDraft.clientId} onChange={(event) => setProviderInstanceDraft((current) => ({ ...current, clientId: event.target.value }))} />
                          </label>
                          <label className="field">
                            <span>Client secret</span>
                            <input type="password" value={providerInstanceDraft.clientSecret} onChange={(event) => setProviderInstanceDraft((current) => ({ ...current, clientSecret: event.target.value }))} />
                          </label>
                        </>
                      )}
                      {(providerInstanceHints.showScopes || (providerInstanceDraft.provider === 'mastodon' && showProviderAdvancedSettings)) && (
                        <label className="field">
                          <span>Scopes</span>
                          <input value={providerInstanceDraft.scopes} onChange={(event) => setProviderInstanceDraft((current) => ({ ...current, scopes: event.target.value }))} />
                        </label>
                      )}
                      {providerInstanceDraft.provider === 'mastodon' && showProviderAdvancedSettings && (
                        <>
                          <label className="field">
                            <span>Authorization endpoint override</span>
                            <input value={providerInstanceDraft.authorizationEndpoint} onChange={(event) => setProviderInstanceDraft((current) => ({ ...current, authorizationEndpoint: event.target.value }))} />
                          </label>
                          <label className="field">
                            <span>Token endpoint override</span>
                            <input value={providerInstanceDraft.tokenEndpoint} onChange={(event) => setProviderInstanceDraft((current) => ({ ...current, tokenEndpoint: event.target.value }))} />
                          </label>
                        </>
                      )}
                      <p className="hint">{providerInstanceHints.helperText}</p>
                      <div className="inline-cluster">
                        <button type="button" className="button button--prominent" onClick={() => void saveProviderInstance()} disabled={syncing}>
                          {providerInstanceDraft.id ? 'Save instance' : 'Register instance'}
                        </button>
                        {providerInstanceDraft.id && (
                          <button type="button" className="button button--secondary" onClick={() => {
                            setProviderInstanceDraft(defaultProviderInstanceDraft())
                            setShowProviderAdvancedSettings(false)
                          }}>
                            Reset form
                          </button>
                        )}
                      </div>
                    </section>
                  </div>
                </div>

                <aside className="panel">
                  <div className="detail-list">
                    <p className="eyebrow">User directory</p>
                    <h2>Global roles</h2>
                    <div className="entity-list">
                      {users.map((user) => (
                        <article key={user.id} className="entity-card">
                          <div>
                            <strong>{user.name}</strong>
                            <p>{user.email}</p>
                          </div>
                          <select value={user.globalRole} onChange={(event) => void updateUserRole(user.id, event.target.value as UserRecord['globalRole'])}>
                            <option value="admin">admin</option>
                            <option value="member">member</option>
                          </select>
                        </article>
                      ))}
                    </div>
                  </div>
                </aside>
              </>
            )}
          </section>
        )}
      </main>

      {composerOpen && (
        <div className="modal-backdrop" role="presentation" onClick={closeComposer}>
          <div className="composer-modal" role="dialog" aria-modal="true" onClick={(event) => event.stopPropagation()} style={{ '--composer-accent': composerAccent } as CSSProperties}>
            <div className="composer-modal__header">
              <div>
                <p className="eyebrow">Composer</p>
                <h2>{composerMode === 'edit' ? 'Edit post' : 'Create post'}</h2>
              </div>
              <div className="inline-cluster">
                {composerMode === 'edit' && editingPostId && (
                  <button type="button" className="icon-button" onClick={() => void deletePost(editingPostId)} aria-label="Delete post">
                    <Icon name="trash" className="button__icon" />
                  </button>
                )}
                <button type="button" className="icon-button" onClick={closeComposer} aria-label="Close composer">
                  <Icon name="chevron-right" className="button__icon" />
                </button>
              </div>
            </div>

            <div className="composer-modal__content">
              <div className="composer-editor">
                <label className="field">
                  <span>Title</span>
                  <input value={editorDraft.title} onChange={(event) => setEditorDraft((current) => ({ ...current, title: event.target.value }))} />
                </label>

                <label className="field field--full">
                  <span>Message</span>
                  <textarea rows={12} value={editorDraft.content} onChange={(event) => setEditorDraft((current) => ({ ...current, content: event.target.value }))} />
                </label>

                <div className="composer-toolbar">
                  <div className={`char-counter ${charCounterClass(editorDraft.content.length, maxChars)}`}>
                    <strong>{editorDraft.content.length}</strong>
                    <span>/ {maxChars || 'select a destination'}</span>
                    {maxChars > 0 && <small>{remainingChars} remaining</small>}
                  </div>
                  <DestinationStack accounts={selectedComposerAccounts} />
                </div>

                <label className="field">
                  <span>Scheduled at</span>
                  <input type="datetime-local" value={editorDraft.scheduledAt} onChange={(event) => setEditorDraft((current) => ({ ...current, scheduledAt: event.target.value }))} />
                </label>

                <div className="field field--full">
                  <span>Destinations</span>
                  <div className="checkbox-grid">
                    {teamAccounts.map((account) => (
                      <label key={account.id} className="checkbox-card">
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
                </div>
              </div>

              <aside className="composer-preview">
                <p className="eyebrow">Live preview</p>
                <article className="preview-card">
                  <div className="preview-card__header">
                    <DestinationStack accounts={selectedComposerAccounts} />
                    <span>{editorDraft.scheduledAt ? format(new Date(editorDraft.scheduledAt), 'PPpp') : 'No time selected'}</span>
                  </div>
                  <strong>{editorDraft.title || 'Untitled post'}</strong>
                  <p>{editorDraft.content || 'Start typing to preview the final post.'}</p>
                </article>
              </aside>
            </div>

            <div className="composer-modal__footer">
              <button type="button" className="button button--secondary" onClick={closeComposer}>
                Cancel
              </button>
              <button type="button" className="button button--prominent button--icon-label" disabled={syncing || editorDraft.targetAccountIds.length === 0 || (maxChars > 0 && editorDraft.content.length > maxChars)} onClick={() => void handleSavePost()}>
                <Icon name="calendar" className="inline-icon" />
                <span>{composerMode === 'edit' ? 'Save changes' : 'Schedule post'}</span>
              </button>
            </div>
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
}) {
  const isBootstrap = view === 'bootstrap'
  const title = isBootstrap ? 'Bootstrap onboarding' : 'Sign in'
  const description = isBootstrap
    ? 'Use the bootstrap admin token configured on the server for the first administrator session.'
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

function DestinationLinkStack({ post, accounts }: { post: PostRecord; accounts: AccountRecord[] }) {
  return (
    <div className="destination-stack">
      {accounts.map((account) => {
        const href = post.publishedLinks?.[account.id]
        return href ? (
          <a key={account.id} href={href} target="_blank" rel="noreferrer" className="destination-link" title={`Open published post on ${account.provider}`}>
            <DestinationAvatar account={account} compact />
          </a>
        ) : (
          <DestinationAvatar key={account.id} account={account} compact />
        )
      })}
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

export default App
