import { useState } from 'react'
import { PageTabs } from '../../components/ui/PageTabs'
import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'

import type { BackendAdminMetrics, BackendAdminSyncStatus, BackendPublishFailure, BackendTeam } from '../../api'
import type { createApiClient } from '../../api'
import type { AccountRecord, ProviderInstanceRecord, RuntimeConfigRecord, UserRecord } from '../../types'
import type { AdminProviderDraft } from './adminTypes'
import { AdminAITab } from './AdminAITab'
import { AdminConfigurationsTab } from './AdminConfigurationsTab'
import { AdminLogsTab } from './AdminLogsTab'
import { AdminProvidersTab } from './AdminProvidersTab'
import { AdminStatusTab } from './AdminStatusTab'
import { AdminUsersTab } from './AdminUsersTab'

export type AdminTab = 'status' | 'configurations' | 'providers' | 'users' | 'logs' | 'ai'

export function AdminView({
  adminMetrics,
  adminMetricsLoading,
  adminRuntime,
  adminSyncStatus,
  adminSyncLoading,
  onTriggerMetricsSync,
  publishFailures,
  onAcknowledgePublishFailure,
  onRetryPublishFailure,
  directoryUsers,
  providerInstances,
  accounts,
  adminProviderDraft,
  setAdminProviderDraft,
  editingProviderId,
  setEditingProviderId,
  showAdminProviderAdvanced,
  setShowAdminProviderAdvanced,
  syncing,
  onSaveAdminProvider,
  onDeleteProviderInstance,
  api,
  adminAIEnabledTeams,
  adminAIEnabledTeamsLoading,
  onLoadAdminAIEnabledTeams,
}: {
  api: ReturnType<typeof createApiClient> | null
  adminMetrics: BackendAdminMetrics | null
  adminMetricsLoading: boolean
  adminRuntime: RuntimeConfigRecord | null
  adminSyncStatus: BackendAdminSyncStatus | null
  adminSyncLoading: boolean
  onTriggerMetricsSync: () => void | Promise<void>
  publishFailures: BackendPublishFailure[]
  onAcknowledgePublishFailure: (postID: string) => void | Promise<void>
  onRetryPublishFailure: (postID: string) => void | Promise<void>
  directoryUsers: UserRecord[]
  providerInstances: ProviderInstanceRecord[]
  accounts: AccountRecord[]
  adminProviderDraft: AdminProviderDraft
  setAdminProviderDraft: Dispatch<SetStateAction<AdminProviderDraft>>
  editingProviderId: string | null
  setEditingProviderId: (id: string | null) => void
  showAdminProviderAdvanced: boolean
  setShowAdminProviderAdvanced: (open: boolean) => void
  syncing: boolean
  onSaveAdminProvider: () => void | Promise<void>
  onDeleteProviderInstance: (instanceId: string) => void | Promise<void>
  adminAIEnabledTeams: BackendTeam[]
  adminAIEnabledTeamsLoading: boolean
  onLoadAdminAIEnabledTeams: () => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<AdminTab>('status')

  const tabs: { id: AdminTab; label: string }[] = [
    { id: 'status', label: t('admin.tabStatus') },
    { id: 'configurations', label: t('admin.tabConfigurations') },
    { id: 'providers', label: t('admin.tabProviders') },
    { id: 'users', label: t('admin.tabUsers') },
    { id: 'logs', label: t('admin.tabLogs') },
    { id: 'ai', label: 'AI Agents' },
  ]

  return (
    <div className="admin-view">
      <PageTabs
        label={t('admin.tabsAria')}
        value={activeTab}
        options={tabs}
        onChange={setActiveTab}
        idPrefix="admin-tab"
        panelId="admin-tab-panel"
        tourPrefix="admin-tab"
      />

      <div className="admin-view__body" id="admin-tab-panel" role="tabpanel" tabIndex={0} aria-labelledby={`admin-tab-${activeTab}`}>
        {activeTab === 'status' ? (
          <AdminStatusTab
            adminMetrics={adminMetrics}
            adminMetricsLoading={adminMetricsLoading}
            adminRuntime={adminRuntime}
            adminSyncStatus={adminSyncStatus}
            adminSyncLoading={adminSyncLoading}
            syncing={syncing}
            onTriggerMetricsSync={onTriggerMetricsSync}
            publishFailures={publishFailures}
            onAcknowledgePublishFailure={onAcknowledgePublishFailure}
            onRetryPublishFailure={onRetryPublishFailure}
          />
        ) : null}

        {activeTab === 'configurations' ? <AdminConfigurationsTab adminRuntime={adminRuntime} /> : null}

        {activeTab === 'providers' ? (
          <AdminProvidersTab
            api={api}
            providerInstances={providerInstances}
            accounts={accounts}
            adminProviderDraft={adminProviderDraft}
            setAdminProviderDraft={setAdminProviderDraft}
            editingProviderId={editingProviderId}
            setEditingProviderId={setEditingProviderId}
            showAdminProviderAdvanced={showAdminProviderAdvanced}
            setShowAdminProviderAdvanced={setShowAdminProviderAdvanced}
            syncing={syncing}
            onSaveAdminProvider={onSaveAdminProvider}
            onDeleteProviderInstance={onDeleteProviderInstance}
          />
        ) : null}

        {activeTab === 'users' ? <AdminUsersTab directoryUsers={directoryUsers} /> : null}

        {activeTab === 'logs' ? <AdminLogsTab api={api} /> : null}

        {activeTab === 'ai' ? (
          <AdminAITab
            teams={adminAIEnabledTeams}
            loading={adminAIEnabledTeamsLoading}
            onLoad={onLoadAdminAIEnabledTeams}
          />
        ) : null}
      </div>
    </div>
  )
}
