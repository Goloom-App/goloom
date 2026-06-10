# internal

## Purpose

Go internal packages — the core application logic. Not importable by external Go modules. Contains all business logic, data access, authentication, scheduling, and provider integrations.

## Ownership

Single Go module (`go.mod` at root). Packages follow standard Go conventions.

## Local Contracts

- App bootstrap: `app/app.go` → `Run(ctx)` function
- Domain models: `domain/models.go` (shared across all packages)
- Store interface: `store/store.go` (80+ methods, the persistence contract)
- Auth middleware: `auth/auth.go`
- Config: `config/config.go`
- Embedded frontend: `webui/webui.go` (serves built Vite output)

## Work Guidance

- Follow Go package conventions (package name matches directory)
- Internal packages cannot be imported outside this module
- Use `slog` for structured logging (`logging/logging.go`)
- Security utilities in `security/security.go` (AES-GCM encryption, rate limiting)
- SSE hub in `sse/hub.go` for real-time updates
- New packages should be small and focused
- Tests use standard `testing` package with table-driven patterns

## Verification

- `go build ./...` must succeed
- `go test ./...` must pass
- `go vet ./...` must pass

## Child DOX Index

- `auth/` — Authentication, authorization, OIDC, feature flags
- `domain/` — Domain models, patch semantics, template variables
- `provider/` — Social media provider integrations (Bluesky, Friendica, Mastodon)
- `scheduler/` — Background job scheduler, recurring/RSS automation
- `store/` — Data access layer (SQLite + PostgreSQL dual backend)
