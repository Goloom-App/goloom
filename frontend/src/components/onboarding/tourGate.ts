export interface TourGateInput {
  authenticated: boolean
  dashboardReady: boolean
  invitePending: boolean
  teamCount: number
  /** The account-level flag from /v1/me — the source of truth. */
  serverTourDone: boolean
  /** The pre-existing localStorage marker from before the server flag existed. */
  legacyLocalDone: boolean
}

export interface TourGateDecision {
  open: boolean
  /** Migrate the legacy localStorage marker to the account instead of replaying the tour. */
  syncLegacyDone: boolean
}

// Decides whether to open the platform tour. The seen-state lives on the user
// account so it follows sign-ins across browsers and devices; the localStorage
// marker is only honored as a one-time migration for users who finished the
// tour before the server flag existed.
export function tourGate(input: TourGateInput): TourGateDecision {
  const ready = input.authenticated && input.dashboardReady && !input.invitePending && input.teamCount > 0
  if (!ready || input.serverTourDone) {
    return { open: false, syncLegacyDone: false }
  }
  if (input.legacyLocalDone) {
    return { open: false, syncLegacyDone: true }
  }
  return { open: true, syncLegacyDone: false }
}
