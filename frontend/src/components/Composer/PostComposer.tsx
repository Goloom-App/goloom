import type { Dispatch, SetStateAction } from 'react'
import { useEffect, useMemo, useState } from 'react'
import type { BackendMediaItem, createApiClient } from '../../api'
import { Icon } from '../../icons'
import type { AccountRecord, TeamSchedulingPreferences } from '../../types'
import { ScheduleInsights } from './ScheduleInsights'
import { DestinationAvatar } from '../post/DestinationAvatar'
import { SocialPreview } from '../post/SocialPreview'
import type { SocialPreviewAttachment } from '../post/SocialPreview.types'
import { charCounterClass, pruneMediaExcludeAfterRemove } from './editorDraft'
import { ComposerMedia } from './ComposerMedia'
import type { EditorDraftState } from './types'

type Api = ReturnType<typeof createApiClient>

function attachmentsForDestination(
  draft: EditorDraftState,
  accountId: string,
  teamId: string | undefined | null,
  api: Api | null | undefined,
  meta: Record<string, Pick<BackendMediaItem, 'filename' | 'mime_type'>>,
): SocialPreviewAttachment[] {
  const ex = new Set(draft.mediaExcludeByAccount[accountId] ?? [])
  if (!teamId || !api) {
    return []
  }
  return draft.mediaIds
    .filter((id) => !ex.has(id))
    .map((id) => ({
      id,
      previewUrl: api.mediaPreviewUrl(teamId, id),
      mimeType: meta[id]?.mime_type ?? 'image/jpeg',
      filename: meta[id]?.filename,
    }))
}

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
  isMobile,
  theme,
  teamAccounts,
  draft,
  setDraft,
  syncing,
  onSave,
  onSaveDraft,
  onClose,
  onMediaUpload,
  teamId,
  api,
  authHeader,
  schedulingPreferences,
  standalone,
}: {
  open: boolean
  mode: 'create' | 'edit'
  isMobile: boolean
  theme: 'dark' | 'light'
  teamAccounts: AccountRecord[]
  draft: EditorDraftState
  setDraft: Dispatch<SetStateAction<EditorDraftState>>
  syncing: boolean
  onSave: () => void | Promise<void>
  onSaveDraft: () => void | Promise<void>
  onClose: () => void
  /** Upload to team media library (POST /teams/:id/media); returns library media id for scheduler JIT sync. */
  onMediaUpload?: (file: File) => Promise<string>
  teamId?: string | null
  api?: Api | null
  authHeader?: string
  schedulingPreferences?: TeamSchedulingPreferences | null
  /** When true, renders as a standalone page view instead of a modal overlay. */
  standalone?: boolean
}) {
  const [activeTab, setActiveTab] = useState<'default' | string>('default')
  const [mobilePanel, setMobilePanel] = useState<'edit' | 'preview'>('edit')
  const [libraryItems, setLibraryItems] = useState<BackendMediaItem[]>([])

  useEffect(() => {
    if (!open || !teamId || !api) {
      setLibraryItems([])
      return
    }
    let cancelled = false
    void api
      .listTeamMedia(teamId)
      .then((res) => {
        if (!cancelled) {
          setLibraryItems(res.items ?? [])
        }
      })
      .catch(() => {
        if (!cancelled) {
          setLibraryItems([])
        }
      })
    return () => {
      cancelled = true
    }
  }, [open, teamId, api])

  const selectedAccounts = useMemo(
    () => teamAccounts.filter((account) => draft.targetAccountIds.includes(account.id)),
    [draft.targetAccountIds, teamAccounts],
  )

  useEffect(() => {
    if (activeTab !== 'default' && !draft.targetAccountIds.includes(activeTab)) {
      setActiveTab('default')
    }
  }, [activeTab, draft.targetAccountIds])

  useEffect(() => {
    if (!isMobile) {
      setMobilePanel('edit')
    }
  }, [isMobile])

  const maxChars = useMemo(() => {
    if (activeTab === 'default') {
      return maxCharsForAccounts(selectedAccounts)
    }
    const acc = selectedAccounts.find((a) => a.id === activeTab)
    return acc ? acc.maxChars : 0
  }, [activeTab, selectedAccounts])

  const bodyLen = effectiveBody(draft, activeTab === 'default' ? null : activeTab).length

  const libraryById = useMemo(() => {
    const o: Record<string, Pick<BackendMediaItem, 'filename' | 'mime_type'>> = {}
    for (const row of libraryItems) {
      o[row.id] = { filename: row.filename, mime_type: row.mime_type }
    }
    for (const id of draft.mediaIds) {
      if (!o[id]) {
        o[id] = {
          filename: id.length > 28 ? `${id.slice(0, 14)}…${id.slice(-8)}` : id,
          mime_type: 'application/octet-stream',
        }
      }
    }
    return o
  }, [libraryItems, draft.mediaIds])

  const toggleDestinationMedia = (tabId: string, mediaId: string, wantAttached: boolean) => {
    setDraft((cur) => {
      const prev = cur.mediaExcludeByAccount[tabId] ?? []
      const next = wantAttached ? prev.filter((x) => x !== mediaId) : prev.includes(mediaId) ? prev : [...prev, mediaId]
      const mediaExcludeByAccount = { ...cur.mediaExcludeByAccount }
      if (next.length === 0) {
        delete mediaExcludeByAccount[tabId]
      } else {
        mediaExcludeByAccount[tabId] = next
      }
      return { ...cur, mediaExcludeByAccount }
    })
  }

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

  const inner = (
    <div className={`composer-container composer-container--enhanced ${isMobile ? 'composer-container--mobile' : ''}`} onClick={(event) => event.stopPropagation()}>
        <div className={`composer-main ${isMobile ? 'composer-main--mobile' : ''}`}>
          <header>
            <p className="eyebrow">Composer</p>
            <h2>{mode === 'edit' ? 'Edit post' : 'Create post'}</h2>
          </header>

          {isMobile ? (
            <div className="composer-mobile-tabs" role="tablist" aria-label="Composer mobile panel">
              <button
                type="button"
                role="tab"
                aria-selected={mobilePanel === 'edit'}
                className={`composer-mobile-tab ${mobilePanel === 'edit' ? 'composer-mobile-tab--active' : ''}`}
                onClick={() => setMobilePanel('edit')}
              >
                Edit
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={mobilePanel === 'preview'}
                className={`composer-mobile-tab ${mobilePanel === 'preview' ? 'composer-mobile-tab--active' : ''}`}
                onClick={() => setMobilePanel('preview')}
              >
                Preview
              </button>
            </div>
          ) : null}

          <div className={isMobile && mobilePanel === 'preview' ? 'composer-mobile-panel composer-mobile-panel--hidden' : 'composer-mobile-panel'}>
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
            libraryById={libraryById}
            onAddIds={(ids) =>
              setDraft((current) => {
                const merged = [...current.mediaIds]
                const seen = new Set(merged)
                for (const id of ids) {
                  if (!seen.has(id)) {
                    seen.add(id)
                    merged.push(id)
                  }
                }
                return { ...current, mediaIds: merged }
              })
            }
            onRemove={(id) =>
              setDraft((current) => ({
                ...current,
                mediaIds: current.mediaIds.filter((x) => x !== id),
                mediaExcludeByAccount: pruneMediaExcludeAfterRemove(current.mediaExcludeByAccount, id),
              }))
            }
            onUpload={onMediaUpload}
            teamId={teamId ?? undefined}
            api={api ?? undefined}
            authHeader={authHeader}
            uploadLabel={onMediaUpload ? undefined : 'Select a workspace to attach media files.'}
            disabled={syncing}
          />

          {activeTab !== 'default' && draft.mediaIds.length > 0 ? (
            <div className="composer-override-media">
              <p className="eyebrow">Media for this destination</p>
              <p className="hint">Uncheck to skip an attachment only for this account.</p>
              <ul className="composer-override-media__list">
                {draft.mediaIds.map((mid) => {
                  const excluded = (draft.mediaExcludeByAccount[activeTab] ?? []).includes(mid)
                  const meta = libraryById[mid]
                  return (
                    <li key={`${activeTab}-${mid}`} className="composer-override-media__row">
                      <label className="composer-override-media__label">
                        <input
                          type="checkbox"
                          checked={!excluded}
                          disabled={syncing}
                          onChange={(ev) => {
                            toggleDestinationMedia(activeTab, mid, ev.target.checked)
                          }}
                        />
                        <span className="composer-override-media__name" title={meta?.filename ?? mid}>
                          {meta?.filename ?? mid}
                        </span>
                      </label>
                    </li>
                  )
                })}
              </ul>
            </div>
          ) : null}

          <label className="field">
            <span>Scheduled at</span>
            <input
              type="datetime-local"
              value={draft.scheduledAt}
              onChange={(event) => setDraft((current) => ({ ...current, scheduledAt: event.target.value }))}
            />
          </label>

          {teamId && api ? (
            <ScheduleInsights
              teamId={teamId}
              api={api}
              scheduledAt={draft.scheduledAt}
              setScheduledAt={(v) => setDraft((c) => ({ ...c, scheduledAt: v }))}
              schedulingPreferences={schedulingPreferences ?? undefined}
            />
          ) : null}

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
        </div>

        <aside
          className={`composer-sidebar composer-sidebar--stack ${isMobile ? 'composer-sidebar--mobile' : ''} ${isMobile && mobilePanel !== 'preview' ? 'composer-mobile-panel--hidden' : ''}`}
        >
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
                  attachments={attachmentsForDestination(draft, account.id, teamId, api, libraryById)}
                  authHeader={authHeader}
                />
              ))
            ) : (
              <p className="hint">Select a destination to see previews.</p>
            )}
          </div>
        </aside>
      </div>
    )

  if (standalone) {
    return <div className="composer-page">{inner}</div>
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      {inner}
    </div>
  )
}
