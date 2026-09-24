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
- Cross-boundary contract: push notification deep links are
  `/?team=<id>&section=reviewQueue`, matching the frontend `AppSection` value
- Push (`internal/push`): subscription endpoints are HTTPS-only (rejected with
  `push_subscription_endpoint_https_required`); the default HTTP client is
  SSRF-guarded (dial + redirect resolution restricted to public addresses, own
  `resolver` seam injectable for tests; `isPublicAddr` additionally denies the
  special-use prefixes in `nonPublicPrefixes`: CGNAT, TEST-NET/IPv6
  documentation, benchmarking, deprecated 6to4/site-local, reserved
  future-use and discard-only ranges, with IPv4-mapped forms normalized via
  `Unmap`); delivery of a batch is bounded by a single `DeliveryTimeout`
  (default 10s) spanning all targets; each target's notification payload
  embeds the user-wide open-review total from `CountUserOpenReviewItems`;
  logs show only the endpoint origin, never the full URL; notification `Topic`
  is `base64.RawURLEncoding(sha256(teamID)[:24])`
- Badge synchronization (frontend → sw.js): the app polls `/v1/me/review-counts`
  when authenticated (cookie session suffices, no stored bearer needed) and
  posts `review-badge` messages with `{total, teams}`; a `total <= 0` closes
  every `review-*` notification (empty `teams` after the last team drops out
  must not leave stale notifications); the async `closeTeamNotifications`/
  `closeAllReviewNotifications` promises are kept alive via
  `event.waitUntil(Promise.all(...))` so an idle-stopped worker cannot strand
  notifications under a cleared badge (E2E asserts the shipped `/sw.js` keeps
  the waitUntil call)
- API review counts (`api/push_subscriptions.go`): a team contributes to the
  badge only if the user holds an assigned role (`UserHasAnyTeamRole` with
  RoleEditor/RoleOwner) and the record is not a viewer membership — viewers
  get no badge for teams they only observe
- Team role checks have no hierarchy: `UserHasAnyTeamRole` matches the stored role exactly. Always pass the complete allowed list — writes need `(RoleEditor, RoleOwner)`, reads `(RoleViewer, RoleEditor, RoleOwner)` — or owners get locked out
- The first user in an empty database becomes admin (`UpsertOIDCUser`); test fixtures must burn that slot before creating regular test users

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
