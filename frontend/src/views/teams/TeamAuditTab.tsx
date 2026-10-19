import type { ComponentProps } from 'react'
import { TeamAuditLogSection } from './TeamAuditLogSection'

export function TeamAuditTab(props: ComponentProps<typeof TeamAuditLogSection>) {
  return <TeamAuditLogSection {...props} />
}
