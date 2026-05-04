import type { ProviderName } from '../../types'

export type AdminProviderDraft = {
  provider: ProviderName
  name: string
  instanceUrl: string
  clientId: string
  clientSecret: string
  scopes: string
  authorizationEndpoint: string
  tokenEndpoint: string
}

export function defaultAdminProviderDraft(): AdminProviderDraft {
  return {
    provider: 'mastodon',
    name: '',
    instanceUrl: '',
    clientId: '',
    clientSecret: '',
    scopes: 'read,write',
    authorizationEndpoint: '',
    tokenEndpoint: '',
  }
}
