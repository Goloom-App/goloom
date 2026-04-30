import type { ProviderName } from './types'

export interface ApiClientOptions {
  baseUrl: string
  token: string
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
  max_chars_override?: number
  created_at: string
}

export interface BackendPost {
  id: string
  team_id: string
  author_user_id: string
  title: string
  content: string
  scheduled_at: string
  status: 'pending' | 'processing' | 'posted' | 'failed' | 'cancelled'
  attempt_count: number
  last_error?: string
  created_at: string
  updated_at: string
  target_accounts: string[]
  published_links?: Record<string, string>
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
  }
  security: {
    allowed_origins: string[]
    rate_limit_per_minute: number
    encryption_configured: boolean
  }
  scheduler: {
    poll_interval: string
    workers: number
  }
  oidc: {
    enabled: boolean
    issuer_url: string
    client_id: string
    has_secret: boolean
  }
}

function buildHeaders(token: string, withJSON = true) {
  const headers = new Headers()
  headers.set('Authorization', `Bearer ${token}`)
  if (withJSON) {
    headers.set('Content-Type', 'application/json')
  }
  return headers
}

async function request<T>(options: ApiClientOptions, path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${options.baseUrl.replace(/\/$/, '')}${path}`, init)
  if (!response.ok) {
    const message = await response.text()
    throw new Error(message || `Request failed with ${response.status}`)
  }
  if (response.status === 204) {
    return undefined as T
  }
  return (await response.json()) as T
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
      client_id: string
      client_secret: string
      scopes: string[]
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
        client_id: string
        client_secret?: string
        scopes: string[]
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
        provider_instance_id: string
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
    deleteAccount(teamID: string, accountID: string) {
      return request<void>(options, `/v1/teams/${teamID}/accounts/${accountID}`, {
        method: 'DELETE',
        headers: buildHeaders(options.token, false),
      })
    },
    listPosts(teamID: string) {
      return request<{ items: BackendPost[] }>(options, `/v1/teams/${teamID}/posts`, {
        headers: buildHeaders(options.token, false),
      })
    },
    createPost(
      teamID: string,
      payload: { title: string; content: string; scheduled_at: string; target_accounts: string[] },
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
      payload: { title: string; content: string; scheduled_at: string; target_accounts: string[] },
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
  }
}
