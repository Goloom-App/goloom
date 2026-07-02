import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { useTranslation } from 'react-i18next'
import { CalendarDays, ChartBar, LayoutDashboard, Settings, Share2, Users } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

const TOUR_STEPS: { key: string; icon: LucideIcon }[] = [
  { key: 'dashboard', icon: LayoutDashboard },
  { key: 'calendar', icon: CalendarDays },
  { key: 'accounts', icon: Share2 },
  { key: 'team', icon: Users },
  { key: 'analytics', icon: ChartBar },
  { key: 'settings', icon: Settings },
]

/**
 * Short guided introduction shown once after the first sign-in (both for
 * users who created their team in onboarding and for invited users).
 */
export function PlatformTour({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const [step, setStep] = useState(0)

  const current = TOUR_STEPS[step]
  const Icon = current.icon
  const isLast = step === TOUR_STEPS.length - 1

  return (
    <Dialog.Root open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content" style={{ maxWidth: 440 }} data-testid="platform-tour">
          <div className="drawer-header">
            <Dialog.Title className="drawer-title">
              <Icon size={18} /> {t(`tour.${current.key}Title`)}
            </Dialog.Title>
          </div>
          <div className="drawer-body stack">
            <p>{t(`tour.${current.key}Text`)}</p>

            <div className="inline-cluster" aria-hidden="true" style={{ justifyContent: 'center', gap: 6 }}>
              {TOUR_STEPS.map((s, index) => (
                <span
                  key={s.key}
                  style={{
                    width: 8,
                    height: 8,
                    borderRadius: '50%',
                    background: index === step ? 'var(--accent, #7c3aed)' : 'var(--border, #444)',
                  }}
                />
              ))}
            </div>

            <div style={{ display: 'flex', gap: 8, justifyContent: 'space-between' }}>
              <button type="button" className="btn btn--ghost btn--sm" onClick={onClose} data-testid="tour-skip">
                {t('tour.skip')}
              </button>
              <div style={{ display: 'flex', gap: 8 }}>
                {step > 0 && (
                  <button type="button" className="btn btn--sm" onClick={() => setStep(step - 1)} data-testid="tour-back">
                    {t('tour.back')}
                  </button>
                )}
                {isLast ? (
                  <button type="button" className="btn btn--primary btn--sm" onClick={onClose} data-testid="tour-done">
                    {t('tour.done')}
                  </button>
                ) : (
                  <button
                    type="button"
                    className="btn btn--primary btn--sm"
                    onClick={() => setStep(step + 1)}
                    data-testid="tour-next"
                  >
                    {t('tour.next')}
                  </button>
                )}
              </div>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
