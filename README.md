# goloom

`goloom` is a Go backend for scheduling and publishing social media posts across Bluesky, Friendica, and Mastodon with team-based access control.

## Features

- Provider registry with a `SocialMediaProvider` abstraction for platform-specific behavior.
- Multi-tenant data model for users, teams, team memberships, social accounts, API tokens, and scheduled posts.
- Dynamic validation that computes the effective character limit from the selected destination accounts.
- Worker-based scheduler with retry backoff for failed publishes.
- OIDC-ready authentication flow plus opaque bearer token support for API clients.
- PostgreSQL schema, multi-stage Docker image, Nix flake developer shell, and `Makefile` workflow.

## Development

Enter the development shell:

```bash
nix develop
```

Apply the schema:

```bash
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/goloom?sslmode=disable
make schema
```

Run the app:

```bash
cp .env.example .env
make tidy
make run
```

## REST API

- `GET /healthz`
- `GET /v1/providers`
- `GET /v1/me`
- `GET /v1/teams/{teamID}/accounts`
- `POST /v1/teams/{teamID}/accounts`
- `GET /v1/teams/{teamID}/posts`
- `POST /v1/teams/{teamID}/posts`
- `POST /v1/teams/{teamID}/posts/validate`
- `GET /v1/teams/{teamID}/posts/{postID}`
- `PATCH /v1/teams/{teamID}/posts/{postID}`
- `DELETE /v1/teams/{teamID}/posts/{postID}`
- `POST /v1/teams/{teamID}/posts/{postID}/cancel`

## Notes

- The provider implementations are structured for extension; production deployments should replace the current generic HTTP posting logic with platform-specific API clients and token refresh flows.
- API tokens are stored as hashes, while third-party provider tokens are encrypted before being persisted.
