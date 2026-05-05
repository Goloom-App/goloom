import type { ProviderName } from './types'

export interface ApiClientOptions {
  baseUrl: string
  token: string
}

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export interface BackendUser {
  id: string
  email: string
  name: string
  subject: string
  is_admin: boolean
  created_at: string
}

export interface BackendTeam {
  id: string
  name: string
  description: string
  created_at: string
  is_personal: boolean
  personal_for_user_id?: string
}

export interface BackendMembership {
  user_id: string
  team_id: string
  role: 'owner' | 'editor' | 'viewer'
  created_at: string
}

export interface BackendAccount {
  id: string
  team_id: string
  provider: ProviderName
  auth_type: 'oauth_token' | 'app_password'
  provider_instance_id?: string
  instance_url: string
  username: string
  remote_account_id: string
  avatar_url?: string
  max_chars_override?: number
  access_token_expires_at?: string
  created_at: string
}

export interface BackendPost {
  id: string
  team_id: string
  author_user_id: string
  title: string
  content: string
  scheduled_at: string
  status: 'pending' | 'processing' | 'posted' | 'failed' | 'cancelled' | 'draft'
  attempt_count: number
  last_error?: string
  created_at: string
  updated_at: string
  target_accounts: string[]
  published_links?: Record<string, string>
  media_ids?: string[]
}

export interface BackendPostEngagementSummary {
  post_id: string
  title: string
  score: number
}

export interface BackendTeamAnalytics {
  metrics_total: Record<string, number>
  top_posts: BackendPostEngagementSummary[]
}

export interface BackendTeamMetricDelta {
  metric: string
  total: number
  delta_vs_prev_day: number
  delta_percent?: number
}

export interface BackendTeamAnalyticsReport {
  metrics: BackendTeamMetricDelta[]
  top_posts: BackendPostEngagementSummary[]
}

export interface BackendPostAnalyticsListRow {
  post_id: string
  title: string
  scheduled_at: string
  score: number
}

export interface BackendMetricHistoryPoint {
  date: string
  value: number
}

export interface BackendPostMetric {
  post_id: string
  account_id: string
  metric: string
  value: number
  updated_at: string
}

export interface BackendPostVersion {
  post_id: string
  account_id: string
  content: string
}

export interface BackendProviderInstance {
  id: string
  provider: ProviderName
  name: string
  instance_url: string
  client_id: string
  has_client_secret: boolean
  scopes: string[]
  authorization_endpoint?: string
  token_endpoint?: string
}

export interface BackendRuntimeConfig {
  general: {
    http_addr: string
    app_env?: string
    log_level?: string
    log_format?: string
  }
  security: {
    allowed_origins: string[]
    rate_limit_per_minute: number
    encryption_configured: boolean
  }
  scheduler: {
    poll_interval: string
    metrics_sync_interval?: string
    workers: number
  }
  oidc: {
    enabled: boolean
    issuer_url: string
    client_id: string
    has_secret: boolean
  }
}

export interface BackendAdminMetrics {
  users_count: number
  teams_count: number
  provider_instances_count: number
  posts_pending: number
  posts_draft?: number
  posts_processing: number
  posts_posted: number
  posts_failed: number
  posts_cancelled: number
}

export interface BackendAPIToken {
  id: string
  user_id: string
  name: string
  last_used_at?: string
  expires_at?: string
  created_at: string
}

export interface BackendAuthStatus {
  bootstrap_enabled: boolean
  bootstrap_recovery_enabled: boolean
  initial_setup_required: boolean
  oidc_enabled: boolean
  oidc_oauth_enabled: boolean
  has_users: boolean
  has_admin_users: boolean
  app_env?: string
}

export interface BackendOAuthAuthorization {
  authorization_url: string
}

function buildHeaders(token: string, withJSON = true) {
  const headers = new Headers()
  if (token.trim()) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  if (withJSON) {
    headers.set('Content-Type', 'application/json')
  }
  return headers
}

async function request<T>(options: ApiClientOptions, path: string, init?: RequestInit): Promise<T> {
  const baseUrl = options.baseUrl.trim().replace(/\/$/, '')
  const response = await fetch(baseUrl ? `${baseUrl}${path}` : path, init)
  if (!response.ok) {
    const message = await response.text()
    throw new ApiError(response.status, message || `Request failed with ${response.status}`)
  }
  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
}

export function requestAuthStatus(baseUrl: string) {
  return request<BackendAuthStatus>({ baseUrl, token: '' }, '/v1/auth/status', {
    headers: buildHeaders('', false),
  })
}

export function requestStartOIDCLogin(baseUrl: string, returnTo: string) {
  return request<BackendOAuthAuthorization>({ baseUrl, token: '' }, '/v1/auth/oidc/start', {
    method: 'POST',
    headers: buildHeaders('', true),
    body: JSON.stringify({ return_to: returnTo }),
  })
}

export function createApiClient(options: ApiClientOptions) {
  return {
    me() {
      return request<{ user: BackendUser; kind: string }>(options, '/v1/me', {
        headers: buildHeaders(options.token, false),
      })
    },
    listTeams() {
      return request<{ items: BackendTeam[] }>(options, '/v1/teams', {
        headers: buildHeaders(options.token, false),
      })
    },
    createTeam(payload: { name: string; description: string }) {
      return request<BackendTeam>(options, '/v1/teams', {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    listUsers() {
      return request<{ items: BackendUser[] }>(options, '/v1/users', {
        headers: buildHeaders(options.token, false),
      })
    },
    updateUser(userID: string, payload: { is_admin: boolean }) {
      return request<BackendUser>(options, `/v1/admin/users/${userID}`, {
        method: 'PATCH',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    runtimeConfig() {
      return request<BackendRuntimeConfig>(options, '/v1/admin/runtime-config', {
        headers: buildHeaders(options.token, false),
      })
    },
    adminMetrics() {
      return request<BackendAdminMetrics>(options, '/v1/admin/metrics', {
        headers: buildHeaders(options.token, false),
      })
    },
    listMyApiTokens() {
      return request<{ items: BackendAPIToken[] }>(options, '/v1/me/api-tokens', {
        headers: buildHeaders(options.token, false),
      })
    },
    createMyApiToken(payload: { name: string; expires_at?: string }) {
      return request<{ token: string; api_token: BackendAPIToken }>(options, '/v1/me/api-tokens', {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    revokeMyApiToken(tokenID: string) {
      return request<void>(options, `/v1/me/api-tokens/${tokenID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    listProviderInstances(provider?: ProviderName) {
      const search = provider ? `?provider=${provider}` : ''
      return request<{ items: BackendProviderInstance[] }>(options, `/v1/provider-instances${search}`, {
        headers: buildHeaders(options.token, false),
      })
    },
    createProviderInstance(payload: {
      provider: ProviderName
      name: string
      instance_url: string
      client_id?: string
      client_secret?: string
      scopes?: string[]
      authorization_endpoint?: string
      token_endpoint?: string
    }) {
      return request<BackendProviderInstance>(options, '/v1/admin/provider-instances', {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    updateProviderInstance(
      instanceID: string,
      payload: {
        provider: ProviderName
        name: string
        instance_url: string
        client_id?: string
        client_secret?: string
        scopes?: string[]
        authorization_endpoint?: string
        token_endpoint?: string
      },
    ) {
      return request<BackendProviderInstance>(options, `/v1/admin/provider-instances/${instanceID}`, {
        method: 'PUT',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    deleteProviderInstance(instanceID: string) {
      return request<void>(options, `/v1/admin/provider-instances/${instanceID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    listTeamMembers(teamID: string) {
      return request<{ items: BackendMembership[] }>(options, `/v1/teams/${teamID}/members`, {
        headers: buildHeaders(options.token, false),
      })
    },
    addTeamMember(teamID: string, payload: { user_id: string; role: 'owner' | 'editor' | 'viewer' }) {
      return request<BackendMembership>(options, `/v1/teams/${teamID}/members`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    removeTeamMember(teamID: string, userID: string) {
      return request<void>(options, `/v1/teams/${teamID}/members/${userID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    listAccounts(teamID: string) {
      return request<{ items: BackendAccount[] }>(options, `/v1/teams/${teamID}/accounts`, {
        headers: buildHeaders(options.token, false),
      })
    },
    createAccount(
      teamID: string,
      payload: {
        provider: ProviderName
        provider_instance_id?: string
        instance_url?: string
        username?: string
        identifier?: string
        access_token?: string
        refresh_token?: string
        app_password?: string
      },
    ) {
      return request<BackendAccount>(options, `/v1/teams/${teamID}/accounts`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    startMastodonOAuth(teamID: string, payload: { provider_instance_id: string; return_to: string }) {
      return request<BackendOAuthAuthorization>(options, `/v1/teams/${teamID}/accounts/oauth/mastodon/start`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    deleteAccount(teamID: string, accountID: string) {
      return request<void>(options, `/v1/teams/${teamID}/accounts/${accountID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    migrateAccount(teamID: string, accountID: string, payload: { target_team_id: string }) {
      return request<BackendAccount>(options, `/v1/teams/${teamID}/accounts/${accountID}/migrate`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    createTeamInvitation(teamID: string, payload: { email: string; role: 'editor' | 'viewer' }) {
      return request<{ invitation: { id: string; team_id: string; email: string; role: string; expires_at: string }; token: string }>(
        options,
        `/v1/teams/${teamID}/invitations`,
        {
          method: 'POST',
          headers: buildHeaders(options.token),
          body: JSON.stringify(payload),
        },
      )
    },
    acceptTeamInvitation(payload: { token: string }) {
      return request<BackendMembership>(options, `/v1/invitations/accept`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    listPosts(teamID: string) {
      return request<{ items: BackendPost[] }>(options, `/v1/teams/${teamID}/posts`, {
        headers: buildHeaders(options.token, false),
      })
    },
    createPost(
      teamID: string,
      payload: {
        title: string
        content: string
        scheduled_at: string
        target_accounts: string[]
        media_ids?: string[]
        draft?: boolean
      },
    ) {
      return request<BackendPost>(options, `/v1/teams/${teamID}/posts`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    updatePost(
      teamID: string,
      postID: string,
      payload: {
        title: string
        content: string
        scheduled_at: string
        target_accounts: string[]
        media_ids?: string[]
        draft?: boolean
      },
    ) {
      return request<BackendPost>(options, `/v1/teams/${teamID}/posts/${postID}`, {
        method: 'PATCH',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    uploadTeamMedia(teamID: string, accountID: string, file: File, altText?: string) {
      const form = new FormData()
      form.set('account_id', accountID)
      form.set('file', file)
      if (altText?.trim()) {
        form.set('alt_text', altText.trim())
      }
      const headers = new Headers()
      if (options.token.trim()) {
        headers.set('Authorization', `Bearer ${options.token}`)
      }
      return request<{ media_id: string }>(options, `/v1/teams/${teamID}/media/upload`, {
        method: 'POST',
        headers,
        body: form,
      })
    },
    deletePost(teamID: string, postID: string) {
      return request<void>(options, `/v1/teams/${teamID}/posts/${postID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    getTeamAnalytics(teamID: string, opts?: { top_posts?: number }) {
      const q =
        opts?.top_posts != null && opts.top_posts > 0
          ? `?top_posts=${encodeURIComponent(String(opts.top_posts))}`
          : ''
      return request<BackendTeamAnalytics>(options, `/v1/teams/${teamID}/analytics${q}`, {
        headers: buildHeaders(options.token, false),
      })
    },
    getTeamAnalyticsSummary(teamID: string, opts?: { top_posts?: number }) {
      const q =
        opts?.top_posts != null && opts.top_posts > 0
          ? `?top_posts=${encodeURIComponent(String(opts.top_posts))}`
          : ''
      return request<BackendTeamAnalyticsReport>(options, `/v1/teams/${teamID}/analytics/summary${q}`, {
        headers: buildHeaders(options.token, false),
      })
    },
    getTeamAnalyticsPosts(teamID: string, opts?: { sort?: string; limit?: number; offset?: number }) {
      const params = new URLSearchParams()
      if (opts?.sort) {
        params.set('sort', opts.sort)
      }
      if (opts?.limit != null && opts.limit > 0) {
        params.set('limit', String(opts.limit))
      }
      if (opts?.offset != null && opts.offset > 0) {
        params.set('offset', String(opts.offset))
      }
      const q = params.toString()
      return request<{ items: BackendPostAnalyticsListRow[] }>(
        options,
        `/v1/teams/${teamID}/analytics/posts${q ? `?${q}` : ''}`,
        {
          headers: buildHeaders(options.token, false),
        },
      )
    },
    getTeamAnalyticsChart(teamID: string, opts: { metric: string; days?: number }) {
      const params = new URLSearchParams()
      params.set('metric', opts.metric)
      if (opts.days != null && opts.days > 0) {
        params.set('days', String(opts.days))
      }
      return request<{ metric: string; days: number; series: BackendMetricHistoryPoint[] }>(
        options,
        `/v1/teams/${teamID}/analytics/chart?${params.toString()}`,
        {
          headers: buildHeaders(options.token, false),
        },
      )
    },
    getPostAnalytics(teamID: string, postID: string) {
      return request<{ items: BackendPostMetric[] }>(options, `/v1/teams/${teamID}/posts/${postID}/analytics`, {
        headers: buildHeaders(options.token, false),
      })
    },
    listPostVersions(teamID: string, postID: string) {
      return request<{ items: BackendPostVersion[] }>(options, `/v1/teams/${teamID}/posts/${postID}/versions`, {
        headers: buildHeaders(options.token, false),
      })
    },
    patchPostVersions(teamID: string, postID: string, payload: { versions: { account_id: string; content: string }[] }) {
      return request<{ items: BackendPostVersion[] }>(options, `/v1/teams/${teamID}/posts/${postID}/versions`, {
        method: 'PATCH',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
  }
}
