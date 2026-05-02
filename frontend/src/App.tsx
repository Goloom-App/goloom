import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { addMinutes, format, parseISO, set, startOfDay } from 'date-fns'

import { ApiError, createApiClient, requestAuthStatus, requestStartOIDCLogin } from './api'
import { initialSettings } from './data'
import { Icon } from './icons'
import type { IconName } from './icons'
import { toAccountRecord, toAuthStatusRecord, toPostRecord, toProviderInstanceRecord, toTeamMemberRecord, toTeamRecord, toUserRecord } from './mappers'
import { postsForTeam, sharedAccountLabels, SLOT_MINUTES } from './schedule'
import type { AccountRecord, AppSection, AuthStatusRecord, PostRecord, SettingsState, TeamRecord, UserRecord } from './types'

const SETTINGS_STORAGE_KEY = 'goloom-ui-settings'

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

      const [teamsResponse, providerInstancesResponse] = await Promise.all([
        api.listTeams(),
        api.listProviderInstances(),
      ])

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

  const navigationItems = useMemo(() => {
    const items: { id: AppSection; label: string; icon: IconName }[] = [
      { id: 'calendar', label: 'Schedule', icon: 'calendar' },
      { id: 'archive', label: 'Archive', icon: 'archive' },
      { id: 'teams', label: 'Teams', icon: 'teams' },
      { id: 'settings', label: 'Settings', icon: 'settings' },
    ]
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

      <div className="main-container">
        <main className="content-column">
          <header className="page-header">
            <div>
              <p className="eyebrow">Social publishing</p>
              <h1>{section.charAt(0).toUpperCase() + section.slice(1)}</h1>
            </div>

            <div className="inline-cluster">
              <select className="team-select" value={effectiveSelectedTeamId} onChange={(event) => setSelectedTeamId(event.target.value)} disabled={teams.length === 0}>
                {teams.length === 0 ? <option value="">No team loaded</option> : teams.map((team) => <option key={team.id} value={team.id}>{team.name}</option>)}
              </select>

              <button type="button" className="button button--primary" onClick={openCreateComposer} disabled={!selectedTeam || syncing}>
                <Icon name="plus" className="inline-icon" />
                <span>Create post</span>
              </button>
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
            <div className="teams-view">
              <div className="glass-panel">
                <div className="panel__header">
                  <h2>{selectedTeam?.name ?? 'No team selected'}</h2>
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
                      <small>{team.members.length} members · {team.accountIds.length} accounts</small>
                    </button>
                  ))}
                </div>
              </div>
            </div>
          )}

          {section === 'settings' && (
            <div className="settings-view">
               <div className="glass-panel">
                 <SettingsCard title="Connection">
                    <label className="field">
                      <span>API base URL (optional)</span>
                      <input value={settings.general.apiBaseUrl} onChange={(event) => updateAPIBaseURL(event.target.value)} />
                    </label>
                    <label className="field">
                      <span>Bearer token</span>
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
            </div>
          )}
        </main>

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
      </div>

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
