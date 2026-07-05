import { test, expect } from '@playwright/test'

import { tourGate } from '../src/components/onboarding/tourGate'

// The tour's "seen" state lives on the user account (server flag), not in the
// browser: clearing site data or switching devices must not re-open the tour,
// and a browser previously used by another user must not suppress it.

const ready = {
  authenticated: true,
  dashboardReady: true,
  invitePending: false,
  teamCount: 1,
}

test.describe('tourGate', () => {
  test('opens for a fresh user once the dashboard is ready', () => {
    expect(tourGate({ ...ready, serverTourDone: false, legacyLocalDone: false })).toEqual({
      open: true,
      syncLegacyDone: false,
    })
  })

  test('server flag wins: never opens for a user who finished the tour elsewhere', () => {
    expect(tourGate({ ...ready, serverTourDone: true, legacyLocalDone: false })).toEqual({
      open: false,
      syncLegacyDone: false,
    })
    // Even a stale local marker changes nothing once the server knows.
    expect(tourGate({ ...ready, serverTourDone: true, legacyLocalDone: true })).toEqual({
      open: false,
      syncLegacyDone: false,
    })
  })

  test('migrates a pre-existing local marker instead of replaying the tour', () => {
    // Users who finished the tour before the server flag existed only have the
    // localStorage marker: do not re-open, sync it to the account instead.
    expect(tourGate({ ...ready, serverTourDone: false, legacyLocalDone: true })).toEqual({
      open: false,
      syncLegacyDone: true,
    })
  })

  test('stays closed until the app is actually ready', () => {
    const fresh = { serverTourDone: false, legacyLocalDone: false }
    for (const notReady of [
      { ...ready, authenticated: false },
      { ...ready, dashboardReady: false },
      { ...ready, invitePending: true },
      { ...ready, teamCount: 0 },
    ]) {
      expect(tourGate({ ...notReady, ...fresh })).toEqual({ open: false, syncLegacyDone: false })
    }
  })
})
