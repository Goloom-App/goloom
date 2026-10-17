# frontend

## Purpose

React SPA frontend built with Vite, embedded into the Go binary at build time via `internal/webui/`. PWA-enabled with Workbox.

## Ownership

Own package manager (pnpm), own build pipeline (tsc + vite), own test framework (Playwright), own linting (ESLint).

## Local Contracts

- Automation template fallbacks carry optional API `ai_fallback_reason`, mapped to `PostRecord.aiFallbackReason`; PostCard and review queue share a warning badge with localized reason tooltip including optional `ai_job_id` (`PostRecord.aiJobId`) to identify the failed job. Ordinary posts show no fallback badge. Mocked API coverage: `e2e/ai-fallback-badge.spec.ts`.

- RSS feeds display API `exhausted_items_count` as a localized warning when positive; mocked coverage: `e2e/rss-feeds.spec.ts`.
- Build output: `../internal/webui/dist` (critical cross-boundary contract)
- API proxy: `vite.config.ts` proxies `/api` to `localhost:8080`
- Entry: `index.html` → `src/main.tsx` → `src/App.tsx`
- Domain types: `src/types.ts` (single source of truth)
- API client: `src/api.ts`
- AI Generator prompt preview defaults to the plain “Generieren” action with current settings, even when an existing draft also offers “Neu generieren”; covered by `e2e/ai-prompt-preview.spec.ts`
- Composer optimization sends `params.no_publish: true` because results only update the editor; AI Generator generation/refinement retains team auto-publish behavior. Composer intent covered by `e2e/composer-ai-no-publish.spec.ts`.
- Team AI setup: saving provider config in `App.tsx` enables a newly toggled team only after the config save succeeds; keep this sequence covered by `e2e/team-ai-setup.spec.ts`
- Team audit UI: keep audit results component-local (outside persisted query cache), bind responses to the current team/filter/page, reset controls on team switches, distinguish failed queries from empty results, and retain navigation on empty pages. Successful API team mutations dispatch `goloom:team-mutation`; audit also refreshes every 30 seconds while visible and on window focus. Render only allowlisted metadata; action filters accept historical/unknown names. Actor kinds `oidc` (and historical `user`), `api_token`, and `system` map to member, API-key, and automation badges.
- Brand profile forms remount per team (`key={selectedTeam.id}`), clearing loaded profile and unsaved form state before editing another team.
- Generator draft/composer actions use the completed job's `payload.params.target_account_ids`; missing or empty targets show an error. Preserve account overrides and schedule during handoff.
- Desktop post previews toggle focus `{type: 'post', id}` on repeated selection; mobile previews focus their post until dismissed; persisted-post editing retains that identity through composer updates. Navigation and team changes clear prior selections/focus, including switching back. Team changes also reset the editor draft, composer mode/schedule error and composer chat context; the open composer stays blank with no destinations selected. Chat content reads still use team-scoped tools; regression: `e2e/ai-chat-post-focus.spec.ts`.
- Chat composer scope filters both `view_context.visible.composer.targets` and the model note. Detach and `/clear` survive automatic edits until the composer closes or its explicit AI-chat action re-attaches.
- Sidebar team menu stays within Radix available height/width; only team list scrolls, with Create team fixed below it. Preserve keyboard navigation and theme tokens; regression: `e2e/sidebar-team-menu.spec.ts`.
- Local-storage helpers: `src/appStorage.ts` (settings, last section/team keys)
- Theming: CSS variables in `src/index.css`; the accent family derives from `--brand-primary` (team brand color, set inline on `<html>` by `App.tsx`; default purple fallback). Theme/breakpoint hooks in `src/hooks/useTheme.ts`
- i18n: `src/i18n/` (i18next)
- Components: `src/components/` (reusable UI)
- Views: `src/views/` (route-level pages)
- Hooks: `src/hooks/` (shared React hooks)
- E2E tests: `e2e/` (Playwright)
- Design system: `src/components/ui/` (11 components)

## Work Guidance

- React 19, TypeScript 6, Vite 8
- Radix UI for accessible primitives
- TanStack Query for server state
- Recharts for charts
- i18next for translations
- Follow existing component patterns in `src/components/ui/`
- New views go in `src/views/<feature>/`
- New hooks go in `src/hooks/`
- E2E test for every user interaction change
- PWA manifest in `public/manifest.json`
- Service-worker/push and badge E2E runs on Playwright's full managed
  Chromium (`channel: 'chromium'`, the single project): chrome-headless-shell
  has no Push API (`ServiceWorker.deliverPushMessage` never reaches a worker)
  and crashes the renderer on the Badging API the badge sync calls on Settings
- Push CDP E2E uses a worker-local notification store: headless Chromium may
  resolve `showNotification` without retaining it; real `sw.js` push, click,
  and badge handlers still run

## Verification

- `pnpm run build` must succeed (tsc + vite)
- `pnpm run lint` must pass
- `npx playwright test` for E2E

## Child DOX Index

- `src/views/` — Route-level view components (13 views)
- `src/components/` — Reusable component library (10 subdirectories)
- `e2e/` — Playwright E2E tests
