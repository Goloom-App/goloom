import type { Dispatch, SetStateAction } from 'react'
import type { createApiClient } from '../../api'
import type { TeamRecord, TeamRole, UserRecord } from '../../types'

export interface TeamSettingsViewProps {
  api: ReturnType<typeof createApiClient> | null
  selectedTeam: TeamRecord
  myRoleInSelectedTeam: TeamRole | null
  principalUser: UserRecord | null
  directoryUsers: UserRecord[]
  syncing: boolean
  teamSettingsName: string
  setTeamSettingsName: (value: string) => void
  teamSettingsDescription: string
  setTeamSettingsDescription: (value: string) => void
  teamAiEnabled: boolean
  setTeamAiEnabled: (value: boolean) => void
  teamAiProvider: string
  setTeamAiProvider: (value: string) => void
  teamAiModel: string
  setTeamAiModel: (value: string) => void
  teamAiApiKey: string
  setTeamAiApiKey: (value: string) => void
  teamAiBaseUrl: string
  setTeamAiBaseUrl: (value: string) => void
  teamAiKeySet: boolean
  teamBrandColor: string
  setTeamBrandColor: (value: string) => void
  externalPostMonitorEnabled: boolean
  onToggleExternalPostMonitor: (enabled: boolean) => void
  onOpenImportOldPosts: () => void
  memberRoleEdits: Record<string, TeamRole>
  setMemberRoleEdits: Dispatch<SetStateAction<Record<string, TeamRole>>>
  addMemberUserId: string
  setAddMemberUserId: (value: string) => void
  onUpdateTeam: () => void
  onSaveAiServiceConfig: () => void
  onSaveBrandColor: (color: string) => void
  onAddTeamMember: (onSuccess: () => void) => void
  onRemoveTeamMember: (userId: string) => void
  onChangeTeamMemberRole: (userId: string) => void
}
