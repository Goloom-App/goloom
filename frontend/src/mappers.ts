import type {
  BackendAccount,
  BackendMembership,
  BackendPost,
  BackendProviderInstance,
  BackendRuntimeConfig,
  BackendAuthStatus,
  BackendTeam,
  BackendUser,
} from './api'
import type {
  AccountRecord,
  AuthStatusRecord,
  PostRecord,
  ProviderInstanceRecord,
  ProviderName,
  RuntimeConfigRecord,
  TeamMemberRecord,
  TeamRecord,
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
  }
}

export function maxCharsForProvider(provider: ProviderName) {
  switch (provider) {
    case 'bluesky':
      return 300
    case 'friendica':
      return 5000
    case 'mastodon':
      return 500
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
  return {
    id: team.id,
    name: team.name,
    description: team.description,
    members,
    accountIds,
    isPersonal: Boolean(team.is_personal),
    personalForUserId: team.personal_for_user_id,
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
    color: colorForProvider(account.provider),
    maxChars: account.max_chars_override ?? maxCharsForProvider(account.provider),
  }
}

export function toPostRecord(post: BackendPost): PostRecord {
  return {
    id: post.id,
    teamId: post.team_id,
    title: post.title || fallbackTitle(post.content),
    content: post.content,
    scheduledAt: post.scheduled_at,
    durationMinutes: 30,
    targetAccountIds: post.target_accounts,
    status: mapStatus(post.status),
    publishedLinks: post.published_links,
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
      encryptionConfigured: runtimeConfig.security.encryption_configured,
    },
    scheduler: {
      pollInterval: runtimeConfig.scheduler.poll_interval,
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

function mapStatus(status: BackendPost['status']): PostRecord['status'] {
  switch (status) {
    case 'posted':
      return 'posted'
    case 'failed':
      return 'failed'
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
