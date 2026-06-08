import { useEffect, useMemo, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'

import type { AIJob, StyleMetadata, TeamProfile } from '../../types'

type ProposedProfile = {
  tonality?: string
  formatting_rules?: string[]
  banned_words?: string[]
  max_hashtags?: number
  preferred_language?: string
}

type SuggestedExample = {
  platform: string
  content: string
  notes?: string
  source_post_id?: string
}

interface ProfileAnalysisReviewDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  profile?: TeamProfile
  job?: AIJob
  onApply: (input: {
    profileFields: Partial<StyleMetadata>
    styleExamples: SuggestedExample[]
  }) => Promise<void>
}

function formatList(values: string[] | undefined) {
  if (!values || values.length === 0) return '—'
  return values.join(', ')
}

function listsEqual(a: string[] | undefined, b: string[] | undefined) {
  return JSON.stringify(a ?? []) === JSON.stringify(b ?? [])
}

export function ProfileAnalysisReviewDialog({
  open,
  onOpenChange,
  profile,
  job,
  onApply,
}: ProfileAnalysisReviewDialogProps) {
  const proposed = (job?.result?.proposed_profile ?? {}) as ProposedProfile
  const suggestedExamples = (job?.result?.suggested_style_examples ?? []) as SuggestedExample[]
  const current = profile?.styleMetadata

  const [applyTonality, setApplyTonality] = useState(true)
  const [applyFormattingRules, setApplyFormattingRules] = useState(true)
  const [applyBannedWords, setApplyBannedWords] = useState(true)
  const [applyMaxHashtags, setApplyMaxHashtags] = useState(true)
  const [applyLanguage, setApplyLanguage] = useState(true)
  const [selectedExamples, setSelectedExamples] = useState<Record<string, boolean>>({})
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setApplyTonality((proposed.tonality ?? '') !== (current?.tonality ?? ''))
    setApplyFormattingRules(!listsEqual(proposed.formatting_rules, current?.formattingRules))
    setApplyBannedWords(!listsEqual(proposed.banned_words, current?.bannedWords))
    setApplyMaxHashtags((proposed.max_hashtags ?? 0) !== (current?.maxHashtags ?? 0))
    setApplyLanguage((proposed.preferred_language ?? '') !== (current?.preferredLanguage ?? ''))
    const defaults: Record<string, boolean> = {}
    suggestedExamples.forEach((example, index) => {
      defaults[example.source_post_id || `idx-${index}`] = true
    })
    setSelectedExamples(defaults)
    setError(null)
  }, [open, job?.id, proposed, current, suggestedExamples])

  const rows = useMemo(
    () => [
      {
        key: 'tonality',
        label: 'Tonality',
        current: current?.tonality || '—',
        proposed: proposed.tonality || '—',
        apply: applyTonality,
        setApply: setApplyTonality,
        changed: (proposed.tonality ?? '') !== (current?.tonality ?? ''),
      },
      {
        key: 'formatting_rules',
        label: 'Formatting rules',
        current: formatList(current?.formattingRules),
        proposed: formatList(proposed.formatting_rules),
        apply: applyFormattingRules,
        setApply: setApplyFormattingRules,
        changed: !listsEqual(proposed.formatting_rules, current?.formattingRules),
      },
      {
        key: 'banned_words',
        label: 'Banned words',
        current: formatList(current?.bannedWords),
        proposed: formatList(proposed.banned_words),
        apply: applyBannedWords,
        setApply: setApplyBannedWords,
        changed: !listsEqual(proposed.banned_words, current?.bannedWords),
      },
      {
        key: 'max_hashtags',
        label: 'Max hashtags',
        current: String(current?.maxHashtags ?? '—'),
        proposed: String(proposed.max_hashtags ?? '—'),
        apply: applyMaxHashtags,
        setApply: setApplyMaxHashtags,
        changed: (proposed.max_hashtags ?? 0) !== (current?.maxHashtags ?? 0),
      },
      {
        key: 'preferred_language',
        label: 'Language',
        current: current?.preferredLanguage || '—',
        proposed: proposed.preferred_language || '—',
        apply: applyLanguage,
        setApply: setApplyLanguage,
        changed: (proposed.preferred_language ?? '') !== (current?.preferredLanguage ?? ''),
      },
    ],
    [
      applyBannedWords,
      applyFormattingRules,
      applyLanguage,
      applyMaxHashtags,
      applyTonality,
      current,
      proposed,
    ],
  )

  const handleApply = async () => {
    setSaving(true)
    setError(null)
    try {
      const profileFields: Partial<StyleMetadata> = {
        tonality: applyTonality ? (proposed.tonality ?? current?.tonality ?? '') : (current?.tonality ?? ''),
        formattingRules: applyFormattingRules
          ? (proposed.formatting_rules ?? current?.formattingRules ?? [])
          : (current?.formattingRules ?? []),
        bannedWords: applyBannedWords
          ? (proposed.banned_words ?? current?.bannedWords ?? [])
          : (current?.bannedWords ?? []),
        maxHashtags: applyMaxHashtags
          ? (proposed.max_hashtags ?? current?.maxHashtags ?? 3)
          : (current?.maxHashtags ?? 3),
        preferredLanguage: applyLanguage
          ? (proposed.preferred_language ?? current?.preferredLanguage ?? 'en')
          : (current?.preferredLanguage ?? 'en'),
      }
      const examples = suggestedExamples.filter((example, index) => {
        const key = example.source_post_id || `idx-${index}`
        return selectedExamples[key]
      })
      await onApply({ profileFields, styleExamples: examples })
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to apply analysis')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content" style={{ maxWidth: '760px' }} data-testid="profile-analysis-review">
          <div className="drawer-header">
            <Dialog.Title className="drawer-title">Review profile analysis</Dialog.Title>
            <Dialog.Close asChild>
              <button type="button" className="btn btn--ghost btn--icon-sm">
                <X size={20} />
              </button>
            </Dialog.Close>
          </div>
          <div className="drawer-body stack">
            <p className="hint">
              Compare the proposed profile with your current settings and choose what to apply.
            </p>
            {error && <div className="status-banner__error">{error}</div>}
            <div className="stack stack--sm">
              {rows.map((row) => (
                <div
                  key={row.key}
                  className="glass-panel glass-panel--compact stack stack--xs"
                  style={{ opacity: row.changed ? 1 : 0.75 }}
                >
                  <div className="flex-row--between" style={{ alignItems: 'center' }}>
                    <strong>{row.label}</strong>
                    <label className="flex-row--center gap-2" style={{ fontSize: '0.85rem' }}>
                      <input
                        type="checkbox"
                        checked={row.apply}
                        disabled={!row.changed}
                        onChange={(e) => row.setApply(e.target.checked)}
                      />
                      <span>Apply</span>
                    </label>
                  </div>
                  <div className="two-column-detail" style={{ gap: '0.75rem' }}>
                    <div>
                      <p className="hint" style={{ margin: 0, fontSize: '0.75rem' }}>Current</p>
                      <p style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{row.current}</p>
                    </div>
                    <div>
                      <p className="hint" style={{ margin: 0, fontSize: '0.75rem' }}>Proposed</p>
                      <p style={{ margin: 0, whiteSpace: 'pre-wrap' }}>{row.proposed}</p>
                    </div>
                  </div>
                </div>
              ))}
            </div>

            {suggestedExamples.length > 0 && (
              <div className="stack stack--sm">
                <h4 className="subsection-title">Suggested style examples</h4>
                {suggestedExamples.map((example, index) => {
                  const key = example.source_post_id || `idx-${index}`
                  return (
                    <label key={key} className="glass-panel glass-panel--compact stack stack--xs">
                      <div className="flex-row--between" style={{ alignItems: 'center' }}>
                        <span className="badge">{example.platform}</span>
                        <input
                          type="checkbox"
                          checked={Boolean(selectedExamples[key])}
                          onChange={(e) =>
                            setSelectedExamples((prev) => ({ ...prev, [key]: e.target.checked }))
                          }
                        />
                      </div>
                      <p style={{ margin: 0, whiteSpace: 'pre-wrap', fontSize: '0.9rem' }}>{example.content}</p>
                    </label>
                  )
                })}
              </div>
            )}

            <div className="flex-row--end gap-2">
              <Dialog.Close asChild>
                <button type="button" className="btn btn--ghost">
                  Dismiss
                </button>
              </Dialog.Close>
              <button type="button" className="btn btn--primary" onClick={() => void handleApply()} disabled={saving}>
                {saving ? 'Applying...' : 'Apply selected'}
              </button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
