import { useState, useEffect } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X, Plus, Trash2 } from 'lucide-react'

import {
  useTeamProfile,
  useUpsertTeamProfile,
  useStyleExamples,
  useCreateStyleExample,
  useDeleteStyleExample,
} from '../../hooks/useAI'
import type { TeamRecord } from '../../types'

interface TeamProfileViewProps {
  team: TeamRecord
}

export function TeamProfileView({ team }: TeamProfileViewProps) {
  const { data: profile, isLoading: profileLoading } = useTeamProfile(team.id)
  const { data: styleExamples, isLoading: examplesLoading } = useStyleExamples(team.id)
  const upsertProfile = useUpsertTeamProfile()
  const createExample = useCreateStyleExample()
  const deleteExample = useDeleteStyleExample()

  const [tonality, setTonality] = useState('')
  const [formattingRules, setFormattingRules] = useState<string[]>([])
  const [newRule, setNewRule] = useState('')
  const [bannedWords, setBannedWords] = useState<string[]>([])
  const [newWord, setNewWord] = useState('')
  const [maxHashtags, setMaxHashtags] = useState(3)
  const [preferredLanguage, setPreferredLanguage] = useState('en')
  const [autoPublishEnabled, setAutoPublishEnabled] = useState(false)

  const [isExampleDialogOpen, setIsExampleDialogOpen] = useState(false)
  const [examplePlatform, setExamplePlatform] = useState('mastodon')
  const [exampleContent, setExampleContent] = useState('')
  const [exampleNotes, setExampleNotes] = useState('')

  const [statusMessage, setStatusMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (profile) {
      setTonality(profile.styleMetadata.tonality || '')
      setFormattingRules(profile.styleMetadata.formattingRules || [])
      setBannedWords(profile.styleMetadata.bannedWords || [])
      setMaxHashtags(profile.styleMetadata.maxHashtags ?? 3)
      setPreferredLanguage(profile.styleMetadata.preferredLanguage || 'en')
      setAutoPublishEnabled(profile.autoPublishEnabled || false)
    }
  }, [profile])

  if (!team.isAiEnabled) {
    return (
      <div className="empty-state">
        <p className="hint">AI features are not enabled for this team.</p>
      </div>
    )
  }

  const handleSaveProfile = async () => {
    setError(null)
    setStatusMessage(null)
    try {
      await upsertProfile.mutateAsync({
        teamId: team.id,
        data: {
          style_metadata: {
            tonality,
            formatting_rules: formattingRules,
            banned_words: bannedWords,
            max_hashtags: maxHashtags,
            preferred_language: preferredLanguage,
          },
          auto_publish_enabled: autoPublishEnabled,
        },
      })
      setStatusMessage('Profile saved successfully')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save profile')
    }
  }

  const handleAddExample = async () => {
    if (!exampleContent.trim()) return
    setError(null)
    setStatusMessage(null)
    try {
      await createExample.mutateAsync({
        teamId: team.id,
        data: {
          platform: examplePlatform,
          content: exampleContent,
          notes: exampleNotes,
        },
      })
      setIsExampleDialogOpen(false)
      setExampleContent('')
      setExampleNotes('')
      setStatusMessage('Style example added')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add example')
    }
  }

  const handleDeleteExample = async (exampleId: string) => {
    if (!window.confirm('Are you sure you want to delete this example?')) return
    setError(null)
    setStatusMessage(null)
    try {
      await deleteExample.mutateAsync({ teamId: team.id, exampleId })
      setStatusMessage('Style example deleted')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete example')
    }
  }

  if (profileLoading || examplesLoading) {
    return <p className="hint">Loading AI profile...</p>
  }

  return (
    <div className="two-column-detail" data-testid="team-profile-view">
      <div className="glass-panel stack">
        <h2 className="section-card__title">AI Team Profile</h2>
        <p className="hint">Configure how the AI should write posts for this team.</p>

        {(error || statusMessage) && (
          <div className="status-banner-panel" style={{ padding: '1rem', marginBottom: '1rem' }}>
            {statusMessage && <span className="status-banner__success" data-testid="profile-status-success">{statusMessage}</span>}
            {error && <span className="status-banner__error" data-testid="profile-status-error">{error}</span>}
          </div>
        )}

        <label className="field">
          <span>Tonality</span>
          <input
            data-testid="profile-tonality"
            value={tonality}
            onChange={(e) => setTonality(e.target.value)}
            placeholder="e.g., Professional, casual, humorous"
          />
        </label>

        <label className="field">
          <span>Preferred Language</span>
          <select data-testid="profile-language" value={preferredLanguage} onChange={(e) => setPreferredLanguage(e.target.value)}>
            <option value="en">English</option>
            <option value="de">German</option>
            <option value="fr">French</option>
            <option value="es">Spanish</option>
          </select>
        </label>

        <label className="field">
          <span>Max Hashtags</span>
          <input
            data-testid="profile-max-hashtags"
            type="number"
            min="0"
            max="30"
            value={maxHashtags}
            onChange={(e) => setMaxHashtags(parseInt(e.target.value, 10) || 0)}
          />
        </label>

        <div className="field">
          <span>Formatting Rules</span>
          <div className="flex-row--wrap gap-2 mb-2">
            {formattingRules.map((rule, idx) => (
              <div key={idx} className="badge flex-row--center gap-1">
                <span>{rule}</span>
                <button
                  type="button"
                  className="btn btn--ghost btn--xs"
                  onClick={() => setFormattingRules(formattingRules.filter((_, i) => i !== idx))}
                >
                  <X size={12} />
                </button>
              </div>
            ))}
          </div>
          <div className="flex-row--center gap-2">
            <input
              value={newRule}
              onChange={(e) => setNewRule(e.target.value)}
              placeholder="Add a rule (e.g., No emojis)"
              onKeyDown={(e) => {
                if (e.key === 'Enter' && newRule.trim()) {
                  e.preventDefault()
                  setFormattingRules([...formattingRules, newRule.trim()])
                  setNewRule('')
                }
              }}
            />
            <button
              type="button"
              className="btn btn--secondary"
              onClick={() => {
                if (newRule.trim()) {
                  setFormattingRules([...formattingRules, newRule.trim()])
                  setNewRule('')
                }
              }}
            >
              Add
            </button>
          </div>
        </div>

        <div className="field">
          <span>Banned Words</span>
          <div className="flex-row--wrap gap-2 mb-2">
            {bannedWords.map((word, idx) => (
              <div key={idx} className="badge flex-row--center gap-1">
                <span>{word}</span>
                <button
                  type="button"
                  className="btn btn--ghost btn--xs"
                  onClick={() => setBannedWords(bannedWords.filter((_, i) => i !== idx))}
                >
                  <X size={12} />
                </button>
              </div>
            ))}
          </div>
          <div className="flex-row--center gap-2">
            <input
              value={newWord}
              onChange={(e) => setNewWord(e.target.value)}
              placeholder="Add a banned word"
              onKeyDown={(e) => {
                if (e.key === 'Enter' && newWord.trim()) {
                  e.preventDefault()
                  setBannedWords([...bannedWords, newWord.trim()])
                  setNewWord('')
                }
              }}
            />
            <button
              type="button"
              className="btn btn--secondary"
              onClick={() => {
                if (newWord.trim()) {
                  setBannedWords([...bannedWords, newWord.trim()])
                  setNewWord('')
                }
              }}
            >
              Add
            </button>
          </div>
        </div>

        <label className="field flex-row--center gap-2" style={{ flexDirection: 'row', alignItems: 'center' }}>
          <input
            data-testid="profile-auto-publish"
            type="checkbox"
            checked={autoPublishEnabled}
            onChange={(e) => setAutoPublishEnabled(e.target.checked)}
          />
          <span>Enable Auto-Publishing</span>
        </label>

        <div>
          <button
            data-testid="profile-save"
            type="button"
            className="btn btn--primary"
            onClick={handleSaveProfile}
            disabled={upsertProfile.isPending}
          >
            {upsertProfile.isPending ? 'Saving...' : 'Save Profile'}
          </button>
        </div>
      </div>

      <div className="glass-panel stack" data-testid="profile-examples-section">
        <div className="flex-row--between">
          <h2 className="section-card__title">Style Examples</h2>
          <Dialog.Root open={isExampleDialogOpen} onOpenChange={setIsExampleDialogOpen}>
            <Dialog.Trigger asChild>
              <button className="btn btn--secondary btn--sm" data-testid="profile-add-example">
                <Plus size={16} />
                <span>Add Example</span>
              </button>
            </Dialog.Trigger>
            <Dialog.Portal>
              <Dialog.Overlay className="dialog-overlay" />
              <Dialog.Content className="dialog-content" data-testid="example-dialog">
                <div className="drawer-header">
                  <Dialog.Title className="drawer-title">Add Style Example</Dialog.Title>
                  <Dialog.Close asChild>
                    <button className="btn btn--ghost btn--icon-sm">
                      <X size={20} />
                    </button>
                  </Dialog.Close>
                </div>
                <div className="drawer-body stack">
                  <label className="field">
                    <span>Platform</span>
                    <select data-testid="example-dialog-platform" value={examplePlatform} onChange={(e) => setExamplePlatform(e.target.value)}>
                      <option value="mastodon">Mastodon</option>
                      <option value="bluesky">Bluesky</option>
                      <option value="friendica">Friendica</option>
                      <option value="general">General</option>
                    </select>
                  </label>
                  <label className="field">
                    <span>Content</span>
                    <textarea
                      data-testid="example-dialog-content"
                      rows={4}
                      value={exampleContent}
                      onChange={(e) => setExampleContent(e.target.value)}
                      placeholder="Paste an example post here..."
                    />
                  </label>
                  <label className="field">
                    <span>Notes (Optional)</span>
                    <input
                      data-testid="example-dialog-notes"
                      value={exampleNotes}
                      onChange={(e) => setExampleNotes(e.target.value)}
                      placeholder="Why is this a good example?"
                    />
                  </label>
                  <div className="flex-row--end gap-2 mt-4">
                    <Dialog.Close asChild>
                      <button className="btn btn--ghost">Cancel</button>
                    </Dialog.Close>
                    <button
                      data-testid="example-dialog-submit"
                      className="btn btn--primary"
                      onClick={handleAddExample}
                      disabled={!exampleContent.trim() || createExample.isPending}
                    >
                      {createExample.isPending ? 'Adding...' : 'Add Example'}
                    </button>
                  </div>
                </div>
              </Dialog.Content>
            </Dialog.Portal>
          </Dialog.Root>
        </div>

        <div className="stack stack--sm mt-4">
          {styleExamples?.length === 0 ? (
            <p className="hint">No style examples added yet.</p>
          ) : (
            styleExamples?.map((example) => (
              <div key={example.id} className="glass-panel glass-panel--compact">
                <div className="flex-row--between mb-2">
                  <span className="badge">{example.platform}</span>
                  <button
                    type="button"
                    className="btn btn--ghost btn--xs"
                    onClick={() => handleDeleteExample(example.id)}
                    disabled={deleteExample.isPending}
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
                <p style={{ whiteSpace: 'pre-wrap', fontSize: '0.9rem' }}>{example.content}</p>
                {example.notes && (
                  <p className="hint mt-2" style={{ fontSize: '0.8rem' }}>
                    <strong>Notes:</strong> {example.notes}
                  </p>
                )}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
