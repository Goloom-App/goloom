import { test, expect } from '@playwright/test'

import { tourStepsForRole } from '../src/components/onboarding/tourSteps'

// Pure-logic coverage for the role-specific guided tour flows: admins are
// routed through platform (provider) setup first, regular users go straight
// to connecting an account.

test.describe('tourStepsForRole', () => {
  test('admin flow sets up a provider before accounts', () => {
    const ids = tourStepsForRole(true).map((s) => s.id)
    expect(ids[0]).toBe('welcome')
    expect(ids[ids.length - 1]).toBe('finish')
    expect(ids).toContain('open-admin')
    expect(ids).toContain('admin-providers')
    expect(ids.indexOf('admin-providers')).toBeLessThan(ids.indexOf('nav-accounts'))
  })

  test('user flow has no admin steps', () => {
    const ids = tourStepsForRole(false).map((s) => s.id)
    expect(ids).toEqual(['welcome', 'nav-accounts', 'accounts-connect', 'new-post', 'finish'])
  })

  test('steps are internally consistent', () => {
    for (const steps of [tourStepsForRole(true), tourStepsForRole(false)]) {
      for (const step of steps) {
        if (step.advanceOn === 'section') {
          // Section steps need a destination and something to highlight.
          expect(step.section, step.id).toBeTruthy()
          expect(step.target, step.id).toBeTruthy()
        }
        if (step.advanceOn === 'click') {
          expect(step.target, step.id).toBeTruthy()
        }
      }
      // First and last step are centered cards (no spotlight target).
      expect(steps[0].target).toBeUndefined()
      expect(steps[steps.length - 1].target).toBeUndefined()
    }
  })
})
