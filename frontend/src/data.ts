import type {
  AccountRecord,
  PostRecord,
  SettingsState,
  SystemMetric,
  TeamRecord,
} from './types'

export const initialTeams: TeamRecord[] = [
  {
    id: 'team-1',
    name: 'Marketing',
    description: 'Campaign planning for product launches and partner announcements.',
    members: [
      { userId: 'user-1', role: 'owner' },
      { userId: 'user-2', role: 'editor' },
      { userId: 'user-3', role: 'viewer' },
    ],
    accountIds: ['account-1', 'account-2'],
    isPersonal: false,
  },
  {
    id: 'team-2',
    name: 'Ops',
    description: 'Service advisories, release notes, and operational updates.',
    members: [
      { userId: 'user-1', role: 'owner' },
      { userId: 'user-4', role: 'editor' },
    ],
    accountIds: ['account-3', 'account-4'],
    isPersonal: false,
  },
]

export const initialAccounts: AccountRecord[] = [
  {
    id: 'account-1',
    teamId: 'team-1',
    name: 'Main Mastodon',
    provider: 'mastodon',
    instance: 'https://fosstodon.org',
    username: '@goloom',
    color: '#7c5cff',
    maxChars: 500,
  },
  {
    id: 'account-2',
    teamId: 'team-1',
    name: 'Blue Launches',
    provider: 'bluesky',
    instance: 'https://bsky.social',
    username: '@goloom.bsky.social',
    color: '#2d9cff',
    maxChars: 300,
  },
  {
    id: 'account-3',
    teamId: 'team-2',
    name: 'Friendica Status',
    provider: 'friendica',
    instance: 'https://social.example.org',
    username: '@ops',
    color: '#0fa37f',
    maxChars: 5000,
  },
  {
    id: 'account-4',
    teamId: 'team-2',
    name: 'Incident Mastodon',
    provider: 'mastodon',
    instance: 'https://mastodon.online',
    username: '@incident-bot',
    color: '#ff7b5c',
    maxChars: 500,
  },
]

export const initialPosts: PostRecord[] = [
  {
    id: 'post-1',
    teamId: 'team-1',
    title: 'Feature teaser',
    content:
      'Preview the new scheduling dashboard, highlight the drag and drop workflow, and invite beta signups.',
    scheduledAt: '2026-04-30T10:00:00.000Z',
    durationMinutes: 30,
    targetAccountIds: ['account-1', 'account-2'],
    status: 'posted',
    publishedLinks: {
      'account-1': 'https://fosstodon.org/@goloom/114418822000100001',
      'account-2': 'https://bsky.app/profile/goloom.bsky.social/post/3lnfeatureteaser',
    },
  },
  {
    id: 'post-2',
    teamId: 'team-1',
    title: 'Partner spotlight',
    content:
      'Celebrate the new integration partner and share the setup guide link for administrators.',
    scheduledAt: '2026-04-30T10:00:00.000Z',
    durationMinutes: 30,
    targetAccountIds: ['account-1'],
    status: 'posted',
    publishedLinks: {
      'account-1': 'https://fosstodon.org/@goloom/114418822000100002',
    },
  },
  {
    id: 'post-3',
    teamId: 'team-1',
    title: 'May calendar drop',
    content:
      'Publish the editorial calendar for May with a breakdown of livestreams, case studies, and launch dates.',
    scheduledAt: '2026-05-02T13:30:00.000Z',
    durationMinutes: 30,
    targetAccountIds: ['account-2'],
    status: 'posted',
    publishedLinks: {
      'account-2': 'https://bsky.app/profile/goloom.bsky.social/post/3lnmaycaldrop',
    },
  },
  {
    id: 'post-4',
    teamId: 'team-2',
    title: 'Maintenance notice',
    content:
      'Planned maintenance window on Saturday at 23:00 UTC. Expect brief API interruptions and delayed queue processing.',
    scheduledAt: '2026-05-04T09:00:00.000Z',
    durationMinutes: 30,
    targetAccountIds: ['account-3', 'account-4'],
    status: 'scheduled',
  },
  {
    id: 'post-5',
    teamId: 'team-2',
    title: 'Release note summary',
    content:
      'Summarize the latest release, mention the scheduler retry improvements, and link to the full changelog.',
    scheduledAt: '2026-05-04T09:30:00.000Z',
    durationMinutes: 30,
    targetAccountIds: ['account-4'],
    status: 'scheduled',
  },
  {
    id: 'post-6',
    teamId: 'team-1',
    title: 'Community Q&A',
    content:
      'Collect questions for the Friday Q&A and ask users which networks they want to see supported next.',
    scheduledAt: '2026-05-07T16:00:00.000Z',
    durationMinutes: 30,
    targetAccountIds: ['account-1'],
    status: 'scheduled',
  },
]

export const initialSettings: SettingsState = {
  ui: {
    colorScheme: 'system',
  },
  general: {
    apiBaseUrl: '',
    bearerToken: '',
    timezone: 'Europe/Berlin',
    defaultCalendarView: 'month',
    slotMinutes: 30,
  },
  oidc: {
    issuerUrl: 'https://auth.example.com/realms/goloom',
    clientId: 'goloom-web',
    redirectUrl: 'http://localhost:5173/auth/callback',
    enableOIDC: true,
    enforcePKCE: true,
  },
  security: {
    rateLimitPerMinute: 120,
    corsOrigins: 'http://localhost:3000, http://localhost:5173',
    sanitizeInput: true,
    encryptProviderTokens: true,
    allowAPITokens: true,
  },
  scheduler: {
    pollIntervalSeconds: 15,
    workerCount: 4,
    retryLimit: 5,
    retryStrategy: 'Quadratic backoff',
  },
  providers: {
    bluesky: {
      enabled: true,
      defaultMaxChars: 300,
      mediaUploads: true,
    },
    friendica: {
      enabled: true,
      defaultMaxChars: 5000,
      mediaUploads: true,
    },
    mastodon: {
      enabled: true,
      defaultMaxChars: 500,
      mediaUploads: true,
    },
  },
}

export const initialMetrics: SystemMetric[] = [
  { label: 'Queued posts', value: '18', tone: 'default' },
  { label: 'Workers healthy', value: '4 / 4', tone: 'success' },
  { label: 'Failed deliveries', value: '2', tone: 'warning' },
  { label: 'Active teams', value: '2', tone: 'default' },
]
