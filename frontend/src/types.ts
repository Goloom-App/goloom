export type AppSection =
  | 'dashboard'
  | 'calendar'
  | 'contentCalendar'
  | 'archive'
  | 'analytics'
  | 'mediaLibrary'
  | 'management'
  | 'teams'
  | 'recurringPosts'
  | 'accounts'
  | 'settings'
  | 'admin'

export type CalendarViewMode = 'month' | 'week' | 'day'

export type TeamRole = 'owner' | 'editor' | 'viewer'

export type GlobalRole = 'admin' | 'member'

export type ProviderName = 'bluesky' | 'friendica' | 'mastodon'

export type PostStatus = 'scheduled' | 'posted' | 'failed' | 'draft'

export type AccountAuthType = 'oauth_token' | 'app_password'

export interface UserRecord {
  id: string
  name: string
  email: string
  globalRole: GlobalRole
  title: string
  /** ISO timestamp from the server */
  createdAt: string
}

export interface TeamMemberRecord {
  userId: string
  role: TeamRole
}

export interface AccountRecord {
  id: string
  teamId: string
  name: string
  provider: ProviderName
  instance: string
  providerInstanceId?: string
  username: string
  authType?: AccountAuthType
  /** Profile image URL from the provider when onboarding fetched it */
  avatarUrl?: string
  /** OAuth access token expiry when known (ISO); app-password accounts omit this */
  accessTokenExpiresAt?: string
  color: string
  maxChars: number
}

export interface TeamSchedulingPreferences {
  timezone: string
  posting_windows: Array<{ weekday: number; start: string; end: string }>
  default_timeslots: string[]
}

export interface TeamRecord {
  id: string
  name: string
  description: string
  members: TeamMemberRecord[]
  accountIds: string[]
  isPersonal: boolean
  personalForUserId?: string
  schedulingPreferences?: TeamSchedulingPreferences
}

export interface PostRecord {
  id: string
  teamId: string
  title: string
  content: string
  scheduledAt: string
  durationMinutes: number
  targetAccountIds: string[]
  status: PostStatus
  publishedLinks?: Record<string, string>
  /** Platform media attachment IDs (Mastodon media IDs, Bluesky-encoded payloads, etc.) */
  mediaIds?: string[]
  /** Per destination: media library IDs excluded from that publish */
  mediaExcludeByAccount?: Record<string, string[]>
}

/** Normalized engagement row for UI (maps from BackendPostMetric). */
export interface PostMetricRecord {
  postId: string
  accountId: string
  metric: string
  value: number
  updatedAt: string
}

/** Per-account draft override for a post (maps from BackendPostVersion). */
export interface PostVersionRecord {
  postId: string
  accountId: string
  content: string
}

/** Workspace analytics snapshot (maps from BackendTeamAnalytics). */
export interface TeamAnalyticsRecord {
  metricsTotal: Record<string, number>
  topPosts: Array<{ postId: string; title: string; score: number }>
}

export interface ProviderInstanceRecord {
  id: string
  provider: ProviderName
  name: string
  instanceUrl: string
  clientId: string
  hasClientSecret: boolean
  scopes: string[]
  authorizationEndpoint: string
  tokenEndpoint: string
}

export interface ProviderSetting {
  enabled: boolean
  defaultMaxChars: number
  mediaUploads: boolean
}

export interface SettingsState {
  ui: {
    colorScheme: 'system' | 'dark' | 'light'
  }
  general: {
    apiBaseUrl: string
    bearerToken: string
    timezone: string
    defaultCalendarView: CalendarViewMode
    slotMinutes: number
  }
  oidc: {
    issuerUrl: string
    clientId: string
    redirectUrl: string
    enableOIDC: boolean
    enforcePKCE: boolean
  }
  security: {
    rateLimitPerMinute: number
    corsOrigins: string
    sanitizeInput: boolean
    encryptProviderTokens: boolean
    allowAPITokens: boolean
  }
  scheduler: {
    pollIntervalSeconds: number
    workerCount: number
    retryLimit: number
    retryStrategy: string
  }
  providers: Record<ProviderName, ProviderSetting>
}

export interface RuntimeConfigRecord {
  general: {
    httpAddr: string
  }
  security: {
    allowedOrigins: string[]
    rateLimitPerMinute: number
    rateLimitAuthenticatedPerMinute: number
    encryptionConfigured: boolean
  }
  scheduler: {
    pollInterval: string
    workers: number
  }
  oidc: {
    enabled: boolean
    issuerUrl: string
    clientId: string
    hasSecret: boolean
  }
}

export interface AuthStatusRecord {
  bootstrapEnabled: boolean
  bootstrapRecoveryEnabled: boolean
  initialSetupRequired: boolean
  oidcEnabled: boolean
  oidcOAuthEnabled: boolean
  hasUsers: boolean
  hasAdminUsers: boolean
  appEnv: string
}

export interface SystemMetric {
  label: string
  value: string
  tone?: 'default' | 'success' | 'warning'
}
