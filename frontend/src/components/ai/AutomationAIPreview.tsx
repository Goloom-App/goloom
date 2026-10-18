import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { getApiClient } from '../../hooks/useAI'
import type { BackendAIJob } from '../../api'
import type { AccountRecord } from '../../types'
import { providerPostLength } from '../../postLength'

function sleep(ms: number, signal: AbortSignal) {
  return new Promise<void>((resolve) => {
    if (signal.aborted) return resolve()
    const onAbort = () => {
      window.clearTimeout(timer)
      signal.removeEventListener('abort', onAbort)
      resolve()
    }
    const timer = window.setTimeout(() => {
      signal.removeEventListener('abort', onAbort)
      resolve()
    }, ms)
    signal.addEventListener('abort', onAbort, { once: true })
  })
}

function asText(value: unknown): string {
  if (typeof value === 'string') return value
  if (value == null) return ''
  return JSON.stringify(value, null, 2)
}

export function AutomationAIPreview({
  teamId,
  accounts,
  requestPreview,
  testId,
  storageKey,
}: {
  teamId: string
  accounts: AccountRecord[]
  requestPreview: () => Promise<{ job_id: string }>
  testId: string
  storageKey?: string
}) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [job, setJob] = useState<BackendAIJob | null>(null)
  const mountedRef = useRef(true)
  const pollControllerRef = useRef<AbortController | null>(null)
  const key = `goloom.automationPreview.${teamId}.${storageKey ?? testId}`
  const [activeJobId, setActiveJobId] = useState(() => {
    try { return window.sessionStorage.getItem(key) ?? '' } catch { return '' }
  })

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      pollControllerRef.current?.abort()
    }
  }, [])

  const saveActiveJobId = (id: string) => {
    if (mountedRef.current) setActiveJobId(id)
    try {
      if (id) window.sessionStorage.setItem(key, id)
      else window.sessionStorage.removeItem(key)
    } catch {
      // The current component state still supports resume if storage is unavailable.
    }
  }

  const runPreview = async () => {
    if (busy) return
    setBusy(true)
    setError('')
    const resumeJobId = activeJobId
    const controller = new AbortController()
    pollControllerRef.current = controller
    if (!resumeJobId) setJob(null)
    try {
      const started = resumeJobId ? { job_id: resumeJobId } : await requestPreview()
      saveActiveJobId(started.job_id)
      if (!mountedRef.current || controller.signal.aborted) return
      let result: BackendAIJob | null = null
      const api = getApiClient()
      for (let attempt = 0; attempt < 90; attempt += 1) {
        result = await api.getAIJob(teamId, started.job_id)
        if (!mountedRef.current || controller.signal.aborted) return
        if (result.status === 'completed') {
          saveActiveJobId('')
          break
        }
        if (result.status === 'failed') {
          saveActiveJobId('')
          throw new Error(result.error_message || t('automationPreview.failed'))
        }
        await sleep(1000, controller.signal)
      }
      if (!mountedRef.current || controller.signal.aborted) return
      if (!result || result.status !== 'completed') throw new Error(t('automationPreview.timeout'))
      setJob(result)
    } catch (cause) {
      if (mountedRef.current && !controller.signal.aborted) {
        setError(cause instanceof Error ? cause.message : t('automationPreview.failed'))
      }
    } finally {
      if (mountedRef.current) setBusy(false)
      if (pollControllerRef.current === controller) pollControllerRef.current = null
    }
  }

  const payload = job?.payload ?? {}
  const snapshot = payload.preview && typeof payload.preview === 'object'
    ? payload.preview as Record<string, unknown>
    : {}
  const params = payload.params && typeof payload.params === 'object'
    ? payload.params as Record<string, unknown>
    : {}
  const result = job?.result ?? {}
  const overrides = result.account_content_override && typeof result.account_content_override === 'object'
    ? result.account_content_override as Record<string, string>
    : {}
  const baseContent = typeof result.content === 'string' ? result.content : ''
  const targetIds = Array.isArray(params.target_account_ids)
    ? params.target_account_ids.filter((id): id is string => typeof id === 'string')
    : accounts.map((account) => account.id)

  return (
    <section className="stack stack--sm" data-testid={testId}>
      <button type="button" className="btn btn--secondary btn--sm" onClick={() => void runPreview()} disabled={busy}>
        {busy ? t('automationPreview.generating') : activeJobId ? t('automationPreview.resume') : t('automationPreview.run')}
      </button>
      {error ? <p className="status-banner__error" role="alert">{error}</p> : null}
      {job ? (
        <div className="glass-panel glass-panel--compact stack stack--sm" data-testid={`${testId}-result`}>
          <h3>{t('automationPreview.result')}</h3>
          <div>
            <strong>{t('automationPreview.source')}</strong>
            <pre>{asText(snapshot.source)}</pre>
          </div>
          <div>
            <strong>{t('automationPreview.template')}</strong>
            <pre>{asText(snapshot.template)}</pre>
          </div>
          <div>
            <strong>{t('automationPreview.inputs')}</strong>
            <pre>{asText(snapshot.generation_inputs)}</pre>
          </div>
          {targetIds.map((id) => {
            const account = accounts.find((candidate) => candidate.id === id)
            const content = overrides[id] ?? baseContent
            return (
              <article key={id} data-testid={`${testId}-account-${id}`}>
                <strong>{account?.name ?? id}</strong>
                <p>{content}</p>
                <span className="hint">
                  {t('automationPreview.characters', { count: account ? providerPostLength(account.provider, content) : Array.from(content).length })}
                  {account ? ` / ${account.maxChars}` : ''}
                </span>
              </article>
            )
          })}
        </div>
      ) : null}
    </section>
  )
}
