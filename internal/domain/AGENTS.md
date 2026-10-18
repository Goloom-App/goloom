# internal/domain

## Purpose

Domain models shared across all Go packages. Single source of truth for data structures, patch semantics, template variables, and review queue logic.

## Ownership

Models in `models.go`, patch utilities in `patch_field.go`, template vars in `templatevars.go`.

## Local Contracts

- `User.PictureURL` is the platform-user picture from OIDC, serialized as `picture_url`; connected social-account avatars remain separate.
- `ScheduledPost.AIFallbackReason` is a nullable, persisted machine reason for automation template fallback or RSS article-fetch warning. `ScheduledPost.AIJobID` identifies the related automation job; `CreatePostInput.AIJobID` and `CreatePostInput.AIFallbackReason` are server-only (`json:"-"`); user payloads cannot forge provenance. Reasons are `provider_failure`, `empty_content`, `invalid_output`, `validation_failed`, `dispatch_failed`, and `article_fetch_failed`.

- `CreatePostInput.RSSItemKey` is server-only and identifies an RSS item for atomic post/marker insertion; `MaxRSSItemAttempts` bounds automatic attempts at 3.
- `ErrRSSDailyLimit` identifies a rejected RSS capacity claim; schedulers skip the item and log only its hashed key.
- `RSSFeedConfig.ExhaustedItemsCount` exposes exhausted unlinked RSS attempts in feed JSON; active jobs and fresh reservations are excluded.
- `RSSFeedConfig.ArticleEnrichmentEnabled` is an opt-in boolean, default false, persisted in both stores and exposed as `article_enrichment_enabled`.
- `ScheduledPost.AIFallbackReason=article_fetch_failed` is a source warning after article retrieval failed while AI used feed content; it is not a template fallback.
- Domain types: `models.go` — User, Team, Post, Account, AIJob, etc.
- Audit actors distinguish `oidc`, `api_token` and `system`. Automation authorship and authenticated initiators live in separate safe metadata; system actors never impersonate tokens.
- AI job provenance is server-owned: `AIJobOriginUser` jobs follow normal completion, `AIJobOriginAutomation` jobs may run RSS/recurring callbacks, and `AIJobOriginPreview` jobs retain generated results without creating posts or changing automation state.
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
