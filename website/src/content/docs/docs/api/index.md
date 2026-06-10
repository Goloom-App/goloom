---
title: API overview
description: REST API base paths, authentication, quickstart examples, and agent integration.
---

The goloom API is designed for developers and AI agents. It returns stable JSON across core endpoints and uses predictable, team-scoped resource paths.

For the full interactive reference, open the [API reference](/docs/api-reference/index.html) (Redoc, generated from the OpenAPI spec).

## Base paths

Both prefixes reach the same handlers:

| Prefix | Example |
|--------|---------|
| `/v1/...` | Primary |
| `/api/v1/...` | Alias for tools that expect `/api/v1` |

## Authentication

Send a bearer token on protected routes:

```http
Authorization: Bearer <oidc-id-token-or-api-token>
```

Create long-lived **API tokens** in the UI under **Settings** after [first login](/docs/getting-started/first-login/). OIDC ID tokens from browser sign-in also work when OIDC is configured.

## Quickstart

Health and auth status (no token required):

```bash
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/v1/auth/status
```

List supported providers:

```bash
curl -s http://localhost:8080/v1/providers
```

Current identity:

```bash
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/v1/me
```

## Endpoint groups

| Area | Paths |
|------|-------|
| Discovery | `/healthz`, `/v1/providers`, `/v1/auth/status` |
| Identity | `/v1/me`, `/v1/me/api-tokens` |
| Teams | `/v1/teams`, `/v1/teams/{teamID}/members` |
| Accounts | `/v1/teams/{teamID}/accounts`, OAuth start endpoints |
| Posts | `/v1/teams/{teamID}/posts`, validation, versions, cancel |
| Analytics | `/v1/teams/{teamID}/analytics*`, post analytics |
| Admin | `/v1/admin/*`, provider instance management |

Validate a draft before scheduling:

```bash
curl -s -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"destinations":[...]}' \
  http://localhost:8080/v1/teams/{teamID}/posts/validate
```

## AI agent integration

Patterns that work well for agents (OpenClaw and similar):

- **Stable JSON** responses on list and detail endpoints.
- **Team-scoped IDs** — always pass the workspace team ID in the path.
- **Validate before schedule** — `POST /v1/teams/{teamID}/posts/validate` catches character limits and missing fields early.
- **API token lifecycle** — create and revoke tokens via `/v1/me/api-tokens` for secure onboarding without sharing passwords.
- **Accept-Language** — optional; API errors can be localized when handlers use keyed messages.

## OpenAPI source

The machine-readable spec lives in the repository at `docs/api/openapi.yaml`. Maintainers can lint and rebuild docs with:

```bash
make docs-api-lint
make docs-api-build
```

## Next steps

- [Interactive API reference](/docs/api-reference/index.html) — every route, schema, and example
- [Teams](/docs/guides/teams/) — workspace roles and team IDs
- [Configuration](/docs/getting-started/configuration/) — `PUBLIC_BASE_URL`, OIDC, and provider settings
