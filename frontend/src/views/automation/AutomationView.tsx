import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CalendarClock, Rss } from 'lucide-react'

import { createApiClient } from '../../api'
import { PageTabs } from '../../components/ui/PageTabs'
import type { AccountRecord, TeamRecord } from '../../types'
import { RecurringPostsView } from '../recurring/RecurringPostsView'
import { RSSFeedsView } from '../rss/RSSFeedsView'

type AutomationTab = 'recurring' | 'rss'

interface AutomationViewProps {
  team: TeamRecord
  api: ReturnType<typeof createApiClient>
  accounts: AccountRecord[]
  canEdit: boolean
  onStatus: (msg: string | null) => void
  onPostsRefresh?: () => void | Promise<void>
}

export function AutomationView({ team, api, accounts, canEdit, onStatus, onPostsRefresh }: AutomationViewProps) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<AutomationTab>('recurring')

  return (
    <div className="brand-wizard stack stack--lg" data-testid="automation-view">
      <PageTabs
        label={t('automation.tabs')}
        value={tab}
        onChange={setTab}
        testIdPrefix="automation-tab"
        idPrefix="automation-tab"
        panelId="automation-panel"
        options={[
          { id: 'recurring', label: <><CalendarClock size={16} />{t('automation.tabRecurring')}</> },
          { id: 'rss', label: <><Rss size={16} />{t('automation.tabRss')}</> },
        ]}
      />

      <div id="automation-panel" role="tabpanel" tabIndex={0} aria-labelledby={`automation-tab-${tab}`}>
      {tab === 'recurring' ? (
        <RecurringPostsView
          teamId={team.id}
          api={api}
          accounts={accounts}
          canEdit={canEdit}
          onStatus={onStatus}
          onPostsRefresh={onPostsRefresh}
          team={team}
        />
      ) : (
        <RSSFeedsView team={team} accounts={accounts} canEdit={canEdit} />
      )}
      </div>
    </div>
  )
}
