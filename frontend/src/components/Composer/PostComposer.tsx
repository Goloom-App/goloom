import type { Dispatch, SetStateAction } from 'react'
import { useEffect, useMemo, useState } from 'react'
import { Icon } from '../../icons'
import type { AccountRecord } from '../../types'
import { DestinationAvatar } from '../post/DestinationAvatar'
import { SocialPreview } from '../post/SocialPreview'
import { charCounterClass } from './editorDraft'
import { ComposerMedia } from './ComposerMedia'
import type { EditorDraftState } from './types'

function effectiveBody(draft: EditorDraftState, accountId: string | null) {
  if (!accountId || accountId === 'default') {
    return draft.content
  }
  return draft.accountContentOverride[accountId] ?? draft.content
}

function maxCharsForAccounts(accounts: AccountRecord[]) {
  if (accounts.length === 0) {
    return 0
  }
  return accounts.reduce((lowest, account) => Math.min(lowest, account.maxChars), accounts[0]!.maxChars)
}

export function PostComposer({
  open,
  mode,
  theme,
  teamAccounts,
  draft,
  setDraft,
  syncing,
  onSave,
  onSaveDraft,
  onClose,
  onMediaUpload,
}: {
  open: boolean
  mode: 'create' | 'edit'
  theme: 'dark' | 'light'
  teamAccounts: AccountRecord[]
  draft: EditorDraftState
  setDraft: Dispatch<SetStateAction<EditorDraftState>>
  syncing: boolean
  onSave: () => void | Promise<void>
  onSaveDraft: () => void | Promise<void>
  onClose: () => void
  /** Upload via POST /teams/:id/media/upload; returns provider media id. */
  onMediaUpload?: (file: File) => Promise<string>
}) {
  const [activeTab, setActiveTab] = useState<'default' | string>('default')

  const selectedAccounts = useMemo(
    () => teamAccounts.filter((account) => draft.targetAccountIds.includes(account.id)),
    [draft.targetAccountIds, teamAccounts],
  )

  useEffect(() => {
    if (activeTab !== 'default' && !draft.targetAccountIds.includes(activeTab)) {
      setActiveTab('default')
    }
  }, [activeTab, draft.targetAccountIds])

  const maxChars = useMemo(() => {
    if (activeTab === 'default') {
      return maxCharsForAccounts(selectedAccounts)
    }
    const acc = selectedAccounts.find((a) => a.id === activeTab)
    return acc ? acc.maxChars : 0
  }, [activeTab, selectedAccounts])

  const bodyLen = effectiveBody(draft, activeTab === 'default' ? null : activeTab).length

  const overAnyLimit = useMemo(() => {
    if (draft.targetAccountIds.length === 0) {
      return true
    }
    for (const id of draft.targetAccountIds) {
      const acc = teamAccounts.find((a) => a.id === id)
      if (!acc) {
        continue
      }
      const body = effectiveBody(draft, id)
      if (acc.maxChars > 0 && body.length > acc.maxChars) {
        return true
      }
    }
    return false
  }, [draft, teamAccounts])

  if (!open) {
    return null
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="composer-container composer-container--enhanced" onClick={(event) => event.stopPropagation()}>
        <div className="composer-main">
          <header>
            <p className="eyebrow">Composer</p>
            <h2>{mode === 'edit' ? 'Edit post' : 'Create post'}</h2>
          </header>

          <label className="field">
            <span>Title</span>
            <input
              value={draft.title}
              onChange={(event) => setDraft((current) => ({ ...current, title: event.target.value }))}
              placeholder="Post title for internal reference"
            />
          </label>

          <div className="composer-tabs" role="tablist" aria-label="Content scope">
            <button
              type="button"
              role="tab"
              aria-selected={activeTab === 'default'}
              className={`composer-tab ${activeTab === 'default' ? 'composer-tab--active' : ''}`}
              onClick={() => setActiveTab('default')}
            >
              Default
            </button>
            {selectedAccounts.map((account) => (
              <button
                key={account.id}
                type="button"
                role="tab"
                aria-selected={activeTab === account.id}
                className={`composer-tab ${activeTab === account.id ? 'composer-tab--active' : ''}`}
                onClick={() => setActiveTab(account.id)}
                title={account.name}
              >
                <DestinationAvatar account={account} compact />
                <span className="composer-tab__label">{account.username.replace(/^@/, '').slice(0, 12)}</span>
              </button>
            ))}
          </div>

          <label className="field">
            <span>{activeTab === 'default' ? 'Message (all destinations)' : `Override for ${selectedAccounts.find((a) => a.id === activeTab)?.name ?? 'account'}`}</span>
            <textarea
              rows={8}
              value={effectiveBody(draft, activeTab === 'default' ? null : activeTab)}
              onChange={(event) => {
                const v = event.target.value
                if (activeTab === 'default') {
                  setDraft((current) => ({ ...current, content: v }))
                } else {
                  setDraft((current) => ({
                    ...current,
                    accountContentOverride: { ...current.accountContentOverride, [activeTab]: v },
                  }))
                }
              }}
              placeholder={activeTab === 'default' ? "What's on your mind?" : 'Leave aligned with default, or type a custom version for this account.'}
            />
          </label>

          <div className={`char-counter ${charCounterClass(bodyLen, maxChars)}`}>
            <strong>{bodyLen}</strong>
            <span>/ {maxChars || '—'}</span>
          </div>

          <ComposerMedia
            mediaIds={draft.mediaIds}
            onAdd={(id) =>
              setDraft((current) =>
                current.mediaIds.includes(id) ? current : { ...current, mediaIds: [...current.mediaIds, id] },
              )
            }
            onRemove={(id) => setDraft((current) => ({ ...current, mediaIds: current.mediaIds.filter((x) => x !== id) }))}
            onUpload={onMediaUpload}
            uploadLabel={teamAccounts.length === 0 ? 'Add a social account to this team to upload media.' : undefined}
            disabled={syncing}
          />

          <label className="field">
            <span>Scheduled at</span>
            <input
              type="datetime-local"
              value={draft.scheduledAt}
              onChange={(event) => setDraft((current) => ({ ...current, scheduledAt: event.target.value }))}
            />
          </label>

          <footer className="composer-footer-actions">
            <button
              type="button"
              className="button button--primary"
              disabled={syncing || draft.targetAccountIds.length === 0 || overAnyLimit}
              onClick={() => void onSave()}
            >
              <Icon name="calendar" className="inline-icon" />
              <span>{mode === 'edit' ? 'Save changes' : 'Schedule post'}</span>
            </button>
            <button type="button" className="button button--secondary" disabled={syncing} onClick={() => void onSaveDraft()}>
              Save draft
            </button>
            <button type="button" className="button button--secondary" onClick={onClose}>
              Cancel
            </button>
          </footer>
        </div>

        <aside className="composer-sidebar composer-sidebar--stack">
          <p className="eyebrow">Destinations</p>
          <div className="composer-destination-row" role="group" aria-label="Post destinations">
            {teamAccounts.map((account) => {
              const selected = draft.targetAccountIds.includes(account.id)
              return (
                <button
                  key={account.id}
                  type="button"
                  className={`composer-destination-toggle ${selected ? 'composer-destination-toggle--selected' : ''}`}
                  aria-pressed={selected}
                  title={`${account.name} · ${account.provider}`}
                  onClick={() =>
                    setDraft((current) => {
                      const has = current.targetAccountIds.includes(account.id)
                      return {
                        ...current,
                        targetAccountIds: has ? current.targetAccountIds.filter((id) => id !== account.id) : [...current.targetAccountIds, account.id],
                      }
                    })
                  }
                >
                  <DestinationAvatar account={account} />
                </button>
              )
            })}
          </div>
          {teamAccounts.length === 0 ? <p className="hint">No accounts for this workspace.</p> : null}

          <div className="divider" />

          <p className="eyebrow">Live previews</p>
          <div className="composer-preview-stack">
            {selectedAccounts.length > 0 ? (
              selectedAccounts.map((account) => (
                <SocialPreview
                  key={account.id}
                  account={account}
                  content={effectiveBody(draft, account.id)}
                  scheduledAt={draft.scheduledAt}
                  theme={theme}
                />
              ))
            ) : (
              <p className="hint">Select a destination to see previews.</p>
            )}
          </div>
        </aside>
      </div>
    </div>
  )
}
