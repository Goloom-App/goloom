# internal/store

## Purpose

Data access layer implementing the Store interface. Dual backend: SQLite (default) and PostgreSQL. Contains schema definitions, migration patterns, and the seriesfill utility.

## Ownership

Store interface in `store.go` (80+ methods). Backend implementations in `sqlite/` and `postgres/`.

## Local Contracts

- `WithAuditTransaction` routes callback-context store calls through one transaction, including decorated stores and nested backend savepoints; callback errors/cancellation roll back mutation and audit together. Keep provider/network calls outside transactions.
- `AfterAuditCommit` defers background dispatch/notifications until successful outer commit, discards hooks on rollback, and supplies the pre-transaction context.
- `EnqueueAuditEvent` atomically records observed external outcomes in `audit_pending`; `DrainAuditEvents` delivers bounded batches without repeating external work. Drain failures must be logged and retried by the caller.
- `LockAutomationSource` accepts only `rss_feed`/`post_template` and requires an active audit transaction; lock source before checking dedupe and submitting/materializing. `HasPostTemplateRoleMaterialized` includes pending/processing AI jobs for the same template, occurrence and role; terminal jobs permit genuine regeneration.
- `RepairFuturePostedPosts` writes one system-attributed `post.repair` event per affected post in the same transaction, retaining authenticated initiator IDs; no-op repairs emit nothing. `MarkPostProcessing` conditionally claims only pending/failed posts and errors when another worker already claimed them.
- `ClaimPostTargetPublication` conditionally marks pending/failed targets processing with `publication_outcome_unknown`; processing targets require reconciliation instead of blind publication retries. `ClaimAIJobCompletion` permits one pending/processing → terminal transition before completion side effects.
- Audit event IDs identify replayed evidence: duplicate IDs are ignored; pagination orders by `(created_at desc, id desc)`.
- `ClaimAIJobCompletion` atomically persists `completed`/`failed` only while status is not `completed` or `failed`; cancellation is `failed` with error message `cancelled`. Only the claim winner may run completion side effects. Missing/terminal jobs return false; nonterminal requested statuses return an error. `UpdateAIJobStatus` remains unconditional so the winner can persist later publication/automation failures
- AI job `origin` is server-owned and immutable through status updates: omitted origin and legacy rows default to `user`; scheduler-created jobs explicitly persist `automation`. All job reads (including global, pending, unfinished) preserve provenance; `AIJob.Origin` is excluded from JSON
- `LoadPostTargets` returns only unposted targets whose social account belongs to the scheduled post's team; persisted foreign-team target rows must never reach the publisher
- RSS completion writes (`IncrementRSSFeedCounter`, `UpdateRSSImportedItemPostID`) require `teamID`; foreign/missing feeds or items return `domain.ErrNotFound`. Item links also require the scheduled post to belong to that team
- Interface: `store.go` — all business logic depends on this contract
- `LookupAPIToken` returns the authenticating token's persisted expiry in `AuthenticatedPrincipal.TokenExpiresAt`; non-expiring tokens return nil in both backends.
- SQLite schema: `sqlite/schema.sql`
- PostgreSQL schema: `postgres/schema.sql`
- Top-level schema reference: `../../db/schema.sql`
- Media storage: `media_storage.go` interface
- Time series gap filling: `seriesfill/fill.go`

## Work Guidance

- Store exposes primitives only; composed read models (e.g. the AI context in `internal/aicontext`) are built outside the store so the two backends cannot drift. Post list primitives must return `TargetAccounts` identically in both backends
- Any new Store method must be added to the interface in `store.go`
- Implement in both `sqlite/` and `postgres/` packages
- New columns go into the `create table` statement of **both** schema files; `alter table … if not exists` is only the migration path for existing databases. The postgres schema must always apply cleanly on an empty database (indexes may not reference columns that only a later alter creates)
- Normalize nil slices/maps before writing not-null Postgres columns (pgx encodes nil as SQL NULL; sqlite JSON-encodes instead) — see `normalizedScopes` in `postgres/store.go`
- `PatchScheduledPost` with `publish_now` must take the fast path (only `scheduled_at` + `status`) and must never rewrite content/visibility/media from the patch — content edits are persisted in a separate PATCH before the queue action and must survive it, for both `draft` and non-draft posts
- `PatchScheduledPost` with the explicit review-transition signal (`review_complete`) must apply a status-guarded UPDATE (`where … status = 'draft'`) setting only `scheduled_at` + `status = 'pending'`, and must return `domain.ErrReviewCompleteConflict` when zero rows match — a stale queue action is rejected instead of overwriting a post that already left the draft state (e.g. a composer save scheduled it concurrently); the guard must be identical in both backends
- Missing records from `Get*` methods wrap `domain.ErrNotFound` together with the driver's no-rows error (`fmt.Errorf("x: %w: %w", domain.ErrNotFound, sql.ErrNoRows)`); API code checks `errors.Is(err, domain.ErrNotFound)`, never error strings
- `ai_service_configs` is unique per `team_id`: Postgres via constraint `ai_service_configs_team_id_key` in `create table` (plus dedupe + `create unique index if not exists` of the same name as migration), SQLite via `ux_ai_service_configs_team` in the migration (`sqlite/personal.go`, dedupe keeps newest row first). `UpsertAIServiceConfig` is one `insert … on conflict(team_id) do update` in both backends: stable id, empty api key keeps the stored key
- Tests use real databases: SQLite in-memory per test; Postgres via `make test-postgres` (tests skip without `TEST_POSTGRES_URL`)
- Postgres tests share one database per run — tests that need global state (e.g. "first user is admin") must reset it themselves
- Follow existing naming conventions in SQL queries
- Use parameterized queries to prevent SQL injection

## Verification

- `go test ./internal/store/...` must pass
- `make test-postgres` must pass for every store or schema change (throwaway container, fresh schema)
- Both SQLite and PostgreSQL implementations must pass identical test suites

## Child DOX Index

- `sqlite/` — SQLite implementation
- `postgres/` — PostgreSQL implementation
- `seriesfill/` — Time series gap filling utility
- `txcontext/` — Nested transaction commit hooks and clean side-effect contexts
