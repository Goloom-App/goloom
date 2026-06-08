import { DestinationAvatar } from '../post/DestinationAvatar'
import type { AccountRecord } from '../../types'

interface DestinationPickerProps {
  accounts: AccountRecord[]
  selectedIds: string[]
  onToggle: (accountId: string) => void
  testIdPrefix?: string
}

export function DestinationPicker({ accounts, selectedIds, onToggle, testIdPrefix = 'ai-dest' }: DestinationPickerProps) {
  if (accounts.length === 0) {
    return <p className="hint">No accounts connected to this team.</p>
  }

  return (
    <div className="composer-destination-row" role="group" aria-label="Target accounts">
      {accounts.map((account) => {
        const selected = selectedIds.includes(account.id)
        return (
          <button
            key={account.id}
            type="button"
            data-testid={`${testIdPrefix}-${account.id}`}
            className={`composer-destination-toggle ${selected ? 'composer-destination-toggle--selected' : ''}`}
            aria-pressed={selected}
            title={`${account.name} · ${account.provider}`}
            onClick={() => onToggle(account.id)}
          >
            <DestinationAvatar account={account} />
          </button>
        )
      })}
    </div>
  )
}
