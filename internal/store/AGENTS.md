# internal/store

## Purpose

Data access layer implementing the Store interface. Dual backend: SQLite (default) and PostgreSQL. Contains schema definitions, migration patterns, and the seriesfill utility.

## Ownership

Store interface in `store.go` (80+ methods). Backend implementations in `sqlite/` and `postgres/`.

## Local Contracts

- Interface: `store.go` — all business logic depends on this contract
- SQLite schema: `sqlite/schema.sql`
- PostgreSQL schema: `postgres/schema.sql`
- Top-level schema reference: `../../db/schema.sql`
- Media storage: `media_storage.go` interface
- Time series gap filling: `seriesfill/fill.go`

## Work Guidance

- Any new Store method must be added to the interface in `store.go`
- Implement in both `sqlite/` and `postgres/` packages
- New columns go into the `create table` statement of **both** schema files; `alter table … if not exists` is only the migration path for existing databases. The postgres schema must always apply cleanly on an empty database (indexes may not reference columns that only a later alter creates)
- Normalize nil slices/maps before writing not-null Postgres columns (pgx encodes nil as SQL NULL; sqlite JSON-encodes instead) — see `normalizedScopes` in `postgres/store.go`
- `PatchScheduledPost` with `publish_now` must take the fast path (only `scheduled_at` + `status`) and must never rewrite content/visibility/media from the patch — content edits are persisted in a separate PATCH before the queue action and must survive it, for both `draft` and non-draft posts
- `PatchScheduledPost` with the explicit review-transition signal (`review_complete`) must apply a status-guarded UPDATE (`where … status = 'draft'`) setting only `scheduled_at` + `status = 'pending'`, and must return `domain.ErrReviewCompleteConflict` when zero rows match — a stale queue action is rejected instead of overwriting a post that already left the draft state (e.g. a composer save scheduled it concurrently); the guard must be identical in both backends
- Tests use real databases: SQLite in-memory per test; Postgres via `make test-postgres` (tests skip without `TEST_POSTGRES_URL`)
- Postgres tests share one database per run — tests that need global state (e.g. "first user is admin") must reset it themselves
- Follow existing naming conventions in SQL queries
- Use parameterized queries to prevent SQL injection

## Verification

- `go test ./internal/store/...` must pass
- `make test-postgres` must pass for every store or schema change (throwaway container, fresh schema)
- Both SQLite and PostgreSQL implementations must pass identical test suites

## Child DOX Index

- `sqlite/` — SQLite implementation (43 files)
- `postgres/` — PostgreSQL implementation (34 files)
- `seriesfill/` — Time series gap filling utility
