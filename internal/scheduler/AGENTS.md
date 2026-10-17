# internal/scheduler

## Purpose

Background job scheduler for recurring posts, RSS imports, external post imports, and AI-triggered content generation. Manages worker pool, job dispatch, and template materialization.

## Ownership

Core scheduler in `scheduler.go`, specialized job types in separate files.

## Local Contracts

- Scheduler core: `scheduler.go` — poll loop, worker pool, job dispatch
- AI job submissions: RSS, recurring main posts, and recurring announcements set server-only `domain.AIJob.Origin=AIJobOriginAutomation` through `aijobs`; payload metadata alone never authorizes automation completion
- Recurring AI: `recurring_ai.go` — recurring post generation via AI
- Recurring AI jobs snapshot template `visibility`, `media_ids`, and `media_exclude_by_account` in `params.recurring_automation` at queue time. The API callback uses that snapshot for main posts and announcements on success and template fallback; later template edits and AI output must not change those publication properties.
- Legacy queued jobs without non-null snapshot `visibility` use the current template's visibility, media, and exclusions, loaded with `GetPostTemplate(job.TeamID, templateID)`. Snapshot presence depends on the non-null visibility field, not its value; explicit public/empty visibility and empty/null attachments must not inherit later template edits. Legacy resolution is logged.
- Recurring callbacks resolve every attachment with `GetMediaItemByID(job.TeamID, id)` before insertion. Missing, foreign, or unreadable media aborts publication (including template fallback); completion logs details and exposes a failed job instead of silently publishing without attachments. Target accounts still pass through `postservice.Prepare(job.TeamID)`.
- RSS AI: `rss_ai.go` — RSS-triggered AI generation
- RSS import: `rss_import.go` — RSS feed import jobs
- External post import: `external_post_import.go` — external post monitoring
- Template materialize: `template_materialize.go` — template → post conversion

## Work Guidance

- Scheduler runs as background goroutine in app bootstrap
- Job types registered in scheduler initialization
- Poll interval configurable via config
- Worker pool size configurable
- Before publishing, validate the full persisted target list through `postservice.ResolveTargets` against the post team, including already-posted targets on retries. Any invalid target fails the whole post with a log and stored error before publishing any account; never treat filtered targets as success
- Job failures logged, not fatal
- Template materialization uses domain template variables
- Integration with store for job persistence

## Verification

- `go test ./internal/scheduler/...` must pass
- Scheduler tests in `scheduler_test.go`
- RSS import tests in `rss_import_test.go`

## Child DOX Index

None — flat package with specialized job files.
