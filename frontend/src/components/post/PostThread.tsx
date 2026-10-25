import { useId, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { formatDistanceToNow, isValid, parseISO } from 'date-fns'
import { de, enUS } from 'date-fns/locale'

import { ApiError, type BackendPostThread, type BackendThreadReply } from '../../api'
import type { AccountRecord } from '../../types'

const THREAD_PROVIDERS = new Set(['mastodon', 'friendica'])
const MAX_INDENT_DEPTH = 4

interface ReplyNode {
  reply: BackendThreadReply
  children: ReplyNode[]
}

/** Nest replies by parent_id; unknown parents (incl. the root post) become top-level. */
function buildReplyTree(replies: BackendThreadReply[]): ReplyNode[] {
  const nodes = new Map<string, ReplyNode>()
  for (const reply of replies) nodes.set(reply.remote_id, { reply, children: [] })
  const roots: ReplyNode[] = []
  for (const reply of replies) {
    const node = nodes.get(reply.remote_id)!
    const parent = reply.parent_id && reply.parent_id !== reply.remote_id ? nodes.get(reply.parent_id) : undefined
    if (parent) parent.children.push(node)
    else roots.push(node)
  }
  return roots
}

function Avatar({ name, url }: { name: string; url?: string }) {
  const { t } = useTranslation()
  const [failed, setFailed] = useState(false)
  const initials = name.replace('@', '').trim().slice(0, 2).toUpperCase() || '?'
  return (
    <span className="post-thread__avatar" aria-hidden={!url || failed}>
      {url && !failed ? (
        <img src={url} alt={t('postThread.avatarAlt', { name })} loading="lazy" referrerPolicy="no-referrer" onError={() => setFailed(true)} />
      ) : (
        initials
      )}
    </span>
  )
}

function ReplyItem({ node, depth, locale }: { node: ReplyNode; depth: number; locale: typeof de }) {
  const { t } = useTranslation()
  const { reply } = node
  const when = parseISO(reply.published_at)
  const name = reply.author.display_name || reply.author.handle
  return (
    <li className="post-thread__item" data-testid="thread-reply">
      <div className="post-thread__reply">
        <Avatar name={name} url={reply.author.avatar_url} />
        <div className="post-thread__main">
          <div className="post-thread__meta">
            <span className="post-thread__name">{name}</span>
            <span className="post-thread__handle">{reply.author.handle}</span>
            {isValid(when) ? (
              <time dateTime={reply.published_at} title={when.toLocaleString()}>
                {formatDistanceToNow(when, { addSuffix: true, locale })}
              </time>
            ) : null}
            {reply.url ? (
              <a href={reply.url} target="_blank" rel="noopener noreferrer" aria-label={`${t('postThread.openReply')}: ${name}`}>
                {t('postThread.openReply')}
              </a>
            ) : null}
          </div>
          <p className="post-thread__content">{reply.content}</p>
        </div>
      </div>
      {node.children.length > 0 ? (
        <ul className={`post-thread__list${depth + 1 < MAX_INDENT_DEPTH ? ' post-thread__list--nested' : ''}`}>
          {node.children.map((child) => (
            <ReplyItem key={child.reply.remote_id} node={child} depth={depth + 1} locale={locale} />
          ))}
        </ul>
      ) : null}
    </li>
  )
}

/** Lazy, read-only replies view for one published target. Fetches only once expanded. */
export function PostThread({
  teamId,
  postId,
  account,
  publishedUrl,
  api,
}: {
  teamId: string
  postId: string
  account: AccountRecord
  publishedUrl?: string
  api: { getPostThread: (teamId: string, postId: string, accountId: string) => Promise<BackendPostThread> } | null | undefined
}) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const panelId = useId()
  const enabled = open && Boolean(api)
  const query = useQuery({
    queryKey: ['post-thread', teamId, postId, account.id],
    queryFn: () => api!.getPostThread(teamId, postId, account.id),
    enabled,
    staleTime: 60_000,
    retry: false,
    refetchOnWindowFocus: false,
  })
  const tree = useMemo(() => buildReplyTree(query.data?.replies ?? []), [query.data])

  if (!publishedUrl || !THREAD_PROVIDERS.has(account.provider)) return null

  const unsupported = query.error instanceof ApiError && query.error.status === 422
  const locale = i18n.language === 'de' ? de : enUS

  return (
    <div className="post-thread" data-testid="post-thread">
      <div className="post-thread__bar">
        <button
          type="button"
          className="btn btn--ghost"
          aria-expanded={open}
          aria-controls={panelId}
          onClick={() => setOpen((v) => !v)}
        >
          {open ? t('postThread.hide') : t('postThread.show')}
        </button>
        {open && !unsupported ? (
          <button
            type="button"
            className="btn btn--ghost"
            onClick={() => void query.refetch()}
            disabled={query.isFetching}
          >
            {t('postThread.refresh')}
          </button>
        ) : null}
      </div>
      <div id={panelId} role="region" aria-label={t('postThread.region')} hidden={!open}>
        {open ? (
          query.isPending ? (
            <p className="hint" role="status">{t('postThread.loading')}</p>
          ) : unsupported ? (
            <p className="hint" data-testid="thread-unsupported">{t('postThread.unsupported')}</p>
          ) : query.isError ? (
            <p className="hint post-thread__error" role="alert" data-testid="thread-error">{t('postThread.error')}</p>
          ) : tree.length === 0 ? (
            <p className="hint" data-testid="thread-empty">{t('postThread.empty')}</p>
          ) : (
            <ul className="post-thread__list">
              {tree.map((node) => (
                <ReplyItem key={node.reply.remote_id} node={node} depth={0} locale={locale} />
              ))}
            </ul>
          )
        ) : null}
      </div>
    </div>
  )
}
