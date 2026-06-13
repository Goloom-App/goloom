# goloom

Lightweight social planning for teams and AI agents.

`goloom` is a self-hosted social media planning application built as one Go binary (API + web UI). It helps teams plan posts across multiple social accounts without the heavy infrastructure and pricing model of typical enterprise-first tools.

## Why goloom exists

Most tools in this space are powerful but heavy: complex stacks, many paid tiers, and product scope optimized for large organizations.

goloom was created for one focused outcome:

- plan posts across different social media accounts
- collaborate across teams
- keep operations simple
- integrate cleanly with AI agents such as OpenClaw

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

- Built-in **MCP server** at `/mcp/` (enabled by default, `MCP_ENABLED`), authenticated with API tokens carrying `ai:read:context` / `ai:write:drafts` scopes.
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

Deployed via Forgejo Pages on push to `main` when `website/` or `docs/api/` changes. Site URL: [https://git.f4mily.net/goloom](https://git.f4mily.net/goloom) (Forgejo Pages).

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

Build:

```bash
docker build -t goloom .
```

Run:

```bash
docker run --rm \
  -p 8080:8080 \
  -e ENCRYPTION_KEY=replace-with-a-long-random-secret \
  -e BOOTSTRAP_ADMIN_TOKEN=replace-with-a-strong-bootstrap-token \
  -v "$(pwd)/data:/app/data" \
  goloom
```

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

## License

See repository license file.
