import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { AccountRecord } from '../../types'

interface ImportOldPostsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  accounts: AccountRecord[]
  onImport: (accountIds: string[], limit: number, untilDate?: string) => Promise<number>
}

export function ImportOldPostsDialog({ open, onOpenChange, accounts, onImport }: ImportOldPostsDialogProps) {
  const { t } = useTranslation()
  const [selectedAccountIds, setSelectedAccountIds] = useState<string[]>([])
  const [mode, setMode] = useState<'count' | 'date'>('count')
  const [postCount, setPostCount] = useState(100)
  const [untilDate, setUntilDate] = useState('')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)

  function toggleAccount(id: string) {
    setSelectedAccountIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
  }

  function selectAll() {
    setSelectedAccountIds(accounts.map((a) => a.id))
  }

  async function handleImport() {
    if (selectedAccountIds.length === 0) return
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const imported = await onImport(
        selectedAccountIds,
        mode === 'count' ? postCount : 500,
        mode === 'date' && untilDate ? untilDate : undefined,
      )
      setResult(imported)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Import failed')
    } finally {
      setLoading(false)
    }
  }

  function handleClose() {
    setSelectedAccountIds([])
    setResult(null)
    setError(null)
    onOpenChange(false)
  }

  return (
    <Dialog.Root open={open} onOpenChange={handleClose}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog" style={{ maxWidth: 520 }}>
          <Dialog.Title className="dialog-title">{t('teams.importOldPostsTitle', 'Import old posts')}</Dialog.Title>
          <Dialog.Description className="dialog-description" style={{ marginBottom: 16 }}>
            {t(
              'teams.importOldPostsHint',
              'Import posts from connected social accounts into goloom. They will be included in future AI profile analysis.',
            )}
          </Dialog.Description>

          {result !== null ? (
            <div className="stack stack--sm">
              <p style={{ color: 'var(--color-success, #16a34a)' }}>
                {t('teams.importOldPostsResult', '{{count}} posts imported.', { count: result })}
              </p>
              <div>
                <button type="button" className="btn btn--primary" onClick={handleClose}>
                  {t('common.done', 'Done')}
                </button>
              </div>
            </div>
          ) : (
            <div className="stack stack--sm">
              <div>
                <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                  <button type="button" className="btn btn--sm" onClick={selectAll}>
                    {t('teams.importOldPostsSelectAll', 'Select all')}
                  </button>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                  {accounts.map((account) => (
                    <label
                      key={account.id}
                      className="toggle-row"
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 6,
                        padding: '4px 10px',
                        borderRadius: 6,
                        border: selectedAccountIds.includes(account.id)
                          ? '1px solid var(--color-primary, #3b82f6)'
                          : '1px solid var(--color-border, #e5e7eb)',
                        cursor: 'pointer',
                        fontSize: 13,
                      }}
                    >
                      <input
                        type="checkbox"
                        checked={selectedAccountIds.includes(account.id)}
                        onChange={() => toggleAccount(account.id)}
                      />
                      <span>{account.name}</span>
                      <span style={{ opacity: 0.5, fontSize: 11 }}>({account.provider})</span>
                    </label>
                  ))}
                </div>
              </div>

              <div style={{ marginTop: 8 }}>
                <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                  <label style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 13, cursor: 'pointer' }}>
                    <input type="radio" name="import-mode" checked={mode === 'count'} onChange={() => setMode('count')} />
                    {t('teams.importOldPostsByCount', 'By count')}
                  </label>
                  <label style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 13, cursor: 'pointer' }}>
                    <input type="radio" name="import-mode" checked={mode === 'date'} onChange={() => setMode('date')} />
                    {t('teams.importOldPostsByDate', 'By date')}
                  </label>
                </div>

                {mode === 'count' ? (
                  <label className="field">
                    <span>{t('teams.importOldPostsCount', 'Number of posts')}</span>
                    <input
                      type="number"
                      min={1}
                      max={500}
                      value={postCount}
                      onChange={(e) => setPostCount(Number(e.target.value))}
                    />
                  </label>
                ) : (
                  <label className="field">
                    <span>{t('teams.importOldPostsUntilDate', 'Import until (inclusive)')}</span>
                    <input type="date" value={untilDate} onChange={(e) => setUntilDate(e.target.value)} />
                  </label>
                )}
              </div>

              {error && <p style={{ color: 'var(--color-danger, #dc2626)', fontSize: 13 }}>{error}</p>}

              <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                <button
                  type="button"
                  className="btn btn--primary"
                  onClick={() => void handleImport()}
                  disabled={loading || selectedAccountIds.length === 0}
                >
                  {loading ? t('teams.importOldPostsImporting', 'Importing...') : t('teams.importOldPostsStart', 'Import')}
                </button>
                <button type="button" className="btn" onClick={handleClose}>
                  {t('common.cancel', 'Cancel')}
                </button>
              </div>
            </div>
          )}

          <Dialog.Close asChild>
            <button className="dialog-close" aria-label={t('common.close', 'Close')}>
              <X size={16} />
            </button>
          </Dialog.Close>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
