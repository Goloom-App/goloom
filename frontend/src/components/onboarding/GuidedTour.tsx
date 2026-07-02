import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

import type { AppSection } from '../../types'
import type { TourStep } from './tourSteps'

const SPOTLIGHT_PADDING = 6
const TOOLTIP_WIDTH = 330
const TOOLTIP_GAP = 14
/** How long to wait for a step's target before falling back to a centered card. */
const TARGET_WAIT_MS = 4000
const TARGET_POLL_MS = 150

/** First visible element carrying the data-tour anchor (sidebar vs mobile nav). */
function findTarget(anchor: string): HTMLElement | null {
  for (const el of Array.from(document.querySelectorAll<HTMLElement>(`[data-tour="${anchor}"]`))) {
    const rect = el.getBoundingClientRect()
    if (rect.width > 0 && rect.height > 0) {
      return el
    }
  }
  return null
}

interface Spotlight {
  top: number
  left: number
  width: number
  height: number
}

function spotlightFor(el: HTMLElement): Spotlight {
  const rect = el.getBoundingClientRect()
  return {
    top: rect.top - SPOTLIGHT_PADDING,
    left: rect.left - SPOTLIGHT_PADDING,
    width: rect.width + SPOTLIGHT_PADDING * 2,
    height: rect.height + SPOTLIGHT_PADDING * 2,
  }
}

function tooltipPosition(spot: Spotlight): { top: number; left: number } {
  const vw = window.innerWidth
  const vh = window.innerHeight
  // Prefer beside the target, then below, then above.
  if (spot.left + spot.width + TOOLTIP_GAP + TOOLTIP_WIDTH <= vw - 8) {
    return { top: Math.min(Math.max(spot.top, 8), vh - 260), left: spot.left + spot.width + TOOLTIP_GAP }
  }
  const left = Math.min(Math.max(spot.left, 8), vw - TOOLTIP_WIDTH - 8)
  if (spot.top + spot.height + TOOLTIP_GAP + 220 <= vh) {
    return { top: spot.top + spot.height + TOOLTIP_GAP, left }
  }
  return { top: Math.max(spot.top - TOOLTIP_GAP - 220, 8), left }
}

/**
 * Interactive product tour: dims the app, spotlights the element the user
 * should interact with, and explains it in a tooltip beside it. Steps advance
 * when the user actually clicks the spotlighted element (or reaches the target
 * section for multi-click paths like menus); explanatory steps use "Next".
 */
export function GuidedTour({
  steps,
  section,
  onClose,
}: {
  steps: TourStep[]
  section: AppSection
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [index, setIndex] = useState(0)
  const [spot, setSpot] = useState<Spotlight | null>(null)
  const [targetMissing, setTargetMissing] = useState(false)
  const targetRef = useRef<HTMLElement | null>(null)

  const step = steps[index]
  const isLast = index === steps.length - 1

  const next = useCallback(() => {
    if (isLast) {
      onClose()
      return
    }
    setIndex((current) => current + 1)
  }, [isLast, onClose])

  // Escape skips the tour from anywhere.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onClose()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  // Section steps advance as soon as the user arrives (multi-click paths).
  useEffect(() => {
    if (step.advanceOn === 'section' && step.section && step.section === section) {
      const timer = window.setTimeout(next, 350)
      return () => window.clearTimeout(timer)
    }
  }, [step, section, next])

  // Locate and track the step's target. Polls while the element is still
  // rendering (e.g. right after a navigation) and keeps the spotlight glued
  // to it through scrolling, resizes and layout animations.
  useEffect(() => {
    targetRef.current = null
    setSpot(null)
    setTargetMissing(!step.target)
    if (!step.target) {
      return
    }

    let cancelled = false
    let raf = 0
    let waited = 0

    const track = () => {
      if (cancelled) {
        return
      }
      const el = targetRef.current
      if (el && el.isConnected) {
        setSpot(spotlightFor(el))
      }
      raf = window.requestAnimationFrame(track)
    }

    const poll = () => {
      if (cancelled) {
        return
      }
      const el = findTarget(step.target!)
      if (el) {
        targetRef.current = el
        el.scrollIntoView({ block: 'nearest', inline: 'nearest' })
        setTargetMissing(false)
        setSpot(spotlightFor(el))
        track()
        return
      }
      waited += TARGET_POLL_MS
      if (waited >= TARGET_WAIT_MS) {
        // Target never showed up (e.g. mobile layout without this control):
        // degrade to a centered card so the tour stays usable.
        setTargetMissing(true)
        return
      }
      window.setTimeout(poll, TARGET_POLL_MS)
    }
    poll()

    return () => {
      cancelled = true
      window.cancelAnimationFrame(raf)
    }
  }, [step])

  // Click steps advance when the user clicks the spotlighted element itself.
  const targetFound = spot !== null
  useEffect(() => {
    if (step.advanceOn !== 'click' || !targetFound) {
      return
    }
    const el = targetRef.current
    if (!el) {
      return
    }
    const onClick = () => {
      window.setTimeout(next, 350)
    }
    el.addEventListener('click', onClick, { once: true })
    return () => el.removeEventListener('click', onClick)
  }, [step, targetFound, next])

  const centered = !step.target || targetMissing
  const tooltipStyle = !centered && spot ? tooltipPosition(spot) : undefined
  const waitsForUser = step.advanceOn !== 'next' && !targetMissing

  return createPortal(
    <div className="guided-tour" data-testid="guided-tour" data-tour-step={step.id}>
      {centered || !spot ? (
        <div className="guided-tour__dim guided-tour__dim--full" />
      ) : (
        <>
          {/* Four panels leave a click-through hole over the target. */}
          <div className="guided-tour__dim" style={{ top: 0, left: 0, right: 0, height: Math.max(spot.top, 0) }} />
          <div
            className="guided-tour__dim"
            style={{ top: spot.top, left: 0, width: Math.max(spot.left, 0), height: spot.height }}
          />
          <div
            className="guided-tour__dim"
            style={{ top: spot.top, left: spot.left + spot.width, right: 0, height: spot.height }}
          />
          <div className="guided-tour__dim" style={{ top: spot.top + spot.height, left: 0, right: 0, bottom: 0 }} />
          <div
            className="guided-tour__ring"
            style={{ top: spot.top, left: spot.left, width: spot.width, height: spot.height }}
          />
        </>
      )}

      <div
        className={`guided-tour__tooltip glass-panel ${centered ? 'guided-tour__tooltip--centered' : ''}`}
        style={tooltipStyle}
        role="dialog"
        aria-label={t(step.titleKey)}
      >
        <p className="eyebrow">{t('tour.progress', { current: index + 1, total: steps.length })}</p>
        <h3 className="guided-tour__title">{t(step.titleKey)}</h3>
        <p className="guided-tour__text">{t(step.textKey)}</p>
        <div className="guided-tour__footer">
          <button type="button" className="btn btn--ghost btn--sm" onClick={onClose} data-testid="tour-skip">
            {t('tour.skip')}
          </button>
          {waitsForUser ? (
            <span className="guided-tour__hint">{t('tour.clickHint')}</span>
          ) : (
            <button
              type="button"
              className="btn btn--primary btn--sm"
              onClick={next}
              data-testid={isLast ? 'tour-done' : 'tour-next'}
            >
              {isLast ? t('tour.done') : t('tour.next')}
            </button>
          )}
        </div>
      </div>
    </div>,
    document.body,
  )
}
