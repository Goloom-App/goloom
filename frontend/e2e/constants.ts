/** Plaintext token; must match server BOOTSTRAP_ADMIN_TOKEN (stored as API token hash at startup). */
export const E2E_DEFAULT_BOOTSTRAP_TOKEN = 'e2e_ci_bootstrap_token_value_32_chars_min_xx'

/** Seeded post title (see global-setup.ts). */
export const E2E_SEEDED_POST_TITLE = 'E2E Draft Post'

/** Automation review draft seeded in review-queue E2E. */
export const E2E_REVIEW_POST_TITLE = 'E2E Automation Review Draft'

export function e2eBootstrapToken(): string {
  return process.env.E2E_BOOTSTRAP_TOKEN?.trim() || E2E_DEFAULT_BOOTSTRAP_TOKEN
}
