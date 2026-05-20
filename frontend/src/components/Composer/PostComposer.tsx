import type { Dispatch, SetStateAction } from 'react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { createPortal } from 'react-dom'
import type { BackendMediaItem, createApiClient } from '../../api'
import { Icon } from '../../icons'
import type { AccountRecord, TeamSchedulingPreferences } from '../../types'
import { ScheduleInsights } from './ScheduleInsights'
import { DestinationAvatar } from '../post/DestinationAvatar'
import { charCounterClass, pruneMediaExcludeAfterRemove } from './editorDraft'
import { ComposerMedia } from './ComposerMedia'
import { ComposerPreviews } from './ComposerPreviews'
import { effectiveBody } from './composerUtils'
import type { EditorDraftState } from './types'

type Api = ReturnType<typeof createApiClient>

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
  previewColumnExternal,
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
  /** When true (standalone desktop), the preview column is rendered externally by the parent */
  previewColumnExternal?: boolean
}) {
  const { t } = useTranslation()
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

  const accountLimitStatus = useMemo(() => {
    const status: Record<string, { len: number; max: number; over: boolean }> = {}
    for (const id of draft.targetAccountIds) {
      const acc = teamAccounts.find((a) => a.id === id)
      if (!acc) continue
      const body = effectiveBody(draft, id)
      status[id] = {
        len: body.length,
        max: acc.maxChars,
        over: acc.maxChars > 0 && body.length > acc.maxChars,
      }
    }
    return status
  }, [draft, teamAccounts])

  const minMaxChars = useMemo(() => maxCharsForAccounts(selectedAccounts), [selectedAccounts])

  if (!open) {
    return null
  }

  const onSaveInternal = async () => {
    if (syncing || !onSave) return
    try {
      const payload = {
        title: draft.title,
        content: draft.content,
        scheduled_at: new Date(draft.scheduledAt).toISOString(),
        target_accounts: draft.targetAccountIds,
        media_ids: draft.mediaIds,
        media_exclude_by_account: draft.mediaExcludeByAccount,
        account_content_override: draft.accountContentOverride,
        draft: false,
      }
      const val = await api!.validatePost(teamId!, payload)
      if (!val.valid) {
        // TODO: UI feedback for validation failure
        return
      }
      // When saving/scheduling, we always want to move out of draft
      setDraft(prev => ({ ...prev, status: 'scheduled' }))
      await onSave()
    } catch (err) {
      console.error('Failed to save post', err)
    }
  }

  const toggleDestination = (accountId: string) => {
    setDraft((current) => {
      const has = current.targetAccountIds.includes(accountId)
      return {
        ...current,
        targetAccountIds: has
          ? current.targetAccountIds.filter((id) => id !== accountId)
          : [...current.targetAccountIds, accountId],
      }
    })
  }

  const destinationsPanel = isMobile ? (
    <section className="composer-destinations-mobile" aria-label={t('composer.postDestinationsAria')}>
      <p className="eyebrow">{t('eyebrow.destinations')}</p>
      <p className="hint composer-destinations-mobile__hint">{t('composer.destinationsHint')}</p>
      {teamAccounts.length === 0 ? (
        <p className="hint">{t('composer.noAccountsWorkspace')}</p>
      ) : (
        <div className="composer-destination-row composer-destination-row--mobile" role="group">
          {teamAccounts.map((account) => {
            const selected = draft.targetAccountIds.includes(account.id)
            return (
              <button
                key={account.id}
                type="button"
                data-testid="composer-destination-toggle"
                className={`composer-destination-mobile ${selected ? 'composer-destination-mobile--selected' : ''}`}
                aria-pressed={selected}
                title={`${account.name} · ${account.provider}`}
                onClick={() => toggleDestination(account.id)}
              >
                <DestinationAvatar account={account} />
                <span className="composer-destination-mobile__label">
                  {account.username.replace(/^@/, '').slice(0, 14)}
                </span>
              </button>
            )
          })}
        </div>
      )}
    </section>
  ) : (
    <aside className="composer-sidebar composer-sidebar--destinations">
      <p className="eyebrow">{t('eyebrow.destinations')}</p>
      <div className="composer-destination-row" role="group" aria-label={t('composer.postDestinationsAria')}>
        {teamAccounts.map((account) => {
          const selected = draft.targetAccountIds.includes(account.id)
          return (
            <button
              key={account.id}
              type="button"
              className={`composer-destination-toggle ${selected ? 'composer-destination-toggle--selected' : ''}`}
              aria-pressed={selected}
              title={`${account.name} · ${account.provider}`}
              onClick={() => toggleDestination(account.id)}
            >
              <DestinationAvatar account={account} />
            </button>
          )
        })}
      </div>
      {teamAccounts.length === 0 ? <p className="hint">{t('composer.noAccountsWorkspace')}</p> : null}
    </aside>
  )

  const previewsPanel = !previewColumnExternal || isMobile ? (
    <aside
      className={`composer-sidebar composer-sidebar--previews ${isMobile ? 'composer-previews-mobile' : ''} ${isMobile && mobilePanel !== 'preview' ? 'composer-mobile-panel--hidden' : ''}`}
    >
      <ComposerPreviews
        draft={draft}
        teamAccounts={teamAccounts}
        teamId={teamId}
        api={api}
        authHeader={authHeader}
        theme={theme}
        libraryItems={libraryItems}
      />
    </aside>
  ) : null

  const composerHeader = (
    <header>
      <p className="eyebrow">{t('eyebrow.composer')}</p>
      <h2 data-testid="composer-title">{mode === 'edit' ? t('composer.editPost') : t('composer.createPost')}</h2>
    </header>
  )

  const editingContent = (
    <>
      <label className="field">
            <span>{t('composer.title')}</span>
            <input
              value={draft.title}
              onChange={(event) => setDraft((current) => ({ ...current, title: event.target.value }))}
              placeholder={t('composer.titlePlaceholder')}
            />
          </label>

          {isMobile && selectedAccounts.length === 0 ? (
            <p className="hint composer-tabs__hint">{t('composer.selectDestinationOverrides')}</p>
          ) : null}

          <div className={`composer-tabs ${isMobile ? 'composer-tabs--mobile' : ''}`} role="tablist" aria-label={t('composer.contentScopeAria')}>
            <button
              type="button"
              role="tab"
              aria-selected={activeTab === 'default'}
              className={`composer-tab ${activeTab === 'default' ? 'composer-tab--active' : ''}`}
              onClick={() => setActiveTab('default')}
            >
              {t('common.default')}
            </button>
            {selectedAccounts.map((account) => {
              const status = accountLimitStatus[account.id]
              return (
                <button
                  key={account.id}
                  type="button"
                  role="tab"
                  aria-selected={activeTab === account.id}
                  className={`composer-tab ${activeTab === account.id ? 'composer-tab--active' : ''} ${status?.over ? 'composer-tab--error' : ''}`}
                  onClick={() => setActiveTab(account.id)}
                  title={status?.over ? t('composer.exceedsLimit', { name: account.name, len: status.len, max: status.max }) : account.name}
                >
                  <DestinationAvatar account={account} compact />
                  <span className="composer-tab__label">{account.username.replace(/^@/, '').slice(0, 12)}</span>
                </button>
              )
            })}
          </div>

          <label className="field">
            <div className="flex-row--between">
              <span>{activeTab === 'default' ? t('composer.messageAll') : t('composer.overrideFor', { name: selectedAccounts.find((a) => a.id === activeTab)?.name ?? 'account' })}</span>
              {activeTab === 'default' && minMaxChars > 0 && (
                <span className="hint" style={{ fontSize: '0.7rem' }}>{t('composer.aimForMaxChars', { count: minMaxChars })}</span>
              )}
            </div>
            <textarea
              rows={8}
              className={activeTab !== 'default' && accountLimitStatus[activeTab]?.over ? 'input--danger' : ''}
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
              placeholder={activeTab === 'default' ? t('composer.placeholderDefault') : t('composer.placeholderOverride')}
            />
          </label>

          <div className={`char-counter ${charCounterClass(bodyLen, maxChars)}`}>
            <strong>{bodyLen}</strong>
            <span>/ {maxChars || t('common.emDash')}</span>
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
            uploadLabel={onMediaUpload ? undefined : t('media.selectWorkspaceMedia')}
            disabled={syncing}
          />

          {activeTab !== 'default' && draft.mediaIds.length > 0 ? (
            <div className="composer-override-media">
              <p className="eyebrow">{t('composer.mediaForDestination')}</p>
              <p className="hint">{t('composer.skipAttachmentHint')}</p>
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
            <span>{t('composer.scheduledAt')}</span>
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
              onClick={() => void onSaveInternal()}
            >
              <Icon name="calendar" className="inline-icon" />
              <span>{mode === 'edit' ? t('composer.saveChanges') : t('composer.schedulePost')}</span>
            </button>
            <button type="button" className="button button--secondary" disabled={syncing} onClick={() => void onSaveDraft()}>
              {t('composer.saveDraft')}
            </button>
            <button type="button" className="button button--secondary" onClick={onClose}>
              {t('common.cancel')}
            </button>
          </footer>
        </>
      )

  const inner = (
    <div className={`composer-container composer-container--enhanced ${previewsPanel ? 'composer-container--three-col' : 'composer-container--two-col'}`} onClick={(event) => event.stopPropagation()}>
      {destinationsPanel}
      <div className="composer-main">
        {composerHeader}
        {editingContent}
      </div>
      {previewsPanel}
    </div>
  )

  if (isMobile) {
    const mobileComposer = (
      <div className="composer-overlay" data-testid="composer-overlay" onClick={(event) => event.stopPropagation()}>
        <div className="composer-container composer-container--enhanced composer-container--mobile">
          <div className="composer-main composer-main--mobile">
          <header className="composer-mobile-header">
            <button type="button" className="btn btn--ghost btn--xs" onClick={onClose} aria-label={t('composer.closeComposer')}>
              <Icon name="close" className="inline-icon" />
            </button>
            <div>
              <p className="eyebrow">{t('eyebrow.composer')}</p>
              <h2 data-testid="composer-title">{mode === 'edit' ? t('composer.editPost') : t('composer.createPost')}</h2>
            </div>
          </header>
          <div className="composer-mobile-tabs" role="tablist" aria-label={t('composer.mobilePanelAria')}>
            <button type="button" role="tab" aria-selected={mobilePanel === 'edit'} className={`composer-mobile-tab ${mobilePanel === 'edit' ? 'composer-mobile-tab--active' : ''}`} onClick={() => setMobilePanel('edit')}>{t('composer.mobileEdit')}</button>
            <button type="button" role="tab" aria-selected={mobilePanel === 'preview'} className={`composer-mobile-tab ${mobilePanel === 'preview' ? 'composer-mobile-tab--active' : ''}`} onClick={() => setMobilePanel('preview')}>{t('composer.mobilePreview')}</button>
          </div>
          {mobilePanel === 'edit' ? (
            <div className="composer-mobile-edit-scroll">
              {destinationsPanel}
              {editingContent}
            </div>
          ) : (
            previewsPanel
          )}
          </div>
        </div>
      </div>
    )
    return createPortal(mobileComposer, document.body)
  }

  if (standalone) {
    return (
      <div className="composer-page" data-testid="composer-view">
        {inner}
      </div>
    )
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      {inner}
    </div>
  )
}
