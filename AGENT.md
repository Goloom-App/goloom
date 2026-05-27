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

## Communication Style

Full‑intensity caveman mode always active: articles, filler words and pleasantries omitted; sentences are short fragments, technical terms unchanged. Keeps communication terse, retains all technical substance.
