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

---

# TDD Development Workflow

## Principles

- **Test first**: Write tests before implementation code.
- **Minimal code**: Only code needed to pass tests.
- **Small cycles**: One unit per iteration.
- **Security by design**: Security constraints before implementation.
- **E2E for frontend**: Every UI change needs browser test (Playwright).

## Roles & Agents

| Role | Agent | Task |
|---|---|---|
| **Planner** | `plan` (Prometheus) | Feature spec |
| **Security Analyst** | `oracle` | Add security constraints |
| **Test Writer** | `deep` + skills | Write tests (unit + E2E) |
| **Developer** | `deep` / `visual-engineering` | Minimal implementation |
| **Security Reviewer** | `oracle` | Review finished code |
| **Committer** | Sisyphus | Commit & push |

## Workflow

### Step 1: Feature Planning

```
Agent: task(subagent_type="plan", ...)
Input:  User story / feature description
Output: Plan with:
  - Acceptance criteria
  - Interfaces / data flow
  - Independent units
  - Frontend components (if applicable)
```

Plan saved in `.sisyphus/plans/<feature>.md`, validated with `momus`.

### Step 2: Security Requirements

```
Agent: task(subagent_type="oracle", ...)
Input:  Feature plan
Output: Security constraints:
  - Auth / authorization (who can do what?)
  - Input validation (XSS, SQL injection, etc.)
  - Rate limiting / DoS prevention
  - Data encryption (PII, tokens)
  - CSRF / CORS (frontend)
```

Append `<security>` section to plan.

### Step 3: Tests (RED)

```
Agent: task(category="deep", ...)
Input:  Feature plan + security constraints
Output: FAILING tests:
  - Go unit tests (_test.go) for backend logic
  - Integration tests for API endpoints
  - Playwright E2E tests for frontend changes
```

**Backend tests:**
- `package_test.go` with `testing.T`
- Mock interfaces for external deps
- Table-driven tests for edge cases

**Frontend E2E (Playwright):**
- `frontend/tests/<feature>.spec.ts`
- Test input validation, error states, success paths
- Use `page.fill()`, `page.click()`, `page.expect()`

### Step 4: Implementation (GREEN)

```
Agent: task(category="deep" / "visual-engineering", ...)
Input:  Feature plan + security constraints + tests
Output: Minimal code passing all tests:
  - Only what's needed
  - Follow existing patterns (codebase check)
  - No refactoring beyond test requirements
```

After each implementation:
1. `go build ./...` check
2. `go test ./<package>/...` run
3. Fail → iterate
4. Pass → next unit (step 3)

### Step 5: Security Review

```
Agent: task(subagent_type="oracle", ...)
Input:  Finished code + tests
Output: Review:
  - Vulnerabilities found
  - Critical → fix immediately
  - Non-critical → file Forgejo issue
```

### Step 6: Commit & Push

```
Exec: Sisyphus (orchestrator)
Output: Logical commits (Conventional Commits)
```

1. `git add -p` for logical grouping
2. Commit: `type(scope): short description`
3. `git push origin main`

## Cycle Diagram

```
┌─────────────────┐
│  1. Planning    │  Prometheus → plan with units
└────────┬────────┘
         ↓
┌─────────────────┐
│  2. Security    │  Oracle → security constraints
└────────┬────────┘
         ↓
┌─────────────────┐     ┌──────────────────────┐
│  3. Tests (RED) │────→│ 4. Code (GREEN)      │
│  deep + skills  │←────│  deep/visual-eng.     │
└─────────────────┘     └──────────────────────┘
         │                     │
         └─── fail ────────────┘
         │
         ✔ all units done
         ↓
┌─────────────────┐
│  5. Security    │  Oracle → review
└────────┬────────┘
         ↓
┌─────────────────┐
│  6. Commit      │  Sisyphus → git push
└─────────────────┘
```

## Coverage Targets

- **Backend**: ≥ 80% coverage for new features
- **API endpoints**: 100% status codes tested (200, 400, 401, 403, 404, 500)
- **Frontend**: Every user interaction (fill, click, submit) as E2E test
- **Edge cases**: Empty input, boundaries, timeouts, missing permissions

## Skills for Test Development

- `playwright` — E2E browser tests (Playwright MCP)
- `frontend-ui-ux` — UI component tests
- `git-master` — Git operations in test setup

## Notes

- Frontend changes → load `playwright` skill for E2E
- API changes → integration tests with `httptest.NewRecorder()`
- DB changes → in-memory SQLite for tests
- Security vulns in review → stop immediately, file issue, fix

## Communication Style — Caveman Mode

Talk like smart caveman. Same brain, fewer tokens.

Compress every response to caveman-style prose. Drop articles, filler, pleasantries, hedging. Keep every technical detail, code block, error string, and symbol exact. Cuts ~65-75% output tokens with full accuracy. Mode persists whole session until changed or stopped.

### Intensity Levels

| Level | What change |
|-------|-------------|
| `lite` | Drop filler/hedging. Sentences stay full. Professional but tight. |
| `full` | Default. Drop articles, fragments OK, short synonyms. |
| `ultra` | Bare fragments. Abbreviations (DB, auth, fn). Arrows for causality. |
| `wenyan-lite` | Classical Chinese register, light compression. |
| `wenyan-full` | Maximum 文言文. 80-90% character reduction. |
| `wenyan-ultra` | Extreme classical compression. |

### Auto-clarity rule

Caveman drops to normal prose for:
- Security warnings
- Irreversible-action confirmations
- Multi-step sequences where fragment ambiguity risks misread
- When user repeats a question

Resumes after clear part.

### How to invoke

```
/caveman              # full mode (default)
/caveman lite         # lighter compression
/caveman ultra        # extreme compression
/caveman wenyan       # classical Chinese
stop caveman          # back to normal prose
```

### Example

Question: "Why does my React component re-render?"

Normal:
> Your component re-renders because you create a new object reference each render. Wrapping it in `useMemo` will fix the issue.

Caveman (full):
> New object ref each render. Inline object prop = new ref = re-render. Wrap in `useMemo`.

Caveman (ultra):
> Inline obj prop → new ref → re-render. `useMemo`.

---

## Child DOX Index

| Child | Scope | Parent-owned rules |
|---|---|---|
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

- `AGENT.md` — This file (DOX root + TDD workflow)
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
