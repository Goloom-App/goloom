import { useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'

import type { BackendAdminMetrics, BackendAdminSyncStatus } from '../../api'
import type { createApiClient } from '../../api'
import type { AccountRecord, ProviderInstanceRecord, RuntimeConfigRecord, UserRecord } from '../../types'
import type { AdminProviderDraft } from './adminTypes'
import { AdminConfigurationsTab } from './AdminConfigurationsTab'
import { AdminLogsTab } from './AdminLogsTab'
import { AdminProvidersTab } from './AdminProvidersTab'
import { AdminStatusTab } from './AdminStatusTab'
import { AdminUsersTab } from './AdminUsersTab'

export type AdminTab = 'status' | 'configurations' | 'providers' | 'users' | 'logs'

export function AdminView({
  adminMetrics,
  adminMetricsLoading,
  adminRuntime,
  adminSyncStatus,
  adminSyncLoading,
  onTriggerMetricsSync,
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
}: {
  api: ReturnType<typeof createApiClient> | null
  adminMetrics: BackendAdminMetrics | null
  adminMetricsLoading: boolean
  adminRuntime: RuntimeConfigRecord | null
  adminSyncStatus: BackendAdminSyncStatus | null
  adminSyncLoading: boolean
  onTriggerMetricsSync: () => void | Promise<void>
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
}) {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<AdminTab>('status')

  const tabs: { id: AdminTab; label: string }[] = [
    { id: 'status', label: t('admin.tabStatus') },
    { id: 'configurations', label: t('admin.tabConfigurations') },
    { id: 'providers', label: t('admin.tabProviders') },
    { id: 'users', label: t('admin.tabUsers') },
    { id: 'logs', label: t('admin.tabLogs') },
  ]

  return (
    <div className="admin-view">
      <header className="admin-view__header page-header" style={{ marginBottom: 0 }}>
        <div>
          <p className="eyebrow">{t('section.admin')}</p>
          <h1 className="admin-view__title">{t('admin.pageTitle')}</h1>
          <p className="hint admin-view__subtitle">{t('admin.pageSubtitle')}</p>
        </div>
        <div className="page-header__actions">
          <nav className="view-toggle view-toggle--scrollable" role="tablist" aria-label={t('admin.tabsAria')}>
            {tabs.map((tab) => (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={activeTab === tab.id}
                className={`view-toggle__btn ${activeTab === tab.id ? 'view-toggle__btn--active' : ''}`}
                onClick={() => setActiveTab(tab.id)}
              >
                {tab.label}
              </button>
            ))}
          </nav>
        </div>
      </header>

      <div className="admin-view__body" role="tabpanel">
        {activeTab === 'status' ? (
          <AdminStatusTab
            adminMetrics={adminMetrics}
            adminMetricsLoading={adminMetricsLoading}
            adminRuntime={adminRuntime}
            adminSyncStatus={adminSyncStatus}
            adminSyncLoading={adminSyncLoading}
            syncing={syncing}
            onTriggerMetricsSync={onTriggerMetricsSync}
          />
        ) : null}

        {activeTab === 'configurations' ? <AdminConfigurationsTab adminRuntime={adminRuntime} /> : null}

        {activeTab === 'providers' ? (
          <AdminProvidersTab
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
      </div>
    </div>
  )
}
