import type {
  BackendAccount,
  BackendAIJob,
  BackendMembership,
  BackendPost,
  BackendProviderInstance,
  BackendReviewQueueItem,
  BackendRSSFeedConfig,
  BackendRuntimeConfig,
  BackendAuthStatus,
  BackendCampaignFormat,
  BackendKnowledgeSource,
  BackendProactiveTriggerSettings,
  BackendStyleExample,
  BackendStyleMetadata,
  BackendAIServiceConfig,
  BackendTeamProfile,
  BackendTeam,
  BackendUser,
} from './api'
import type {
  AIJob,
  AIServiceConfig,
  CampaignFormat,
  KnowledgeSource,
  AccountRecord,
  AuthStatusRecord,
  ProactiveTriggerSettings,
  RSSFeedConfig,
  PostRecord,
  ReviewQueueItem,
  ProviderInstanceRecord,
  ProviderName,
  StyleExample,
  StyleMetadata,
  RuntimeConfigRecord,
  TeamMemberRecord,
  TeamProfile,
  TeamRecord,
  TeamSchedulingPreferences,
  UserRecord,
} from './types'

export function colorForProvider(provider: ProviderName) {
  switch (provider) {
    case 'bluesky':
      return '#007AFF'
    case 'friendica':
      return '#1FC77A'
    case 'mastodon':
      return '#8B5CF6'
    case 'pixelfed':
      return '#F76C5E'
  }
}

/** Providers that reject text-only posts and require at least one media attachment. */
export function providerRequiresMedia(provider: ProviderName) {
  return provider === 'pixelfed'
}

export function maxCharsForProvider(provider: ProviderName) {
  switch (provider) {
    case 'bluesky':
      return 300
    case 'friendica':
      return 5000
    case 'mastodon':
      return 500
    case 'pixelfed':
      return 2000
  }
}

export function toUserRecord(user: BackendUser): UserRecord {
  return {
    id: user.id,
    name: user.name,
    email: user.email,
    globalRole: user.is_admin ? 'admin' : 'member',
    title: user.is_admin ? 'Administrator' : 'Team Member',
    createdAt: user.created_at ?? '',
  }
}

export function toTeamRecord(team: BackendTeam, members: TeamMemberRecord[], accountIds: string[]): TeamRecord {
  let scheduling: TeamSchedulingPreferences | undefined
  if (team.scheduling_preferences) {
    scheduling = {
      timezone: team.scheduling_preferences.timezone || 'UTC',
      posting_windows: team.scheduling_preferences.posting_windows ?? [],
      default_timeslots: team.scheduling_preferences.default_timeslots ?? [],
    }
  }
  return {
    id: team.id,
    name: team.name,
    description: team.description,
    members,
    accountIds,
    isAiEnabled: Boolean(team.is_ai_enabled),
    schedulingPreferences: scheduling,
    brandColor: team.brand_color ?? '',
  }
}

export function toTeamMemberRecord(membership: BackendMembership): TeamMemberRecord {
  return {
    userId: membership.user_id,
    role: membership.role,
  }
}

export function toAccountRecord(account: BackendAccount, instances: ProviderInstanceRecord[]): AccountRecord {
  const matchingInstance = instances.find((instance) => instance.id === account.provider_instance_id)
  const label = matchingInstance ? `${matchingInstance.name} · ${account.username}` : account.username
  return {
    id: account.id,
    teamId: account.team_id,
    name: label,
    provider: account.provider,
    instance: account.instance_url,
    providerInstanceId: account.provider_instance_id,
    username: account.username,
    authType: account.auth_type,
    avatarUrl: account.avatar_url?.trim() || undefined,
    accessTokenExpiresAt: account.access_token_expires_at?.trim() || undefined,
    accountMetricsSyncedAt: account.account_metrics_synced_at?.trim() || undefined,
    postEngagementSyncedAt: account.post_engagement_synced_at?.trim() || undefined,
    color: colorForProvider(account.provider),
    maxChars: account.max_chars_override ?? maxCharsForProvider(account.provider),
  }
}

/** OAuth accounts past access token expiry should reconnect; app-password accounts are treated as active. */
export function accountConnectionStatus(account: AccountRecord): 'active' | 'reauth' {
  if (account.authType !== 'oauth_token') {
    return 'active'
  }
  const raw = account.accessTokenExpiresAt?.trim()
  if (!raw) {
    return 'active'
  }
  const t = new Date(raw).getTime()
  if (Number.isNaN(t)) {
    return 'active'
  }
  return t < Date.now() ? 'reauth' : 'active'
}

export function toPostRecord(post: BackendPost): PostRecord {
  return {
    id: post.id,
    teamId: post.team_id,
    title: post.title || fallbackTitle(post.content),
    content: post.content,
    scheduledAt: post.scheduled_at,
    durationMinutes: 30,
    targetAccountIds: post.target_accounts ?? [],
    status: mapStatus(post.status),
    source:
      post.source === 'imported' ? 'imported' : post.source === 'automation' ? 'automation' : 'scheduled',
    publishedLinks: post.published_links,
    mediaIds: post.media_ids?.length ? post.media_ids : undefined,
    mediaExcludeByAccount: post.media_exclude_by_account && Object.keys(post.media_exclude_by_account).length > 0 ? post.media_exclude_by_account : undefined,
  }
}

export function toProviderInstanceRecord(instance: BackendProviderInstance): ProviderInstanceRecord {
  return {
    id: instance.id,
    provider: instance.provider,
    name: instance.name,
    instanceUrl: instance.instance_url,
    clientId: instance.client_id,
    hasClientSecret: instance.has_client_secret,
    scopes: instance.scopes ?? [],
    authorizationEndpoint: instance.authorization_endpoint ?? '',
    tokenEndpoint: instance.token_endpoint ?? '',
  }
}

export function toRuntimeConfigRecord(runtimeConfig: BackendRuntimeConfig): RuntimeConfigRecord {
  return {
    general: {
      httpAddr: runtimeConfig.general.http_addr,
    },
    security: {
      allowedOrigins: runtimeConfig.security.allowed_origins,
      rateLimitPerMinute: runtimeConfig.security.rate_limit_per_minute,
      rateLimitAuthenticatedPerMinute: runtimeConfig.security.rate_limit_authenticated_per_minute ?? 300,
      encryptionConfigured: runtimeConfig.security.encryption_configured,
    },
    scheduler: {
      pollInterval: runtimeConfig.scheduler.poll_interval,
      metricsSyncInterval: runtimeConfig.scheduler.metrics_sync_interval,
      accountHealthInterval: runtimeConfig.scheduler.account_health_interval,
      workers: runtimeConfig.scheduler.workers,
    },
    oidc: {
      enabled: runtimeConfig.oidc.enabled,
      issuerUrl: runtimeConfig.oidc.issuer_url,
      clientId: runtimeConfig.oidc.client_id,
      hasSecret: runtimeConfig.oidc.has_secret,
    },
  }
}

export function toAuthStatusRecord(status: BackendAuthStatus): AuthStatusRecord {
  return {
    bootstrapEnabled: status.bootstrap_enabled,
    bootstrapRecoveryEnabled: Boolean(status.bootstrap_recovery_enabled),
    initialSetupRequired: Boolean(status.initial_setup_required),
    oidcEnabled: status.oidc_enabled,
    oidcOAuthEnabled: status.oidc_oauth_enabled,
    hasUsers: status.has_users,
    hasAdminUsers: status.has_admin_users,
    appEnv: status.app_env ?? '',
  }
}

export function mapTeamProfile(raw: BackendTeamProfile): TeamProfile {
  return {
    id: raw.id,
    teamId: raw.team_id,
    styleMetadata: mapStyleMetadata(raw.style_metadata),
    autoPublishEnabled: raw.auto_publish_enabled,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
  }
}

export function mapCampaignFormat(raw: BackendCampaignFormat): CampaignFormat {
  return {
    id: raw.id,
    teamId: raw.team_id,
    name: raw.name,
    weekday: raw.weekday ?? null,
    structure: raw.structure,
    requiredHashtags: raw.required_hashtags,
    isActive: raw.is_active,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
  }
}

export function mapStyleExample(raw: BackendStyleExample): StyleExample {
  return {
    id: raw.id,
    teamId: raw.team_id,
    platform: raw.platform,
    content: raw.content,
    notes: raw.notes,
    createdAt: raw.created_at,
  }
}

export function mapAIJob(raw: BackendAIJob): AIJob {
  return {
    id: raw.id,
    teamId: raw.team_id,
    authorUserId: raw.author_user_id,
    type: raw.type,
    status: raw.status,
    payload: raw.payload,
    result: raw.result,
    errorMessage: raw.error_message,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
    completedAt: raw.completed_at,
  }
}

export function mapAIServiceConfig(raw: BackendAIServiceConfig): AIServiceConfig {
  return {
    id: raw.id,
    teamId: raw.team_id,
    provider: raw.provider,
    model: raw.model,
    baseUrl: raw.base_url,
    apiKeySet: Boolean(raw.api_key_set),
    description: raw.description,
    createdAt: raw.created_at,
  }
}

export function mapReviewQueueItem(raw: BackendReviewQueueItem): ReviewQueueItem {
  return {
    ...toPostRecord(raw),
    isOverdue: Boolean(raw.is_overdue),
    rssFeedName: raw.rss_feed_name ?? undefined,
  }
}

export function mapRSSFeedConfig(raw: BackendRSSFeedConfig): RSSFeedConfig {
  const outputMode = raw.output_mode === 'scheduled' || raw.output_mode === 'publish_now' ? raw.output_mode : 'draft'
  return {
    id: raw.id,
    teamId: raw.team_id,
    feedUrl: raw.feed_url,
    name: raw.name,
    isActive: raw.is_active,
    aiEnhanceEnabled: Boolean(raw.ai_enhance_enabled),
    contentTemplate: raw.content_template ?? '{title}\n\n{link}',
    titleTemplate: raw.title_template ?? '{title}',
    titleHint: raw.title_hint ?? '',
    outputMode,
    maxPostsPerDay: raw.max_posts_per_day ?? 10,
    counterNext: raw.counter_next,
    promptHint: raw.prompt_hint ?? '',
    targetAccountIds: raw.target_account_ids ?? [],
    tonality: raw.tonality ?? '',
    initialSyncMode: raw.initial_sync_mode === 'publish_latest' ? 'publish_latest' : 'baseline',
    lastFetchedAt: raw.last_fetched_at,
    createdAt: raw.created_at,
  }
}

export function mapProactiveTriggerSettings(raw: BackendProactiveTriggerSettings): ProactiveTriggerSettings {
  return {
    id: raw.id,
    teamId: raw.team_id,
    contentGapThresholdDays: raw.content_gap_threshold_days,
    autoFillEnabled: raw.auto_fill_enabled,
    maxTriggersPerDay: raw.max_triggers_per_day,
    cronSchedule: raw.cron_schedule,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
  }
}

function mapStatus(status: BackendPost['status']): PostRecord['status'] {
  switch (status) {
    case 'posted':
      return 'posted'
    case 'failed':
      return 'failed'
    case 'draft':
      return 'draft'
    default:
      return 'scheduled'
  }
}

function fallbackTitle(content: string) {
  const trimmed = content.trim()
  if (trimmed.length <= 36) {
    return trimmed || 'Untitled post'
  }
  return `${trimmed.slice(0, 33)}...`
}

export function mapKnowledgeSource(raw: BackendKnowledgeSource): KnowledgeSource {
  return {
    id: raw.id,
    teamId: raw.team_id,
    type: raw.type,
    name: raw.name,
    content: raw.content,
    sourceUrl: raw.source_url,
    mediaId: raw.media_id,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
  }
}

function mapStyleMetadata(raw: BackendStyleMetadata): StyleMetadata {
  return {
    tonality: raw.tonality ?? '',
    formattingRules: raw.formatting_rules ?? [],
    bannedWords: raw.banned_words ?? [],
    maxHashtags: raw.max_hashtags,
    preferredLanguage: raw.preferred_language,
    identity: raw.identity
      ? {
          archetype: raw.identity.archetype ?? '',
          persona: raw.identity.persona ?? '',
          industry: raw.identity.industry ?? '',
          mainValue: raw.identity.main_value ?? '',
          targetAudience: raw.identity.target_audience ?? '',
        }
      : undefined,
    languageDna: raw.language_dna
      ? {
          sentenceStyle: raw.language_dna.sentence_style ?? '',
          preferredWords: raw.language_dna.preferred_words ?? [],
          signaturePhrases: raw.language_dna.signature_phrases ?? [],
          humorStyle: raw.language_dna.humor_style ?? '',
          antiAiOverride: Boolean(raw.language_dna.anti_ai_override),
        }
      : undefined,
    reachStrategy: raw.reach_strategy
      ? {
          hookStyle: raw.reach_strategy.hook_style ?? '',
          ctaFocus: raw.reach_strategy.cta_focus ?? '',
        }
      : undefined,
  }
}
