import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CalendarClock, Rss, Zap } from 'lucide-react'

import { createApiClient } from '../../api'
import { SectionCard } from '../../components/ui'
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
}

export function AutomationView({ team, api, accounts, canEdit, onStatus }: AutomationViewProps) {
  const { t } = useTranslation()
  const [tab, setTab] = useState<AutomationTab>('recurring')

  return (
    <div className="brand-wizard stack stack--lg" data-testid="automation-view">
      <SectionCard
        icon={<Zap size={18} />}
        title={t('automation.title', 'Automation')}
        subtitle={t('automation.hint')}
      >
        <div className="flex-row--center gap-2" role="tablist" aria-label={t('automation.tabs')}>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'recurring'}
            data-testid="automation-tab-recurring"
            className={`btn btn--sm ${tab === 'recurring' ? 'btn--secondary' : 'btn--ghost'}`}
            onClick={() => setTab('recurring')}
          >
            <CalendarClock size={16} />
            <span>{t('automation.tabRecurring')}</span>
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'rss'}
            data-testid="automation-tab-rss"
            className={`btn btn--sm ${tab === 'rss' ? 'btn--secondary' : 'btn--ghost'}`}
            onClick={() => setTab('rss')}
          >
            <Rss size={16} />
            <span>{t('automation.tabRss')}</span>
          </button>
        </div>
      </SectionCard>

      {tab === 'recurring' ? (
        <RecurringPostsView
          teamId={team.id}
          api={api}
          accounts={accounts}
          canEdit={canEdit}
          onStatus={onStatus}
          team={team}
        />
      ) : (
        <RSSFeedsView team={team} accounts={accounts} canEdit={canEdit} />
      )}
    </div>
  )
}
