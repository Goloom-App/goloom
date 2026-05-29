import i18n from './i18n'
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

export interface BackendTeamSchedulingPreferences {
  timezone: string
  posting_windows: Array<{ weekday: number; start: string; end: string }>
  default_timeslots: string[]
}

export interface BackendTeam {
  id: string
  name: string
  description: string
  created_at: string
  is_personal: boolean
  is_ai_enabled?: boolean
  personal_for_user_id?: string
  scheduling_preferences?: BackendTeamSchedulingPreferences
}

export interface BackendPostTemplate {
  id: string
  team_id: string
  author_user_id: string
  title: string
  content: string
  recurrence_json: string
  visibility: string
  media_ids?: string[]
  media_exclude_by_account?: Record<string, string[]>
  target_account_ids: string[]
  enabled: boolean
  next_materialize_at?: string
  counter_next: number
  announces_template_id?: string
  announcement_days_before?: number
  created_at: string
  updated_at: string
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
  account_metrics_synced_at?: string
  post_engagement_synced_at?: string
  created_at: string
}

export interface BackendAdminSyncStatus {
  post_metrics_sync_interval: string
  account_metrics_sync_interval: string
  account_health_interval: string
  posted_targets_pending_sync: number
  posted_targets_never_synced: number
  posted_targets_with_metrics: number
  accounts_with_follower_metrics: number
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
  media_exclude_by_account?: Record<string, string[]>
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

export interface BackendAccountGrowthPoint {
  account_id: string
  date: string
  followers: number
  following: number
  posts: number
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

export interface BackendMediaItem {
  id: string
  team_id: string
  sha256: string
  filename: string
  mime_type: string
  size_bytes: number
  width?: number
  height?: number
  created_at: string
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
    rate_limit_authenticated_per_minute: number
    encryption_configured: boolean
  }
  scheduler: {
    poll_interval: string
    metrics_sync_interval?: string
    account_health_interval?: string
    workers: number
  }
  oidc: {
    enabled: boolean
    issuer_url: string
    client_id: string
    has_secret: boolean
  }
}

export interface BackendLogEntry {
  id: string
  level: string
  message: string
  attributes: Record<string, string>
  source_file?: string
  source_line?: number
  created_at: string
  archived_at?: string
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
  scopes?: string[]
  team_id?: string
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

export interface BackendStyleMetadata {
  tonality: string
  formatting_rules: string[]
  banned_words: string[]
  max_hashtags: number
  preferred_language: string
}

export interface BackendTeamProfile {
  id: string
  team_id: string
  style_metadata: BackendStyleMetadata
  auto_publish_enabled: boolean
  created_at: string
  updated_at: string
}

export interface BackendCampaignFormat {
  id: string
  team_id: string
  name: string
  weekday: number | null
  structure: Record<string, unknown>
  required_hashtags: string[]
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface BackendStyleExample {
  id: string
  team_id: string
  platform: string
  content: string
  notes: string
  created_at: string
}

export interface BackendAIJob {
  id: string
  team_id: string
  author_user_id: string
  type: 'voice_engine' | 'campaign_autopilot' | 'proactive_trigger'
  status: 'pending' | 'processing' | 'completed' | 'failed'
  payload: Record<string, unknown>
  result: Record<string, unknown> | null
  error_message: string | null
  created_at: string
  updated_at: string
  completed_at: string | null
}

export interface BackendAITriggerResponse {
  jobId: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
}

export interface BackendAIServiceConfig {
  id: string
  team_id: string | null
  service_url: string
  description: string
  created_at: string
}

export interface BackendRSSFeedConfig {
  id: string
  team_id: string
  feed_url: string
  name: string
  is_active: boolean
  last_fetched_at: string | null
  created_at: string
}

export interface BackendProactiveTriggerSettings {
  id: string
  team_id: string
  content_gap_threshold_days: number
  auto_fill_enabled: boolean
  max_triggers_per_day: number
  cron_schedule: string
  created_at: string
  updated_at: string
}

function currentAcceptLanguage(): string {
  try {
    const raw = localStorage.getItem('goloom-ui-settings')
    if (raw) {
      const parsed = JSON.parse(raw) as { ui?: { language?: string } }
      if (parsed.ui?.language?.trim()) {
        return parsed.ui.language.trim()
      }
    }
  } catch {
    /* ignore */
  }
  return typeof navigator !== 'undefined' ? navigator.language || 'en' : 'en'
}

function buildHeaders(token: string, withJSON = true) {
  const headers = new Headers()
  if (token.trim()) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  headers.set('Accept-Language', currentAcceptLanguage())
  if (withJSON) {
    headers.set('Content-Type', 'application/json')
  }
  return headers
}

async function request<T>(options: ApiClientOptions, path: string, init?: RequestInit): Promise<T> {
  const baseUrl = options.baseUrl.trim().replace(/\/$/, '')
  const response = await fetch(baseUrl ? `${baseUrl}${path}` : path, init)
  if (!response.ok) {
    let message = await response.text()
    if (response.status === 429) {
      const trimmed = message.trim().toLowerCase()
      if (!trimmed || trimmed.includes('rate limit')) {
        message = i18n.t('common.rateLimit')
      }
    }
    throw new ApiError(
      response.status,
      message || i18n.t('common.requestFailed', { status: response.status }),
    )
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
    authorizationHeader() {
      const t = options.token.trim()
      return t ? `Bearer ${t}` : ''
    },
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
    updateTeam(
      teamID: string,
      payload: {
        name: string
        description: string
        scheduling_preferences?: BackendTeamSchedulingPreferences
        is_ai_enabled?: boolean
      },
    ) {
      return request<BackendTeam>(options, `/v1/teams/${teamID}`, {
        method: 'PATCH',
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
    adminSyncStatus() {
      return request<BackendAdminSyncStatus>(options, '/v1/admin/sync-status', {
        headers: buildHeaders(options.token, false),
      })
    },
    adminSyncMetrics() {
      return request<{ status: string; message: string }>(options, '/v1/admin/sync-metrics', {
        method: 'POST',
        headers: buildHeaders(options.token, false),
      })
    },
    listAIEnabledTeams() {
      return request<{ items: BackendTeam[] }>(options, '/v1/admin/ai-enabled-teams', {
        headers: buildHeaders(options.token, false),
      })
    },
    listMyApiTokens() {
      return request<{ items: BackendAPIToken[] }>(options, '/v1/me/api-tokens', {
        headers: buildHeaders(options.token, false),
      })
    },
    createMyApiToken(payload: { name: string; expires_at?: string; scopes?: string[]; team_id?: string }) {
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
    updateAccount(
      teamID: string,
      accountID: string,
      payload: {
        name?: string
        max_chars_override?: number
        access_token?: string
        refresh_token?: string
      },
    ) {
      return request<BackendAccount>(options, `/v1/teams/${teamID}/accounts/${accountID}`, {
        method: 'PATCH',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
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
        media_exclude_by_account?: Record<string, string[]>
        account_content_override?: Record<string, string>
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
        title?: string
        content?: string
        scheduled_at?: string
        target_accounts?: string[]
        media_ids?: string[]
        media_exclude_by_account?: Record<string, string[]>
        account_content_override?: Record<string, string>
        draft?: boolean
      },
    ) {
      return request<BackendPost>(options, `/v1/teams/${teamID}/posts/${postID}`, {
        method: 'PATCH',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    deletePost(teamID: string, postID: string) {
      return request<void>(options, `/v1/teams/${teamID}/posts/${postID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    listTeamMedia(teamID: string) {
      return request<{ items: BackendMediaItem[] }>(options, `/v1/teams/${teamID}/media`, {
        headers: buildHeaders(options.token, false),
      })
    },
    uploadTeamMediaToLibrary(teamID: string, file: File) {
      const form = new FormData()
      form.set('file', file)
      const headers = new Headers()
      if (options.token.trim()) {
        headers.set('Authorization', `Bearer ${options.token}`)
      }
      return request<BackendMediaItem>(options, `/v1/teams/${teamID}/media`, {
        method: 'POST',
        headers,
        body: form,
      })
    },
    deleteTeamMedia(teamID: string, mediaID: string) {
      return request<void>(options, `/v1/teams/${teamID}/media/${mediaID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    mediaPreviewUrl(teamID: string, mediaID: string): string {
      const baseUrl = options.baseUrl.trim().replace(/\/$/, '')
      const path = `/v1/teams/${teamID}/media/${mediaID}/preview`
      const url = baseUrl ? `${baseUrl}${path}` : path
      // Note: This URL requires Authorization header which standard <img> doesn't send.
      // We'll need a way to pass the token, e.g. as a query param or use a custom hook to fetch as blob.
      // For simplicity in this PWA evolution, we'll allow token in query param for previews
      // OR better: use a blob URL via fetch in the component.
      return url
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
    getTeamAccountGrowth(teamID: string, accountID: string, opts?: { days?: number }) {
      const params = new URLSearchParams()
      if (opts?.days != null && opts.days > 0) {
        params.set('days', String(opts.days))
      }
      const encodedAccount = encodeURIComponent(accountID)
      const suffix = params.toString()
      return request<{ days: number; account: string; series: BackendAccountGrowthPoint[] }>(
        options,
        `/v1/teams/${teamID}/analytics/account/${encodedAccount}/growth${suffix ? `?${suffix}` : ''}`,
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
    getTeamEngagementHours(teamID: string, opts?: { days?: number }) {
      const q = opts?.days != null && opts.days > 0 ? `?days=${encodeURIComponent(String(opts.days))}` : ''
      return request<{ hours: { hour: number; score: number }[] }>(
        options,
        `/v1/teams/${teamID}/analytics/engagement-hours${q}`,
        {
          headers: buildHeaders(options.token, false),
        },
      )
    },
    listPostTemplates(teamID: string) {
      return request<{ items: BackendPostTemplate[] }>(options, `/v1/teams/${teamID}/post-templates`, {
        headers: buildHeaders(options.token, false),
      })
    },
    listAllPostVersions(teamID: string) {
      return request<{ items: BackendPostVersion[] }>(options, `/v1/teams/${teamID}/versions`, {
        headers: buildHeaders(options.token, false),
      })
    },
    createPostTemplate(
      teamID: string,
      payload: {
        title: string
        content: string
        recurrence_json: string
        visibility?: string
        media_ids?: string[]
        media_exclude_by_account?: Record<string, string[]>
        target_account_ids: string[]
        enabled?: boolean
        announces_template_id?: string
        announcement_days_before?: number
      },
    ) {
      return request<BackendPostTemplate>(options, `/v1/teams/${teamID}/post-templates`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    updatePostTemplate(
      teamID: string,
      templateID: string,
      payload: Record<string, unknown>,
    ) {
      return request<BackendPostTemplate>(options, `/v1/teams/${teamID}/post-templates/${templateID}`, {
        method: 'PATCH',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    deletePostTemplate(teamID: string, templateID: string) {
      return request<void>(options, `/v1/teams/${teamID}/post-templates/${templateID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    skipPostTemplateOccurrence(teamID: string, templateID: string, occurrenceAtIso: string, shiftToIso?: string) {
      const body: Record<string, string> = { occurrence_at: occurrenceAtIso }
      if (shiftToIso) body.shift_to = shiftToIso
      return request<void>(options, `/v1/teams/${teamID}/post-templates/${templateID}/skip`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(body),
      })
    },
    listLogEntries(params?: { level?: string; search?: string; archived?: boolean; before?: string; after?: string; limit?: number; offset?: number }) {
      const search = new URLSearchParams()
      if (params?.level) search.set('level', params.level)
      if (params?.search) search.set('search', params.search)
      if (params?.archived != null) search.set('archived', String(params.archived))
      if (params?.before) search.set('before', params.before)
      if (params?.after) search.set('after', params.after)
      if (params?.limit != null) search.set('limit', String(params.limit))
      if (params?.offset != null) search.set('offset', String(params.offset))
      const q = search.toString()
      return request<{ entries: BackendLogEntry[]; total: number }>(options, `/v1/admin/logs${q ? `?${q}` : ''}`, {
        headers: buildHeaders(options.token, false),
      })
    },
    archiveLogEntry(id: string) {
      return request<void>(options, `/v1/admin/logs/${id}/archive`, {
        method: 'POST',
        headers: buildHeaders(options.token, false),
      })
    },
    unarchiveLogEntry(id: string) {
      return request<void>(options, `/v1/admin/logs/${id}/unarchive`, {
        method: 'POST',
        headers: buildHeaders(options.token, false),
      })
    },
    deleteLogEntry(id: string) {
      return request<void>(options, `/v1/admin/logs/${id}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    pruneLogEntries(before?: string) {
      const q = before ? `?before=${encodeURIComponent(before)}` : ''
      return request<{ deleted_count: number }>(options, `/v1/admin/logs/prune${q}`, {
        method: 'POST',
        headers: buildHeaders(options.token, false),
      })
    },
    validatePost(
      teamID: string,
      payload: {
        title: string
        content: string
        scheduled_at: string
        target_accounts: string[]
        media_ids?: string[]
        media_exclude_by_account?: Record<string, string[]>
        account_content_override?: Record<string, string>
        draft?: boolean
      },
    ) {
      return request<{
        max_chars: number
        content_length: number
        valid: boolean
        destinations: Array<{
          account_id: string
          provider: string
          max_chars: number
          length: number
          valid: boolean
        }>
      }>(options, `/v1/teams/${teamID}/posts/validate`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    getTeamProfile(teamID: string) {
      return request<BackendTeamProfile>(options, `/v1/teams/${teamID}/profile`, {
        headers: buildHeaders(options.token, false),
      })
    },
    upsertTeamProfile(
      teamID: string,
      payload: {
        style_metadata: BackendStyleMetadata
        auto_publish_enabled: boolean
      },
    ) {
      return request<BackendTeamProfile>(options, `/v1/teams/${teamID}/profile`, {
        method: 'PUT',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    deleteTeamProfile(teamID: string) {
      return request<void>(options, `/v1/teams/${teamID}/profile`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    listCampaignFormats(teamID: string) {
      return request<{ items: BackendCampaignFormat[] }>(options, `/v1/teams/${teamID}/campaign-formats`, {
        headers: buildHeaders(options.token, false),
      })
    },
    createCampaignFormat(
      teamID: string,
      payload: {
        name: string
        weekday: number | null
        structure: Record<string, unknown>
        required_hashtags: string[]
        is_active: boolean
      },
    ) {
      return request<BackendCampaignFormat>(options, `/v1/teams/${teamID}/campaign-formats`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    updateCampaignFormat(
      teamID: string,
      formatID: string,
      payload: {
        name?: string
        weekday?: number | null
        structure?: Record<string, unknown>
        required_hashtags?: string[]
        is_active?: boolean
      },
    ) {
      return request<BackendCampaignFormat>(options, `/v1/teams/${teamID}/campaign-formats/${formatID}`, {
        method: 'PATCH',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    deleteCampaignFormat(teamID: string, formatID: string) {
      return request<void>(options, `/v1/teams/${teamID}/campaign-formats/${formatID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    listStyleExamples(teamID: string) {
      return request<{ items: BackendStyleExample[] }>(options, `/v1/teams/${teamID}/style-examples`, {
        headers: buildHeaders(options.token, false),
      })
    },
    createStyleExample(
      teamID: string,
      payload: {
        platform: string
        content: string
        notes: string
      },
    ) {
      return request<BackendStyleExample>(options, `/v1/teams/${teamID}/style-examples`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    deleteStyleExample(teamID: string, exampleID: string) {
      return request<void>(options, `/v1/teams/${teamID}/style-examples/${exampleID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    triggerAIJob(teamID: string, type: 'voice_engine' | 'campaign_autopilot' | 'proactive_trigger', params: Record<string, unknown>) {
      return request<BackendAITriggerResponse>(options, `/v1/teams/${teamID}/ai-trigger`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify({ type, params }),
      })
    },
    getAIJob(teamID: string, jobID: string) {
      return request<BackendAIJob>(options, `/v1/teams/${teamID}/ai-jobs/${jobID}`, {
        headers: buildHeaders(options.token, false),
      })
    },
    listAIJobs(teamID: string) {
      return request<{ items: BackendAIJob[] }>(options, `/v1/teams/${teamID}/ai-jobs`, {
        headers: buildHeaders(options.token, false),
      })
    },
    getAIContext(teamID: string) {
      return request<{
        team: BackendTeam
        profile: BackendTeamProfile | null
        campaign_formats: BackendCampaignFormat[]
        style_examples: BackendStyleExample[]
        recent_posts: BackendPost[]
      }>(options, `/v1/teams/${teamID}/ai-context`, {
        headers: buildHeaders(options.token, false),
      })
    },
    listRSSFeeds(teamID: string) {
      return request<{ items: BackendRSSFeedConfig[] }>(options, `/v1/teams/${teamID}/rss-feeds`, {
        headers: buildHeaders(options.token, false),
      })
    },
    createRSSFeed(
      teamID: string,
      payload: {
        feed_url: string
        name: string
        is_active: boolean
      },
    ) {
      return request<BackendRSSFeedConfig>(options, `/v1/teams/${teamID}/rss-feeds`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    updateRSSFeed(
      teamID: string,
      feedID: string,
      payload: {
        feed_url?: string
        name?: string
        is_active?: boolean
      },
    ) {
      return request<BackendRSSFeedConfig>(options, `/v1/teams/${teamID}/rss-feeds/${feedID}`, {
        method: 'PATCH',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    deleteRSSFeed(teamID: string, feedID: string) {
      return request<void>(options, `/v1/teams/${teamID}/rss-feeds/${feedID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    getAIServiceConfig(teamID: string) {
      return request<BackendAIServiceConfig>(options, `/v1/teams/${teamID}/ai-service-config`, {
        headers: buildHeaders(options.token, false),
      })
    },
    upsertAIServiceConfig(
      teamID: string,
      payload: {
        service_url: string
        description: string
      },
    ) {
      return request<BackendAIServiceConfig>(options, `/v1/teams/${teamID}/ai-service-config`, {
        method: 'PUT',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    getProactiveSettings(teamID: string) {
      return request<BackendProactiveTriggerSettings>(options, `/v1/teams/${teamID}/proactive-settings`, {
        headers: buildHeaders(options.token, false),
      })
    },
    upsertProactiveSettings(
      teamID: string,
      payload: {
        content_gap_threshold_days: number
        auto_fill_enabled: boolean
        max_triggers_per_day: number
        cron_schedule: string
      },
    ) {
      return request<BackendProactiveTriggerSettings>(options, `/v1/teams/${teamID}/proactive-settings`, {
        method: 'PUT',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    createAIDraft(teamID: string, payload: Record<string, unknown>) {
      return request<BackendPost>(options, `/v1/teams/${teamID}/posts/draft`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
  }
}
