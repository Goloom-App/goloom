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
- AI context: `aicontext.Build(ctx, store, teamID, opts)` is the only place that composes `domain.AIContext` from store primitives (never in a store backend); callers pick parts via `aicontext.Full()` / `aicontext.Chat()` / `Options{}`. Output is always well-formed: slices non-nil, `Profile` nil (no error) when the team has no brand profile — prompt code must fall back to defaults. Posts carry `TargetAccounts` in both backends
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
  `resolver` seam injectable for tests; the public-address test is
  `netguard.IsPublicAddr`); delivery of a batch is bounded by a single `DeliveryTimeout`
  (default 10s) spanning all targets; each target's notification payload
  embeds the user-wide open-review total from `CountUserOpenReviewItems`;
  logs show only the endpoint origin, never the full URL; notification `Topic`
  is `base64.RawURLEncoding(sha256(teamID)[:24])`
- SSRF guard (`internal/netguard`): single source for "is this address
  public" (`IsPublicAddr` denies loopback/private/link-local plus the
  special-use prefixes in `nonPublicPrefixes`, IPv4-mapped forms normalized via
  `Unmap`, NAT64/SIIT/IPv4-compatible IPv6 judged by the embedded IPv4).
  `Policy` = public-only plus admin-allowlisted `host` or `host:port` entries;
  `CheckURL` validates on save, `HTTPClient` guards dial + redirects (DNS
  rebinding) and ignores env proxies. Any outbound call to a user-supplied URL
  must use it
- AI outbound (`internal/ai`): LLM HTTP clients come from
  `ai.NewHTTPClient`/`ai.NewService(allowedPrivateHosts...)` (netguard policy
  from `AI_ALLOWED_PRIVATE_HOSTS`, redirects never followed); never build a
  plain `http.Client` for a team-configured base URL
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
- AI chat write gate (`agenttools`): the chat transport proposes a tool for user confirmation when `Tool.Confirm` is set or its `confirmIf` hook returns true (fail closed); `modify_post` is autonomous only for pure content edits of drafts (no `scheduled_at`/`target_accounts` change). `RunConfirmed` accepts only tools with `Confirm` or `confirmIf`. MCP ignores both. Tool output from external sources (e.g. `fetch_url`) must be wrapped with `ai.WrapUntrusted` before it reaches the model
- AI chat history is client-held and untrusted: `ai.ChatMessagesFromDomain` accepts only `user`/`assistant` roles, folds `AIChatMessage.tool_results` (allowlisted write tools only, sanitized, capped) into the assistant text as `[Tool results: …]`, and `ai.TrimChatHistory` keeps the last 30 messages / 40k chars (limits in `internal/ai/chat.go`), never dropping the latest user turn; every message is capped at 40k chars. The frontend records tool outcomes per assistant message instead of sending full payloads
- The first user in an empty database becomes admin (`UpsertOIDCUser`); test fixtures must burn that slot before creating regular test users
- AI prompts (`ai/prompts.go`): social-media defaults (platform hashtag rules, tone/hook/CTA, language fallback) live only in `ai/defaults.go` and apply only where the brand profile is empty; the task prompt's char limit comes from `AIContext.Accounts` (honors per-account overrides). Truncate user text for prompts/API with `textcount.Truncate` (grapheme-safe, never `s[:n]`)
- Prompt budget (`ai/prompts.go`): knowledge-base section capped at `knowledgeTotalChars` (12000, per source `knowledgeSourceChars` 4000), newest-updated first, with an omitted-count line; style examples newest first (limit 3). Anthropic requests mark the system block and last tool `cache_control: ephemeral` — keep volatile text out of the system-prompt head. Knowledge-source API limits (20000 chars, 50/team) live in `api/ai_validation.go`

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
