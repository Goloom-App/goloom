import { isValid, parseISO } from 'date-fns'

import type { BackendAPIToken } from '../../api'

export const WEB_SESSION_API_TOKEN_NAME = '__web_session'

export function apiTokenDisplayName(name: string): string {
  return name === WEB_SESSION_API_TOKEN_NAME ? 'Web session' : name
}

export function isApiTokenExpired(token: BackendAPIToken, now = Date.now()): boolean {
  if (!token.expires_at) {
    return false
  }
  const expires = parseISO(token.expires_at)
  return isValid(expires) && expires.getTime() <= now
}
