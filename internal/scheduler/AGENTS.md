# internal/scheduler

## Purpose

Background job scheduler for recurring posts, RSS imports, external post imports, and AI-triggered content generation. Manages worker pool, job dispatch, and template materialization.

## Ownership

Core scheduler in `scheduler.go`, specialized job types in separate files.

## Local Contracts

- Scheduler core: `scheduler.go` — poll loop, worker pool, job dispatch
- Recurring AI: `recurring_ai.go` — recurring post generation via AI
- RSS AI: `rss_ai.go` — RSS-triggered AI generation
- RSS import: `rss_import.go` — RSS feed import jobs
- External post import: `external_post_import.go` — external post monitoring
- Template materialize: `template_materialize.go` — template → post conversion

## Work Guidance

- Scheduler runs as background goroutine in app bootstrap
- Job types registered in scheduler initialization
- Poll interval configurable via config
- Worker pool size configurable
- Job failures logged, not fatal
- Template materialization uses domain template variables
- Integration with store for job persistence

## Verification

- `go test ./internal/scheduler/...` must pass
- Scheduler tests in `scheduler_test.go`
- RSS import tests in `rss_import_test.go`

## Child DOX Index

None — flat package with specialized job files.
