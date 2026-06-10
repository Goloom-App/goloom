# internal/auth

## Purpose

Authentication, authorization, OIDC integration, feature flags, and permission scopes. Security-critical boundary for the entire application.

## Ownership

Auth service in `auth.go`, feature flags in `feature_flags.go`, scopes in `scopes.go`.

## Local Contracts

- Auth service: `auth.go` — token validation, middleware, OIDC, bootstrap
- Feature flags: `feature_flags.go` — runtime feature toggles
- Scopes: `scopes.go` — permission scope definitions
- OIDC JWT: `oidc_jwt_keyset.go` — keyset handling
- Bootstrap token lifecycle for initial admin setup

## Work Guidance

- All API endpoints must check auth middleware
- OIDC flow follows standard authorization code grant
- Feature flags checked before new functionality
- Scopes define granular permissions (read, write, admin)
- Bootstrap token generated on first run, single-use
- JWT keyset refreshed from OIDC provider

## Verification

- `go test ./internal/auth/...` must pass
- Feature flag tests in `feature_flags_test.go`
- Scope tests in `scopes_test.go`

## Child DOX Index

None — flat package structure.
