# internal/domain

## Purpose

Domain models shared across all Go packages. Single source of truth for data structures, patch semantics, template variables, and review queue logic.

## Ownership

Models in `models.go`, patch utilities in `patch_field.go`, template vars in `templatevars.go`.

## Local Contracts

- `ScheduledPost.AIFallbackReason` is a nullable, persisted machine reason for automation template fallback. `CreatePostInput.AIFallbackReason` is server-only (`json:"-"`); user payloads cannot forge fallback provenance. Reasons are `provider_failure`, `empty_content`, `invalid_output`, `validation_failed`, and `dispatch_failed`.

- `CreatePostInput.RSSItemKey` is server-only and identifies an RSS item for atomic post/marker insertion; `MaxRSSItemAttempts` bounds automatic attempts at 3.
- Domain types: `models.go` — User, Team, Post, Account, AIJob, etc.
- Audit actors distinguish `oidc`, `api_token` and `system`. Automation authorship and authenticated initiators live in separate safe metadata; system actors never impersonate tokens.
- `AuthenticatedPrincipal.TokenExpiresAt` carries the authenticating token's expiry for delegation checks; nil means no token expiry.
- Patch semantics: `update_post_patch.go` — partial update rules
- Template variables: `templatevars.go` — post template variable handling
- Review queue: `review_queue.go` — queue domain logic
- Scheduling prefs: `scheduling_prefs.go` — scheduling preferences model

## Work Guidance

- All new domain types go in `models.go`
- Patch fields use `PatchField[T]` generic type
- Template variables must be documented in `templatevars_test.go`
- Follow existing naming conventions (CamelCase types, snake_case SQL columns)
- Avoid importing business logic packages — this package is pure data

## Verification

- `go test ./internal/domain/...` must pass

## Child DOX Index

None — flat package structure.
