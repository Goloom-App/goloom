# frontend

## Purpose

React SPA frontend built with Vite, embedded into the Go binary at build time via `internal/webui/`. PWA-enabled with Workbox.

## Ownership

Own package manager (pnpm), own build pipeline (tsc + vite), own test framework (Playwright), own linting (ESLint).

## Local Contracts

- Build output: `../internal/webui/dist` (critical cross-boundary contract)
- API proxy: `vite.config.ts` proxies `/api` to `localhost:8080`
- Entry: `index.html` → `src/main.tsx` → `src/App.tsx`
- Domain types: `src/types.ts` (407 lines, single source of truth)
- API client: `src/api.ts`
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

## Verification

- `pnpm run build` must succeed (tsc + vite)
- `pnpm run lint` must pass
- `npx playwright test` for E2E

## Child DOX Index

- `src/views/` — Route-level view components (13 views)
- `src/components/` — Reusable component library (10 subdirectories)
- `e2e/` — Playwright E2E tests (13 spec files)
