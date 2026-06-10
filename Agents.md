# DOX framework

- DOX is highly performant AGENTS.md hierarchy installed here
- Agent must follow DOX instructions across any edits

## Core Contract

- AGENTS.md files are binding work contracts for their subtrees
- Work products, source materials, instructions, records, assets, and durable docs must stay understandable from the nearest applicable AGENTS.md plus every parent AGENTS.md above it

## Read Before Editing

1. Read the root AGENTS.md
2. Identify every file or folder you expect to touch
3. Walk from the repository root to each target path
4. Read every AGENTS.md found along each route
5. If a parent AGENTS.md lists a child AGENTS.md whose scope contains the path, read that child and continue from there
6. Use the nearest AGENTS.md as the local contract and parent docs for repo-wide rules
7. If docs conflict, the closer doc controls local work details, but no child doc may weaken DOX

Do not rely on memory. Re-read the applicable DOX chain in the current session before editing.

## Update After Editing

Every meaningful change requires a DOX pass before the task is done.

Update the closest owning AGENTS.md when a change affects:

- purpose, scope, ownership, or responsibilities
- durable structure, contracts, workflows, or operating rules
- required inputs, outputs, permissions, constraints, side effects, or artifacts
- user preferences about behavior, communication, process, organization, or quality
- AGENTS.md creation, deletion, move, rename, or index contents

Update parent docs when parent-level structure, ownership, workflow, or child index changes. Update child docs when parent changes alter local rules. Remove stale or contradictory text immediately. Small edits that do not change behavior or contracts may leave docs unchanged, but the DOX pass still must happen.

## Hierarchy

- Root AGENTS.md is the DOX rail: project-wide instructions, global preferences, durable workflow rules, and the top-level Child DOX Index
- Child AGENTS.md files own domain-specific instructions and their own Child DOX Index
- Each parent explains what its direct children cover and what stays owned by the parent
- The closer a doc is to the work, the more specific and practical it must be

## Child Doc Shape

- Create a child AGENTS.md when a folder becomes a durable boundary with its own purpose, rules, responsibilities, workflow, materials, or quality standards
- Work Guidance must reflect the current standards of the project or user instructions; if there are no specific standards or instructions yet, leave it empty
- Verification must reflect an existing check; if no verification framework exists yet, leave it empty and update it when one exists

Default section order:
- Purpose
- Ownership
- Local Contracts
- Work Guidance
- Verification
- Child DOX Index

## Style

- Keep docs concise, current, and operational
- Document stable contracts, not diary entries
- Put broad rules in parent docs and concrete details in child docs
- Prefer direct bullets with explicit names
- Do not duplicate rules across many files unless each scope needs a local version
- Delete stale notes instead of explaining history
- Trim obvious statements, repeated rules, misplaced detail, and warnings for risks that no longer exist

## Closeout

1. Re-check changed paths against the DOX chain
2. Update nearest owning docs and any affected parents or children
3. Refresh every affected Child DOX Index
4. Remove stale or contradictory text
5. Run existing verification when relevant
6. Report any docs intentionally left unchanged and why

## User Preferences

When the user requests a durable behavior change, record it here or in the relevant child AGENTS.md

## Child DOX Index

| Child | Scope | Parent-owned rules |
|---|---|---|
| [`ai-service/AGENTS.md`](ai-service/AGENTS.md) | Python FastAPI microservice for AI content generation | LLM adapters, prompt templates, job workers |
| [`frontend/AGENTS.md`](frontend/AGENTS.md) | React SPA (Vite, TypeScript, PWA) embedded in Go binary | Build output contract: `internal/webui/dist`, API proxy config |
| [`internal/AGENTS.md`](internal/AGENTS.md) | Go core packages — all business logic | Package dependency graph, bootstrap sequence, shared concerns (config, logging, store interface) |
| [`internal/auth/AGENTS.md`](internal/auth/AGENTS.md) | Auth, authorization, OIDC, feature flags | Security-critical: middleware chains, scope definitions, bootstrap token lifecycle |
| [`internal/domain/AGENTS.md`](internal/domain/AGENTS.md) | Domain models shared across all packages | Pure data types: patch semantics, template variables |
| [`internal/provider/AGENTS.md`](internal/provider/AGENTS.md) | Social media provider integrations | Bluesky, Friendica, Mastodon implementations per Provider interface |
| [`internal/scheduler/AGENTS.md`](internal/scheduler/AGENTS.md) | Background job scheduler | Recurring/RSS automation, worker pool, template materialization |
| [`internal/store/AGENTS.md`](internal/store/AGENTS.md) | Data access layer (SQLite + PostgreSQL) | Store interface (80+ methods), dual-backend schema contract |
| [`develop/AGENTS.md`](develop/AGENTS.md) | Planning docs, feature catalogs, design specs | Plan structure conventions, feature catalog format |

### Directories without AGENTS.md (no durable boundary needed)

- `.cursor/` — IDE agent config
- `.e2e/` — E2E test runtime artifacts (gitignored)
- `.forgejo/` — CI/CD workflows (self-contained YAML)
- `.pnpm-store/` — pnpm cache
- `.sisyphus/` — AI orchestration state (borderline; may warrant AGENTS.md if conventions grow)
- `api/` — Go HTTP handlers (follows Go conventions, borderline)
- `bin/` — Compiled binary (build artifact)
- `cmd/server/` — Go entry point (single file)
- `data/` — Runtime SQLite data (gitignored)
- `db/` — Database schemas (borderline; schema evolution process is durable)
- `docs/` — API specs + migration guides (self-documenting)
- `locales/` — Backend i18n files (small, self-contained)
- `website/` — Documentation site (borderline; Astro/Starlight with own build)

### Files

- `AGENT.md` — TDD development workflow (root-level, separate from DOX hierarchy)
- `README.md` — Project overview
- `ROADMAP.md` — 5-phase implementation roadmap
- `Makefile` — Build orchestration for all components
- `Dockerfile` — Multi-stage Docker build
- `docker-compose.yml` — Dev/SQLite mode
- `docker-compose-traefik.yml` — Production deployment
- `flake.nix` — Nix dev shell
- `go.mod` / `go.sum` — Go module definition
- `redocly.yaml` — Redocly API docs config
- `renovate.json` — Dependency update automation
- `pyrightconfig.json` — Python type checking config
