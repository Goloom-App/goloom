# goloom

## Purpose

Self-hosted social media scheduling and automation server (Go backend, embedded React SPA) for Mastodon, Bluesky, and Friendica. This is the DOX rail: project-wide rules and the top-level Child DOX Index. The DOX framework itself is defined in `AGENT.md`.

## Local Contracts

- Single Go module; server entry `cmd/server`, bootstrap `internal/app/app.go`
- Frontend build output is embedded from `internal/webui/dist` (tracked: `index.html`, `manifest.json`)
- Locales: `locales/de.json` + `locales/en.json` must stay key-identical

## Work Guidance

- **Test-driven development is mandatory.** Write or extend a test alongside (ideally before) every behavior change; new logic without a test is unfinished work. Bug fixes start with a test that reproduces the bug.
- Silent fallbacks must be observable: when code degrades gracefully (e.g. AI falls back to template content), the failure must surface in logs *and* to the user.
- Store changes need both backends: implement and test in `internal/store/sqlite` and `internal/store/postgres` (`make test-postgres` — schema must apply on a fresh database, so new columns belong in `create table`, with `alter table … if not exists` only as migration for existing databases).

## Verification

- `go build ./...`, `go vet ./...`, `go test ./...` must pass
- `make test-postgres` must pass when the store layer changed
- `pnpm --dir frontend build` (includes `tsc -b`) and `pnpm --dir frontend lint` must pass when the frontend changed
- `make cover` reports total coverage — it must not drop through a change; raising it is part of the work (state: ~31 % total, 2026-06)

## Child DOX Index

- `internal/` — Go core: business logic, store, scheduler, providers, AI engine
- `frontend/` — React SPA (Vite, Playwright E2E)
- `develop/` — planning documents, feature catalogs, security mandates
