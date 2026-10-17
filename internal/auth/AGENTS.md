# internal/auth

## Purpose

Authentication, authorization, OIDC integration, feature flags, and permission scopes. Security-critical boundary for the entire application.

## Ownership

Auth service in `auth.go`, feature flags in `feature_flags.go`, scopes in `scopes.go`.

## Local Contracts

- Auth service: `auth.go` — token validation, middleware, OIDC, bootstrap
- `RequireAdmin` requires both an admin user and full global token authority (no scopes or team binding), for every HTTP method. Browser sessions retain access.
- Feature flags: `feature_flags.go` — runtime feature toggles
- Scopes: `scopes.go` — permission scope definitions; `PrincipalAllows` enforces explicit scopes regardless of `Kind` (legacy API tokens named like browser sessions are classified as OIDC). Callers without scopes, including browser sessions, retain full authority
- `PrincipalCanManageAPIToken` governs API-token delegation, revocation, and listing visibility: callers with scopes or a team binding are restricted regardless of `Kind`; normalized target scopes must satisfy the scope hierarchy, an empty target scope list requires an unscoped caller, and team bindings must match exactly (including nil). Callers without restrictions bypass this check.
- Restricted callers with `TokenExpiresAt` may create tokens only with expiry at or before that time, including the resolved 90-day default. Later expiry returns `api_token_exceeds_authority` (403); callers without expiry retain existing lifetime rules. Expiry does not constrain revocation.
- OIDC JWT: `oidc_jwt_keyset.go` — keyset handling
- Bootstrap token lifecycle for initial admin setup
- Token-to-browser-session exchange requires empty scopes and no team binding, regardless of user admin status or token name. Bootstrap/recovery tokens are ordinary full-authority API tokens and pass this check without an exemption.

## Work Guidance

- All API endpoints must check auth middleware
- OIDC flow follows standard authorization code grant
- Feature flags checked before new functionality
- Scopes define granular permissions (read, write, admin)
- Post updates enforce scope against the patch's TARGET state: a patch that publishes or schedules (moves the post out of draft) requires `write:schedule`, even when the post currently sits in draft as `write:draft`; a plain draft-save with `draft: true` stays on `write:draft`. The target state is derived before any `publish_now` resolution.
- Bootstrap/recovery token is generated on first run or supplied through config; session exchange does not consume it
- JWT keyset refreshed from OIDC provider

## Verification

- `go test ./internal/auth/...` must pass
- Feature flag tests in `feature_flags_test.go`
- Scope tests in `scopes_test.go`

## Child DOX Index

None — flat package structure.
