import i18n from './i18n'
import type { AITriggerResponse, ProviderName } from './types'

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
  tour_done: boolean
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
  is_ai_enabled?: boolean
  scheduling_preferences?: BackendTeamSchedulingPreferences
  brand_color?: string
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
  ai_enhance_enabled?: boolean
  ai_enhance_announcement?: boolean
  output_mode?: 'draft' | 'scheduled' | 'publish_now'
  prompt_hint?: string
  title_hint?: string
  tonality?: string
  materialize_horizon_days?: number
  next_materialize_at?: string
  counter_next: number
  announcement_enabled?: boolean
  announcement_title?: string
  announcement_content?: string
  announcement_days_before?: number
  announcement_counter_next?: number
  announcement_target_account_ids?: string[]
  created_at: string
  updated_at: string
}

export interface BackendPostTemplateLinkedPost {
  id: string
  status: BackendPost['status']
  template_occurrence_at: string
  template_post_role: string
  template_counter?: number
}

export interface BackendMembership {
  user_id: string
  team_id: string
  role: 'owner' | 'editor' | 'viewer'
  created_at: string
}

export interface BackendTeamInvitation {
  id: string
  team_id: string
  email: string
  role: 'editor' | 'viewer'
  expires_at: string
  created_by_user_id: string
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
  source?: 'scheduled' | 'imported' | 'automation'
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

export interface BackendHashtagPerformance {
  tag: string
  display: string
  uses: number
  total_engagement: number
  avg_engagement: number
  score: number
}

export interface BackendHashtagInsights {
  posts_total: number
  posts_with_tags: number
  distinct_tags: number
  total_tag_uses: number
  avg_tags_per_post: number
  avg_engagement_with_tags: number
  avg_engagement_without_tags: number
}

export interface BackendEngagementHeatmapBucket {
  weekday: number
  hour: number
  score: number
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
  component?: string
  created_at: string
  archived_at?: string
}

export interface BackendAuditEvent {
  id: string
  team_id: string
  actor_user_id: string
  actor_name: string
  actor_email: string
  actor_kind: string
  token_id?: string
  token_name?: string
  action: string
  target_type: string
  target_id?: string
  summary?: string
  metadata?: Record<string, string>
  created_at: string
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

export interface BackendPublishFailureTarget {
  account_id: string
  account_name: string
  provider: string
  status: string
  last_error?: string
  published_url?: string
}

export interface BackendPublishFailure {
  post_id: string
  team_id: string
  team_name: string
  title: string
  scheduled_at: string
  attempt_count: number
  last_error?: string
  updated_at: string
  targets: BackendPublishFailureTarget[]
}

export interface BackendAPIToken {
  id: string
  user_id: string
  name: string
  description?: string
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

export interface BackendVersionInfo {
  current: string
  latest: string
  update_available: boolean
}

export interface BackendOAuthAuthorization {
  authorization_url: string
}

export interface BackendBrandIdentity {
  archetype?: string
  persona?: string
  industry: string
  main_value: string
  target_audience: string
}

export interface BackendBrandLanguageDNA {
  sentence_style: string
  preferred_words: string[]
  signature_phrases?: string[]
  humor_style: string
  anti_ai_override?: boolean
}

export interface BackendBrandReachStrategy {
  hook_style: string
  cta_focus: string
}

export interface BackendStyleMetadata {
  tonality?: string
  formatting_rules: string[]
  banned_words: string[]
  max_hashtags: number
  preferred_language: string
  identity?: BackendBrandIdentity
  language_dna?: BackendBrandLanguageDNA
  reach_strategy?: BackendBrandReachStrategy
}

export interface BackendKnowledgeSource {
  id: string
  team_id: string
  type: 'text' | 'url' | 'file'
  name: string
  content: string
  source_url?: string
  media_id?: string
  created_at: string
  updated_at: string
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
  type: 'voice_engine' | 'campaign_autopilot' | 'proactive_trigger' | 'profile_analysis' | 'vibe_preview' | 'profile_assistant'
  status: 'pending' | 'processing' | 'completed' | 'failed'
  payload: Record<string, unknown>
  result: Record<string, unknown> | null
  error_message: string | null
  created_at: string
  updated_at: string
  completed_at: string | null
}

export interface BackendAITriggerResponse {
  job_id: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
}

export interface BackendAIServiceConfig {
  id: string
  team_id: string | null
  provider: string
  model: string
  base_url: string
  api_key_set: boolean
  description: string
  created_at: string
}

export interface BackendAIChatMention {
  type: 'campaign' | 'recurring' | 'rss' | 'account'
  id: string
  name: string
}

export interface BackendAIChatMessage {
  role: 'user' | 'assistant'
  content: string
  mentions?: BackendAIChatMention[]
}

export interface BackendAIChatEvent {
  type: 'status' | 'message' | 'tool_call' | 'tool_result' | 'error' | 'done'
  message?: string
  tool_name?: string
  tool_args?: unknown
  payload?: unknown
}

export interface BackendReviewQueueItem extends BackendPost {
  is_overdue: boolean
  rss_feed_name?: string
}

export interface BackendRSSFeedConfig {
  id: string
  team_id: string
  feed_url: string
  name: string
  is_active: boolean
  ai_enhance_enabled?: boolean
  content_template?: string
  title_template?: string
  title_hint?: string
  output_mode?: 'draft' | 'scheduled' | 'publish_now'
  max_posts_per_day?: number
  counter_next?: number
  prompt_hint: string
  target_account_ids: string[]
  tonality: string
  initial_sync_mode?: 'baseline' | 'publish_latest'
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

export interface BackendExternalPostMonitorSettings {
  id?: string
  team_id: string
  enabled: boolean
  backfill_completed_at?: string | null
  last_sync_at?: string | null
  created_at?: string
  updated_at?: string
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

function readCookie(name: string): string {
  if (typeof document === 'undefined') {
    return ''
  }
  const match = document.cookie.split('; ').find((row) => row.startsWith(`${name}=`))
  return match ? decodeURIComponent(match.slice(name.length + 1)) : ''
}

async function request<T>(options: ApiClientOptions, path: string, init?: RequestInit): Promise<T> {
  const baseUrl = options.baseUrl.trim().replace(/\/$/, '')
  const method = (init?.method ?? 'GET').toUpperCase()
  const headers = new Headers(init?.headers)
  // CSRF double-submit token for cookie-authenticated, state-changing requests.
  // Bearer (API token) requests don't carry the cookie, so this is harmless there.
  if (method !== 'GET' && method !== 'HEAD') {
    const csrf = readCookie('goloom_csrf')
    if (csrf) {
      headers.set('X-CSRF-Token', csrf)
    }
  }
  // Send the session cookie on same-origin requests (the production norm).
  const response = await fetch(baseUrl ? `${baseUrl}${path}` : path, { ...init, headers, credentials: 'same-origin' })
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

export function requestVersionInfo(baseUrl: string) {
  return request<BackendVersionInfo>({ baseUrl, token: '' }, '/v1/version', {
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
      return request<{ user: BackendUser; kind: string; token_id?: string }>(options, '/v1/me', {
        headers: buildHeaders(options.token, false),
      })
    },
    // Persist the guided-tour flag on the account so it follows the user across browsers.
    setTourDone(done: boolean) {
      return request<BackendUser>(options, '/v1/me/tour', {
        method: 'PUT',
        headers: buildHeaders(options.token, true),
        body: JSON.stringify({ done }),
      })
    },
    // Exchange a bearer token (bootstrap/recovery or API token) for a cookie session.
    sessionFromToken(token: string) {
      return request<{ user: BackendUser }>(options, '/v1/auth/session/token', {
        method: 'POST',
        headers: buildHeaders('', true),
        body: JSON.stringify({ token }),
      })
    },
    logout() {
      return request<void>(options, '/v1/auth/logout', {
        method: 'POST',
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
        brand_color?: string
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
    adminPublishFailures() {
      return request<{ items: BackendPublishFailure[] }>(options, '/v1/admin/publish-failures', {
        headers: buildHeaders(options.token, false),
      })
    },
    adminAcknowledgePublishFailure(postID: string) {
      return request<{ status: string }>(options, `/v1/admin/publish-failures/${postID}/acknowledge`, {
        method: 'POST',
        headers: buildHeaders(options.token, false),
      })
    },
    adminRetryPublishFailure(postID: string) {
      return request<{ status: string }>(options, `/v1/admin/publish-failures/${postID}/retry`, {
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
    createMyApiToken(payload: { name: string; description?: string; expires_at?: string; scopes?: string[]; team_id?: string }) {
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
    providerInstanceHealth(instanceID: string) {
      return request<{ healthy: boolean; status: string; detail?: string }>(
        options,
        `/v1/admin/provider-instances/${instanceID}/health`,
        { headers: buildHeaders(options.token, false) },
      )
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
      return request<{ invitation: BackendTeamInvitation; token: string }>(
        options,
        `/v1/teams/${teamID}/invitations`,
        {
          method: 'POST',
          headers: buildHeaders(options.token),
          body: JSON.stringify(payload),
        },
      )
    },
    listTeamInvitations(teamID: string) {
      return request<{ items: BackendTeamInvitation[] }>(options, `/v1/teams/${teamID}/invitations`, {
        headers: buildHeaders(options.token, false),
      })
    },
    deleteTeamInvitation(teamID: string, invitationID: string) {
      return request<void>(options, `/v1/teams/${teamID}/invitations/${invitationID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token),
      })
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
    getPost(teamID: string, postID: string) {
      return request<BackendPost>(options, `/v1/teams/${teamID}/posts/${postID}`, {
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
    renameTeamMedia(teamID: string, mediaID: string, filename: string) {
      return request<BackendMediaItem>(options, `/v1/teams/${teamID}/media/${mediaID}`, {
        method: 'PATCH',
        headers: buildHeaders(options.token),
        body: JSON.stringify({ filename }),
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
      return request<{ items: BackendPostMetric[]; published_links?: Record<string, string> }>(options, `/v1/teams/${teamID}/posts/${postID}/analytics`, {
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
    getTeamEngagementHeatmap(teamID: string, opts?: { days?: number; account?: string }) {
      const params = new URLSearchParams()
      if (opts?.days != null && opts.days > 0) {
        params.set('days', String(opts.days))
      }
      if (opts?.account && opts.account !== 'all') {
        params.set('account', opts.account)
      }
      const suffix = params.toString()
      return request<{ days: number; account: string; buckets: BackendEngagementHeatmapBucket[] }>(
        options,
        `/v1/teams/${teamID}/analytics/engagement-heatmap${suffix ? `?${suffix}` : ''}`,
        {
          headers: buildHeaders(options.token, false),
        },
      )
    },
    getTeamHashtagPerformance(teamID: string, opts?: { days?: number; provider?: string; limit?: number }) {
      const params = new URLSearchParams()
      if (opts?.days != null && opts.days > 0) {
        params.set('days', String(opts.days))
      }
      if (opts?.provider) {
        params.set('provider', opts.provider)
      }
      if (opts?.limit != null && opts.limit > 0) {
        params.set('limit', String(opts.limit))
      }
      const suffix = params.toString()
      return request<{ days: number; provider: string; items: BackendHashtagPerformance[]; insights?: BackendHashtagInsights }>(
        options,
        `/v1/teams/${teamID}/analytics/hashtags${suffix ? `?${suffix}` : ''}`,
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
        ai_enhance_enabled?: boolean
        ai_enhance_announcement?: boolean
        output_mode?: 'draft' | 'scheduled' | 'publish_now'
        prompt_hint?: string
        title_hint?: string
        tonality?: string
        announcement_enabled?: boolean
        announcement_title?: string
        announcement_content?: string
        announcement_days_before?: number
        announcement_counter_next?: number
        announcement_target_account_ids?: string[]
        materialize_horizon_days?: number
        counter_next?: number
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
    listPostTemplateLinkedPosts(teamID: string, templateID: string) {
      return request<{ items: BackendPostTemplateLinkedPost[] }>(
        options,
        `/v1/teams/${teamID}/post-templates/${templateID}/linked-posts`,
        { headers: buildHeaders(options.token, false) },
      )
    },
    regeneratePostTemplate(teamID: string, templateID: string, body: { mode: 'occurrence' | 'horizon'; occurrence_at?: string }) {
      return request<{ deleted_posts: number; regenerated_occurrences: number }>(
        options,
        `/v1/teams/${teamID}/post-templates/${templateID}/regenerate`,
        {
          method: 'POST',
          headers: buildHeaders(options.token),
          body: JSON.stringify(body),
        },
      )
    },
    listLogEntries(params?: { level?: string; search?: string; component?: string; archived?: boolean; before?: string; after?: string; limit?: number; offset?: number }) {
      const search = new URLSearchParams()
      if (params?.level) search.set('level', params.level)
      if (params?.search) search.set('search', params.search)
      if (params?.component) search.set('component', params.component)
      if (params?.archived != null) search.set('archived', String(params.archived))
      if (params?.before) search.set('before', params.before)
      if (params?.after) search.set('after', params.after)
      if (params?.limit != null) search.set('limit', String(params.limit))
      if (params?.offset != null) search.set('offset', String(params.offset))
      const q = search.toString()
      return request<{ entries: BackendLogEntry[]; total: number; components?: string[] }>(options, `/v1/admin/logs${q ? `?${q}` : ''}`, {
        headers: buildHeaders(options.token, false),
      })
    },
    listTeamAuditLog(teamId: string, params?: { actor?: string; action?: string; limit?: number; offset?: number }) {
      const search = new URLSearchParams()
      if (params?.actor) search.set('actor', params.actor)
      if (params?.action) search.set('action', params.action)
      if (params?.limit != null) search.set('limit', String(params.limit))
      if (params?.offset != null) search.set('offset', String(params.offset))
      const q = search.toString()
      return request<{ entries: BackendAuditEvent[]; total: number }>(options, `/v1/teams/${teamId}/audit-log${q ? `?${q}` : ''}`, {
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
    listKnowledgeSources(teamID: string) {
      return request<{ items: BackendKnowledgeSource[] }>(options, `/v1/teams/${teamID}/knowledge-sources`, {
        headers: buildHeaders(options.token, false),
      })
    },
    createKnowledgeSource(
      teamID: string,
      payload: {
        type: 'text' | 'url' | 'file'
        name: string
        content?: string
        source_url?: string
        media_id?: string
      },
    ) {
      return request<BackendKnowledgeSource>(options, `/v1/teams/${teamID}/knowledge-sources`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    updateKnowledgeSource(
      teamID: string,
      sourceID: string,
      payload: {
        type: 'text' | 'url' | 'file'
        name: string
        content?: string
        source_url?: string
        media_id?: string
      },
    ) {
      return request<BackendKnowledgeSource>(options, `/v1/teams/${teamID}/knowledge-sources/${sourceID}`, {
        method: 'PATCH',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    deleteKnowledgeSource(teamID: string, sourceID: string) {
      return request<void>(options, `/v1/teams/${teamID}/knowledge-sources/${sourceID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    previewAIPrompt(teamID: string, params: Record<string, unknown>) {
      return request<{ system_prompt: string; generation_prompt: string }>(
        options,
        `/v1/teams/${teamID}/ai/prompt-preview`,
        {
          method: 'POST',
          headers: buildHeaders(options.token),
          body: JSON.stringify({ params }),
        },
      )
    },
    triggerAIJob(
      teamID: string,
      type: 'voice_engine' | 'campaign_autopilot' | 'proactive_trigger' | 'profile_analysis' | 'vibe_preview' | 'profile_assistant',
      params: Record<string, unknown>,
    ) {
      return request<BackendAITriggerResponse>(options, `/v1/teams/${teamID}/ai-trigger`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify({ type, params }),
      }).then((raw) => ({
        jobId: raw.job_id,
        status: raw.status,
      }) satisfies AITriggerResponse)
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
    cancelAIJob(teamID: string, jobID: string) {
      return request<BackendAIJob>(options, `/v1/teams/${teamID}/ai-jobs/${jobID}/cancel`, {
        method: 'POST',
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
    listReviewQueue(teamID: string) {
      return request<{ items: BackendReviewQueueItem[] }>(options, `/v1/teams/${teamID}/review-queue`, {
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
        ai_enhance_enabled?: boolean
        content_template?: string
        title_template?: string
        title_hint?: string
        output_mode?: 'draft' | 'scheduled' | 'publish_now'
        max_posts_per_day?: number
        prompt_hint?: string
        target_account_ids?: string[]
        tonality?: string
        initial_sync_mode?: 'baseline' | 'publish_latest'
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
        ai_enhance_enabled?: boolean
        content_template?: string
        title_template?: string
        title_hint?: string
        output_mode?: 'draft' | 'scheduled' | 'publish_now'
        max_posts_per_day?: number
        prompt_hint?: string
        target_account_ids?: string[]
        tonality?: string
        initial_sync_mode?: 'baseline' | 'publish_latest'
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
        provider: string
        model: string
        base_url: string
        // Write-only: empty keeps the stored key.
        api_key: string
        description: string
      },
    ) {
      return request<BackendAIServiceConfig>(options, `/v1/teams/${teamID}/ai-service-config`, {
        method: 'PUT',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    async streamAIChat(
      teamID: string,
      messages: BackendAIChatMessage[],
      onEvent: (event: BackendAIChatEvent) => void,
      signal?: AbortSignal,
      viewContext?: unknown,
    ) {
      const baseUrl = options.baseUrl.trim().replace(/\/$/, '')
      const response = await fetch(`${baseUrl}/v1/teams/${teamID}/ai/chat`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify(viewContext ? { messages, view_context: viewContext } : { messages }),
        signal,
      })
      if (!response.ok || !response.body) {
        const message = await response.text().catch(() => '')
        throw new ApiError(response.status, message || i18n.t('common.requestFailed', { status: response.status }))
      }
      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''
      for (;;) {
        const { value, done } = await reader.read()
        if (done) {
          break
        }
        buffer += decoder.decode(value, { stream: true })
        let separator = buffer.indexOf('\n\n')
        while (separator !== -1) {
          const chunk = buffer.slice(0, separator)
          buffer = buffer.slice(separator + 2)
          for (const line of chunk.split('\n')) {
            if (!line.startsWith('data:')) {
              continue
            }
            try {
              onEvent(JSON.parse(line.slice(5).trim()) as BackendAIChatEvent)
            } catch {
              /* ignore malformed event */
            }
          }
          separator = buffer.indexOf('\n\n')
        }
      }
    },
    // confirmAgentAction runs a write the assistant proposed (scheduling,
    // deletion, automations) only after the user confirms it in the chat.
    confirmAgentAction(teamID: string, tool: string, args: unknown) {
      return request<{ summary: string; payload: unknown }>(options, `/v1/teams/${teamID}/ai/confirm-action`, {
        method: 'POST',
        headers: buildHeaders(options.token),
        body: JSON.stringify({ tool, args }),
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
    getExternalPostMonitor(teamID: string) {
      return request<BackendExternalPostMonitorSettings>(options, `/v1/teams/${teamID}/external-post-monitor`, {
        headers: buildHeaders(options.token, false),
      })
    },
    upsertExternalPostMonitor(teamID: string, payload: { enabled: boolean }) {
      return request<BackendExternalPostMonitorSettings>(options, `/v1/teams/${teamID}/external-post-monitor`, {
        method: 'PUT',
        headers: buildHeaders(options.token),
        body: JSON.stringify(payload),
      })
    },
    importOldPosts(teamID: string, payload: { account_ids: string[]; limit: number; until_date?: string }) {
      return request<{ imported: number }>(options, `/v1/teams/${teamID}/import-old-posts`, {
        method: 'POST',
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
