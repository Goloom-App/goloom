import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { getApiClient, useCreateStyleExample } from '../../hooks/useAI'
import type { AccountRecord } from '../../types'

interface Props {
  teamId: string
  content: string
  accounts: AccountRecord[]
  overrides?: Record<string, string>
  postId?: string
  initialSource?: string
}

/** Explicit, isolated style-example mutation; never saves or approves a post. */
export function SaveStyleExample(props: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [saved, setSaved] = useState(false)
  return (
    <div onClick={event => event.stopPropagation()}>
      <button type="button" className="btn btn--secondary btn--sm" onClick={() => { setSaved(false); setOpen(true) }}>
        {t('styleExample.action')}
      </button>
      {saved ? <p role="status" className="hint">{t('styleExample.success')}</p> : null}
      <Dialog.Root open={open} onOpenChange={setOpen}>
        {open ? <SaveDialog {...props} onSaved={() => { setSaved(true); setOpen(false) }} /> : null}
      </Dialog.Root>
    </div>
  )
}

function SaveDialog({ teamId, content, accounts, overrides = {}, postId, initialSource, onSaved }: Props & { onSaved: () => void }) {
  const { t } = useTranslation()
  // Capture the visible editor state on explicit opening, not an AI job result.
  const [snapshot] = useState({ content, accounts, overrides })
  const [source, setSource] = useState(initialSource ?? '')
  const [platform, setPlatform] = useState(accounts.find(account => account.id === initialSource)?.provider ?? accounts[0]?.provider ?? 'mastodon')
  const [notes, setNotes] = useState('')
  const createExample = useCreateStyleExample()
  const versions = useQuery({
    queryKey: ['team', teamId, 'style-example-versions', postId],
    queryFn: () => getApiClient().listPostVersions(teamId, postId!),
    enabled: Boolean(postId),
    staleTime: 0,
  })
  const accountVersions = postId
    ? Object.fromEntries((versions.data?.items ?? []).map(version => [version.account_id, version.content]))
    : snapshot.overrides
  const text = source && Object.hasOwn(accountVersions, source) ? accountVersions[source] : snapshot.content
  const loading = Boolean(postId) && versions.isFetching
  const failed = Boolean(postId) && versions.isError

  return (
    <Dialog.Portal>
      <Dialog.Overlay className="dialog-overlay" />
      <Dialog.Content className="dialog-content" style={{ maxWidth: 600 }}>
        <Dialog.Title className="drawer-title">{t('styleExample.action')}</Dialog.Title>
        <Dialog.Description className="hint">{t('styleExample.description')}</Dialog.Description>
        <form className="stack" onSubmit={event => {
          event.preventDefault()
          if (loading || failed || createExample.isPending || createExample.isSuccess) return
          createExample.mutate({ teamId, data: { platform, content: text, notes } }, { onSuccess: onSaved })
        }}>
          <label className="field">
            <span>{t('styleExample.source')}</span>
            <select value={source} disabled={createExample.isPending} onChange={event => {
              setSource(event.target.value)
              const account = snapshot.accounts.find(account => account.id === event.target.value)
              if (account) setPlatform(account.provider)
            }}>
              <option value="">{t('styleExample.shared')}</option>
              {snapshot.accounts.map(account => <option key={account.id} value={account.id}>{account.name} ({account.provider})</option>)}
            </select>
          </label>
          <label className="field">
            <span>{t('styleExample.text')}</span>
            <textarea rows={8} value={loading || failed ? '' : text} readOnly />
          </label>
          <label className="field">
            <span>{t('styleExample.platform')}</span>
            <select value={platform} disabled={createExample.isPending} onChange={event => setPlatform(event.target.value as typeof platform)}>
              <option value="mastodon">Mastodon</option>
              <option value="bluesky">Bluesky</option>
              <option value="friendica">Friendica</option>
              <option value="pixelfed">Pixelfed</option>
            </select>
          </label>
          <label className="field">
            <span>{t('styleExample.note')}</span>
            <textarea rows={2} maxLength={1000} value={notes} disabled={createExample.isPending} onChange={event => setNotes(event.target.value)} />
          </label>
          {loading ? <p role="status" className="hint">{t('common.loading')}</p> : null}
          {failed ? <p role="alert" className="status-banner__error">{t('styleExample.loadFailed')}</p> : null}
          {createExample.isError ? <p role="alert" className="status-banner__error">{createExample.error.message}</p> : null}
          <div className="flex-row--center gap-2">
            <Dialog.Close asChild><button type="button" className="btn btn--ghost" disabled={createExample.isPending}>{t('common.cancel')}</button></Dialog.Close>
            <button type="submit" className="btn btn--primary" disabled={loading || failed || !text.trim() || Array.from(text).length > 5000 || createExample.isPending || createExample.isSuccess}>
              {t('styleExample.save')}
            </button>
          </div>
        </form>
      </Dialog.Content>
    </Dialog.Portal>
  )
}
