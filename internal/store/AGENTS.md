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
- Schema changes need migration SQL (ALTER TABLE) in both backends
- Tests use real database (SQLite in-memory or PostgreSQL test DB)
- Follow existing naming conventions in SQL queries
- Use parameterized queries to prevent SQL injection

## Verification

- `go test ./internal/store/...` must pass
- Both SQLite and PostgreSQL implementations must pass identical test suites

## Child DOX Index

- `sqlite/` — SQLite implementation (28 files)
- `postgres/` — PostgreSQL implementation (25 files)
- `seriesfill/` — Time series gap filling utility
