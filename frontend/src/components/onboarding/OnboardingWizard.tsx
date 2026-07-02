import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Users } from 'lucide-react'

/**
 * Shown after sign-in while the user has no team yet: the first (and only
 * mandatory) onboarding step is creating a team. Users arriving through an
 * invite link never see this — accepting the invitation already puts them
 * into a team.
 */
export function OnboardingWizard({
  userName,
  userEmail,
  onCreate,
}: {
  userName: string
  userEmail: string
  onCreate: (name: string, description: string) => Promise<void>
}) {
  const { t } = useTranslation()
  const emailLocalPart = userEmail.includes('@') ? userEmail.split('@')[0] : ''
  const suggestedName = userName.trim() || emailLocalPart || t('onboarding.defaultTeamName')
  const [name, setName] = useState(suggestedName)
  const [description, setDescription] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit() {
    const trimmed = name.trim()
    if (!trimmed || submitting) return
    setSubmitting(true)
    setError(null)
    try {
      await onCreate(trimmed, description.trim())
    } catch (err) {
      setError(err instanceof Error && err.message ? err.message : t('teams.createTeamError'))
      setSubmitting(false)
    }
  }

  return (
    <div className="stack" data-testid="onboarding-wizard">
      <div>
        <h2 className="section-card__title">
          <Users size={18} /> {t('onboarding.welcomeTitle', { name: userName.trim() || emailLocalPart })}
        </h2>
        <p className="hint">{t('onboarding.welcomeText')}</p>
      </div>
      <form
        className="stack"
        onSubmit={(event) => {
          event.preventDefault()
          void handleSubmit()
        }}
      >
        <label className="stack stack--sm">
          <span>{t('teams.teamName')}</span>
          <input
            data-testid="onboarding-team-name"
            autoFocus
            value={name}
            onChange={(event) => setName(event.target.value)}
            maxLength={120}
            required
          />
        </label>
        <label className="stack stack--sm">
          <span>{t('teams.createTeamDescriptionLabel')}</span>
          <textarea
            data-testid="onboarding-team-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            placeholder={t('teams.descriptionPlaceholder')}
            rows={3}
          />
        </label>
        <p className="hint">{t('onboarding.teamHint')}</p>
        {error && (
          <p role="alert" style={{ color: 'var(--color-danger, #dc2626)', fontSize: 13 }}>
            {error}
          </p>
        )}
        <button
          type="submit"
          className="btn btn--primary"
          data-testid="onboarding-create-team"
          disabled={submitting || !name.trim()}
        >
          {submitting ? t('teams.createTeamCreating') : t('onboarding.createTeamButton')}
        </button>
      </form>
    </div>
  )
}
