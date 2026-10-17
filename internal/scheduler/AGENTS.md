# internal/scheduler

## Purpose

Background job scheduler for recurring posts, RSS imports, external post imports, and AI-triggered content generation. Manages worker pool, job dispatch, and template materialization.

## Ownership

Core scheduler in `scheduler.go`, specialized job types in separate files.

## Local Contracts

- AI dispatch failures that use RSS/recurring template content persist `AIFallbackReason=dispatch_failed` and log WARN with source IDs, post ID, and reason. Recurring announcement dispatch failures create the template fallback before advancing their counter. Completion fallbacks persist fixed reasons and log WARN after insertion with job/source/post IDs.

- Scheduler core: `scheduler.go` — poll loop, worker pool, job dispatch
- AI job submissions: RSS, recurring main posts, and recurring announcements set server-only `domain.AIJob.Origin=AIJobOriginAutomation` through `aijobs`; payload metadata alone never authorizes automation completion
- Recurring teardown cancels jobs through `ClaimAIJobCompletion`; concurrent completion winners keep their terminal result.
- Recurring AI: `recurring_ai.go` — recurring post generation via AI
- Recurring AI jobs snapshot template `visibility`, `media_ids`, and `media_exclude_by_account` in `params.recurring_automation` at queue time. The API callback uses that snapshot for main posts and announcements on success and template fallback; later template edits and AI output must not change those publication properties.
- Legacy queued jobs without non-null snapshot `visibility` use the current template's visibility, media, and exclusions, loaded with `GetPostTemplate(job.TeamID, templateID)`. Snapshot presence depends on the non-null visibility field, not its value; explicit public/empty visibility and empty/null attachments must not inherit later template edits. Legacy resolution is logged.
- Recurring callbacks resolve every attachment with `GetMediaItemByID(job.TeamID, id)` before insertion. Missing, foreign, or unreadable media aborts publication (including template fallback); completion logs details and exposes a failed job instead of silently publishing without attachments. Target accounts still pass through `postservice.Prepare(job.TeamID)`.
- RSS AI: `rss_ai.go` — RSS-triggered AI generation; reserve each item before submitting its job, retry known no-post failures/cancellation at most three times, and skip active/exhausted/consumed markers. Successfully imported items stay consumed after their post is deleted, for AI and direct imports. Polls reclaim orphaned/terminal-job reservations after a fifteen-minute lease, preserving the three-attempt cap; live jobs protect aged markers. RSS direct fallback links its item atomically with insertion.
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
- Automated post creation and `post.automation_create` audit commit together. RSS dedupe/counters share that transaction; recurring events correlate occurrence and role. System actors retain initiators separately.
- Direct template fallback after failed AI submission (RSS, recurring main, announcement) records `post.automation_create` with `outcome=fallback` and `error_code=ai_submission_failed`; retain source/author correlation and exclude raw errors. Normal AI-disabled creation stays `outcome=success`. RSS job creation and item reservation share a savepoint so failed reservation cannot dispatch an orphan job before fallback.
- Claim publication targets before fallible preparation. Persist provider outcomes with `EnqueueAuditEvent` in one transaction; drain at startup/poll and after outcomes. Delivery retries never invoke providers. Unknown processing targets require reconciliation; explicit rejections and preparation failures remain retryable.
- External calls and review push stay outside database transactions; defer side effects with `store.AfterAuditCommit` when callers can supply a transaction.
- Source locks serialize RSS imports and recurring generation with callback/skip/regeneration. Pending dedupe accepts only same-team jobs with server-owned automation provenance; authorship never substitutes for an authenticated initiator.

## Verification

- `go test ./internal/scheduler/...` must pass
- Scheduler tests in `scheduler_test.go`
- RSS import tests in `rss_import_test.go`

## Child DOX Index

None — flat package with specialized job files.
