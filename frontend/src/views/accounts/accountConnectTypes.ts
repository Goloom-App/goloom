import type { ProviderName } from '../../types'

export type AccountConnectDraft = {
  provider: ProviderName
  providerInstanceId: string
  instanceUrl: string
  accessToken: string
  refreshToken: string
  identifier: string
  appPassword: string
  blueskyAuthMode: 'app_password' | 'access_token'
}

export function defaultAccountConnectDraft(): AccountConnectDraft {
  return {
    provider: 'mastodon',
    providerInstanceId: '',
    instanceUrl: '',
    accessToken: '',
    refreshToken: '',
    identifier: '',
    appPassword: '',
    blueskyAuthMode: 'app_password',
  }
}
