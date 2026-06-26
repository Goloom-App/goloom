<div align="center">

<img src="frontend/public/icon-192.png" width="96" alt="goloom logo" />

# goloom

**Lightweight, self-hosted social media planning for teams and AI agents.**

Plan posts across Mastodon, Bluesky and Friendica from a single Go binary — no enterprise stack, no per-seat pricing.

<!-- Build & tech -->
[![Deploy website](https://github.com/Goloom-App/goloom/actions/workflows/pages.yml/badge.svg)](https://github.com/Goloom-App/goloom/actions/workflows/pages.yml)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go&logoColor=white)](go.mod)
[![TypeScript](https://img.shields.io/badge/TypeScript-6.0-3178C6?style=flat&logo=typescript&logoColor=white)](frontend)
[![Single binary](https://img.shields.io/badge/deploy-single%20binary-7c3aed?style=flat)](#deployment-options)
[![Self-hosted](https://img.shields.io/badge/self--hosted-yes-22c55e?style=flat)](#docker)
[![API-first](https://img.shields.io/badge/API-first-0ea5e9?style=flat&logo=openapiinitiative&logoColor=white)](#api-documentation)
[![E2E: Playwright](https://img.shields.io/badge/E2E-Playwright-2EAD33?style=flat&logo=playwright&logoColor=white)](frontend/playwright.config.ts)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat)](LICENSE)

<!-- Repository activity -->
[![Last commit](https://img.shields.io/github/last-commit/Goloom-App/goloom?style=flat)](https://github.com/Goloom-App/goloom/commits/main)
[![Open issues](https://img.shields.io/github/issues/Goloom-App/goloom?style=flat)](https://github.com/Goloom-App/goloom/issues)
[![Open PRs](https://img.shields.io/github/issues-pr/Goloom-App/goloom?style=flat)](https://github.com/Goloom-App/goloom/pulls)
[![Stars](https://img.shields.io/github/stars/Goloom-App/goloom?style=flat)](https://github.com/Goloom-App/goloom/stargazers)

<br />

<img src="website/src/assets/screenshots/content-calendar.png" alt="goloom content calendar" width="820" />

</div>

---

`goloom` is a self-hosted social media planning application built as one Go binary (API + web UI). It helps teams plan posts across multiple social accounts without the heavy infrastructure and pricing model of typical enterprise-first tools.

## Why goloom exists

Most tools in this space are powerful but heavy: complex stacks, many paid tiers, and product scope optimized for large organizations.

goloom was created for one focused outcome:

- plan posts across different social media accounts
- collaborate across teams
- keep operations simple
- integrate cleanly with AI agents such as OpenClaw

## Screenshots

<table>
  <tr>
    <td width="50%"><img src="website/src/assets/screenshots/composer.png" alt="Post composer with live previews" /></td>
    <td width="50%"><img src="website/src/assets/screenshots/analytics.png" alt="Team performance analytics" /></td>
  </tr>
  <tr>
    <td align="center"><strong>Composer</strong> — write once, preview per platform</td>
    <td align="center"><strong>Analytics</strong> — engagement & follower trends</td>
  </tr>
</table>

## Highlights

- Single binary deployment: UI and API in one process.
- SQLite by default: no external database needed.
- PostgreSQL optional for larger deployments.
- Team workspaces with member roles (`owner`, `editor`, `viewer`).
- Scheduling, validation, per-account post versions, media library.
- Built-in analytics for post and account metrics.
- API-first architecture with bearer-token auth.
- OIDC support for browser sign-in.
- Mastodon onboarding can auto-register app credentials from instance URL.

## Getting Started

Three ways to run goloom — pick one (full guide:
[Installation](https://goloom-app.github.io/getting-started/installation/)):

- **Docker** — `docker run ... ghcr.io/goloom-app/goloom:latest` (see [Docker](#docker)).
- **Prebuilt binary** — download a static Linux binary from the
  [latest release](https://github.com/Goloom-App/goloom/releases/latest)
  (`goloom_<version>_linux_amd64` / `arm64`), `chmod +x`, set `ENCRYPTION_KEY` +
  `BOOTSTRAP_ADMIN_TOKEN` and run it. The web UI is embedded; data goes to
  `./data/goloom.db` (SQLite) by default.
- **From source** — build it yourself (below).

### 1) Configure environment

```bash
cp .env.example .env
```

Set required values:

```bash
ENCRYPTION_KEY=replace-with-a-long-random-secret
BOOTSTRAP_ADMIN_TOKEN=replace-with-a-strong-bootstrap-token
```

### 2) Build and run

```bash
make build
./bin/goloom
```

Open [http://localhost:8080](http://localhost:8080).

### 3) Bootstrap first admin access

Use the bootstrap token in the UI Settings screen. After first login, create normal API tokens and rotate bootstrap secrets.

## API Documentation

goloom API is designed for both developers and AI agents.
Professional documentation stack uses OpenAPI + Redocly.

### Base paths

- Primary: `/v1/...`
- Alias: `/api/v1/...` (same handlers, for tools expecting `/api/v1`)

### Authentication

Use bearer tokens:

```http
Authorization: Bearer <oidc-id-token-or-api-token>
```

### API quickstart (curl)

Health and auth status:

```bash
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/v1/auth/status
```

List providers:

```bash
curl -s http://localhost:8080/v1/providers
```

Get current identity:

```bash
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/v1/me
```

### Endpoint groups

- Discovery: `/healthz`, `/v1/providers`, `/v1/auth/status`
- Identity: `/v1/me`, `/v1/me/api-tokens`
- Teams: `/v1/teams`, `/v1/teams/{teamID}/members`
- Accounts: `/v1/teams/{teamID}/accounts`, OAuth start endpoints
- Posts: `/v1/teams/{teamID}/posts`, validation, versions, cancel
- Analytics: `/v1/teams/{teamID}/analytics*`, post analytics
- Admin: `/v1/admin/*`, provider instance management

For complete route list, see `api/http.go`.

### API docs

The interactive API reference is rendered with [Scalar](https://scalar.com) and
served by the website at `/api/`. It reads the OpenAPI spec directly, so there is
no separate static build step.

Lint the OpenAPI spec:

```bash
make docs-api-lint
```

- source spec: `docs/api/openapi.yaml` (single source of truth)
- the spec is copied to `website/public/openapi.yaml` by `make website-build`

### AI agent integration notes (OpenClaw and similar)

- Built-in **MCP server** (Streamable HTTP transport) at `/mcp` (enabled by default, `MCP_ENABLED`), authenticated with API tokens carrying `read` / `write:draft` / `write:schedule` / `write` / `delete` scopes (unscoped tokens have full access).
- Stable JSON responses across core endpoints.
- Predictable resource paths with team-scoped objects.
- Validation endpoint before scheduling: `POST /v1/teams/{teamID}/posts/validate`.
- API token lifecycle endpoints for secure agent onboarding.

## Website & Documentation

The project website (Astro + Starlight) lives in `website/`: a marketing landing
page, the documentation (3-column Starlight layout) and the Scalar API reference at `/api/`.

Local dev server:

```bash
make website-dev
```

Build static site (includes API docs):

```bash
make website-build
```

Generated output: `website/dist/`

Deployed via GitHub Pages on push to `main` when `website/` or `docs/api/`
changes: the [`pages.yml`](.github/workflows/pages.yml) workflow builds the site
with `make website-build` and force-pushes `website/dist` to the org Pages repo
[`goloom-app.github.io`](https://github.com/Goloom-App/goloom-app.github.io),
which GitHub serves at the root URL
[https://goloom-app.github.io/](https://goloom-app.github.io/).

## Provider Support

### Mastodon

- OAuth account connection
- optional automatic app registration via instance URL
- publishing and metrics (`likes`, `reposts`, `replies`)

### Friendica

- manual provider app credentials
- publishing and Mastodon-compatible metrics

### Bluesky

- account connection with app password
- publishing and metrics support

## Deployment Options

### Default (single binary + SQLite)

Best for low-ops environments and small-to-medium teams.

```bash
DATABASE_URL=file:./data/goloom.db
```

### PostgreSQL

Use when you need external DB operations and scale patterns:

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/goloom?sslmode=disable
```

## Production migration (Docker → Kubernetes)

If you run Goloom with Docker PostgreSQL and want to move to the homelab CNPG deployment, see [`docs/migrations/docker-to-kubernetes-homelab.md`](docs/migrations/docker-to-kubernetes-homelab.md).

## Docker

Use the published multi-arch image (linux/amd64 + arm64) from GHCR:

```bash
docker run --rm \
  -p 8080:8080 \
  -e ENCRYPTION_KEY=replace-with-a-long-random-secret \
  -e BOOTSTRAP_ADMIN_TOKEN=replace-with-a-strong-bootstrap-token \
  -v "$(pwd)/data:/app/data" \
  ghcr.io/goloom-app/goloom:latest
```

For production, pin a version tag (e.g. `ghcr.io/goloom-app/goloom:v0.1.0`)
instead of `:latest`. To build the image yourself: `docker build -t goloom .`.

## Development

```bash
nix develop
make run
```

Frontend-only dev server:

```bash
make frontend-dev
```

Recommended API-doc workflow in CI:

- run `make docs-api-lint` on pull requests to validate the OpenAPI spec
- the reference is published as part of the website (`make website-build`)

## Configuration

Start from `.env.example`. Common keys:

- `APP_ENV`, `HTTP_ADDR`, `PUBLIC_BASE_URL`
- `DATABASE_URL`
- `ENCRYPTION_KEY`
- `BOOTSTRAP_ADMIN_*`
- `SCHEDULER_*`
- `OIDC_*`
- `MASTODON_*`

## Positioning vs heavy platforms

goloom is intentionally optimized for:

- lower runtime overhead
- easier self-hosting
- practical team collaboration
- API-first automation for agent workflows

If you need broad enterprise suites, many commercial upsell modules, or advanced campaign ecosystems, other products may fit better. If you need a focused scheduler with strong API ergonomics and simple ops, goloom is the target shape.

## Security notes

- Provider access tokens are encrypted at rest.
- API tokens are stored as hashes.
- Set strong `ENCRYPTION_KEY` and rotate bootstrap/admin secrets after setup.

## Versioning & releases

goloom follows [Semantic Versioning](https://semver.org/) and is intentionally
**pre-1.0 (0.x)** — usable and self-hostable today, but breaking changes can
still land between minor versions, so pin a version and read the notes before
upgrading.

- Releases are automated from [Conventional Commits](https://www.conventionalcommits.org/)
  via [release-please](https://github.com/googleapis/release-please): a release PR
  maintains `CHANGELOG.md` and the version; merging it tags `vX.Y.Z`, publishes a
  [GitHub Release](https://github.com/Goloom-App/goloom/releases) with prebuilt
  Linux binaries (amd64/arm64), and pushes the `ghcr.io/goloom-app/goloom` image.
- The running version is reported by `GET /healthz` and the agent discovery doc.
- The REST API has its own contract version under `/v1`, independent of the app
  version; it changes only on breaking API changes.

## About this project

goloom grew out of my own needs as a self-hosted tool. It is built with heavy use
of AI, and I use it as a testbed for exploring methods for efficient, AI-assisted
development.

## License

Licensed under the [MIT License](LICENSE).
