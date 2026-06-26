import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface CreateTeamModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Create the team. Should throw on failure so the modal can show the error. */
  onCreate: (name: string, description: string) => Promise<void>
}

export function CreateTeamModal({ open, onOpenChange, onCreate }: CreateTeamModalProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleOpenChange = (next: boolean) => {
    if (submitting) return
    if (!next) {
      setName('')
      setDescription('')
      setError(null)
    }
    onOpenChange(next)
  }

  const handleSubmit = async () => {
    const trimmed = name.trim()
    if (!trimmed || submitting) return
    setSubmitting(true)
    setError(null)
    try {
      await onCreate(trimmed, description.trim())
      setName('')
      setDescription('')
      setSubmitting(false)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error && err.message ? err.message : t('teams.createTeamError', 'Could not create the team. Please try again.'))
      setSubmitting(false)
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={handleOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content" style={{ maxWidth: 480 }} data-testid="create-team-modal">
          <div className="drawer-header">
            <Dialog.Title className="drawer-title">{t('teams.createTeamTitle', 'Create a team')}</Dialog.Title>
            {!submitting && (
              <Dialog.Close asChild>
                <button type="button" className="btn btn--ghost btn--icon-sm" aria-label={t('common.close', 'Close')}>
                  <X size={20} />
                </button>
              </Dialog.Close>
            )}
          </div>
          <form
            className="drawer-body stack"
            onSubmit={(event) => {
              event.preventDefault()
              void handleSubmit()
            }}
          >
            <label className="stack stack--sm">
              <span>{t('teams.teamName', 'Team name')}</span>
              <input
                data-testid="create-team-name"
                autoFocus
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder={t('teams.createTeamNamePlaceholder', 'e.g. Marketing')}
                maxLength={120}
                required
              />
            </label>
            <label className="stack stack--sm">
              <span>{t('teams.createTeamDescriptionLabel', 'Description (optional)')}</span>
              <textarea
                data-testid="create-team-description"
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder={t('teams.descriptionPlaceholder', "Describe your team's purpose and focus")}
                rows={3}
              />
            </label>
            {error && <p role="alert" style={{ color: 'var(--color-danger, #dc2626)', fontSize: 13 }}>{error}</p>}
            <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
              <button
                type="submit"
                className="btn btn--primary"
                data-testid="create-team-submit"
                disabled={submitting || !name.trim()}
              >
                {submitting ? t('teams.createTeamCreating', 'Creating…') : t('teams.createTeamSubmit', 'Create team')}
              </button>
              <button type="button" className="btn" onClick={() => handleOpenChange(false)} disabled={submitting}>
                {t('common.cancel', 'Cancel')}
              </button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
