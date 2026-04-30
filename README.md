# goloom

`goloom` is now a single Go application binary that serves both the API and the React frontend. By default it stores data in SQLite for zero-dependency deployments, and it can still use PostgreSQL when `DATABASE_URL` points at a Postgres instance.

## Highlights

- Single binary deployment: one process serves the UI and the API.
- SQLite by default: no external database required.
- PostgreSQL supported: set `DATABASE_URL=postgres://...` or `postgresql://...`.
- Embedded frontend assets: production builds do not need a separate Node container.
- Optional bootstrap admin token for first startup.
- Mastodon instance registration can auto-create app credentials from just the instance URL.
- Existing provider registry, scheduler, team model, OIDC support, and bearer-token API auth remain available.

## Quick Start

Copy the example environment file:

```bash
cp .env.example .env
```

Set at least these values:

```bash
ENCRYPTION_KEY=replace-with-a-long-random-secret
BOOTSTRAP_ADMIN_TOKEN=replace-with-a-strong-bootstrap-token
```

Build and run:

```bash
make build
./bin/goloom
```

Then open [http://localhost:8080](http://localhost:8080) and use the bootstrap token in the Settings screen. If you leave the API base URL empty, the frontend talks to the same server automatically.

## Provider Onboarding

### Mastodon

Mastodon provider instances can be registered automatically from:

- instance name
- instance URL

The backend will call the target instance's `/api/v1/apps` endpoint, store the returned `client_id` and `client_secret`, discover the authorization/token endpoints automatically, and use them for the browser-based OAuth authorization flow when a team connects an account.

Optional backend configuration:

```bash
MASTODON_APP_NAME=goloom
MASTODON_REDIRECT_URI=http://localhost:8080/v1/oauth/mastodon/callback
MASTODON_WEBSITE=
MASTODON_DEFAULT_SCOPES=read,write
```

### Friendica

Friendica does not have the same portable automatic app registration flow here. If your instance provides OAuth app credentials, enter them manually in the admin provider-instance form.

### Bluesky

For the current onboarding flow, Bluesky does not need stored client credentials. Register the PDS endpoint and then connect accounts with an app password.

## Database Configuration

### Default SQLite

This is the default when `DATABASE_URL` is unset:

```bash
DATABASE_URL=file:./data/goloom.db
```

The app creates the SQLite database and schema automatically at startup.

### PostgreSQL

To use PostgreSQL instead:

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/goloom?sslmode=disable
```

The app also applies the embedded Postgres schema automatically on startup. The `make schema` target is still available if you want to apply `db/schema.sql` manually.

## Development

Enter the development shell:

```bash
nix develop
```

Run the single app locally:

```bash
make run
```

Run the frontend dev server separately:

```bash
make frontend-dev
```

The Vite dev server proxies `/v1` and `/healthz` to `http://localhost:8080`, so the browser can keep using same-origin API paths during development.

## Docker

Build the production image:

```bash
docker build -t goloom .
```

Run with the default SQLite setup:

```bash
docker run --rm \
  -p 8080:8080 \
  -e ENCRYPTION_KEY=replace-with-a-long-random-secret \
  -e BOOTSTRAP_ADMIN_TOKEN=replace-with-a-strong-bootstrap-token \
  -v "$(pwd)/data:/app/data" \
  goloom
```

Use `docker compose up -d app` for the SQLite deployment.

If you want PostgreSQL in Compose, start the profiled services explicitly:

```bash
docker compose --profile postgres up -d db app-postgres
```

That exposes the Postgres-backed app on [http://localhost:8081](http://localhost:8081).

## Authentication Bootstrap

For fresh deployments without OIDC, set:

```bash
BOOTSTRAP_ADMIN_EMAIL=admin@localhost
BOOTSTRAP_ADMIN_NAME=Local Administrator
BOOTSTRAP_ADMIN_TOKEN=replace-with-a-strong-bootstrap-token
```

On startup the app ensures that an admin user and hashed API token exist for that bootstrap identity. You can later rotate away from the bootstrap token or switch to OIDC.

## REST API

All authenticated endpoints expect:

```http
Authorization: Bearer <oidc-id-token-or-api-token>
```

### Discovery

- `GET /healthz`
- `GET /v1/providers`
- `GET /v1/me`
- `GET /v1/users`
- `GET /v1/teams`
- `POST /v1/teams`
- `GET /v1/provider-instances`
- `GET /v1/provider-instances/{instanceID}`

### Team management

- `GET /v1/teams/{teamID}/members`
- `POST /v1/teams/{teamID}/members`
- `DELETE /v1/teams/{teamID}/members/{userID}`
- `GET /v1/teams/{teamID}/accounts`
- `POST /v1/teams/{teamID}/accounts/oauth/mastodon/start`
- `POST /v1/teams/{teamID}/accounts`
- `DELETE /v1/teams/{teamID}/accounts/{accountID}`
- `GET /v1/teams/{teamID}/posts`
- `POST /v1/teams/{teamID}/posts`
- `POST /v1/teams/{teamID}/posts/validate`
- `GET /v1/teams/{teamID}/posts/{postID}`
- `PATCH /v1/teams/{teamID}/posts/{postID}`
- `DELETE /v1/teams/{teamID}/posts/{postID}`
- `POST /v1/teams/{teamID}/posts/{postID}/cancel`

### Admin endpoints

- `GET /v1/admin/users`
- `PATCH /v1/admin/users/{userID}`
- `GET /v1/admin/runtime-config`
- `GET /v1/admin/provider-instances`
- `POST /v1/admin/provider-instances`
- `PUT /v1/admin/provider-instances/{instanceID}`

### OAuth callback

- `GET /v1/oauth/mastodon/callback`

## Notes

- Provider tokens are encrypted before persistence, while API tokens are stored as hashes.
- The provider implementations are structured for extension; production deployments should replace the current generic HTTP posting logic with provider-specific refresh and publishing flows where needed.
