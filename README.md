# goloom

`goloom` is a Go backend plus frontend workspace for scheduling and publishing social media posts across Bluesky, Friendica, and Mastodon with team-based access control.

## Features

- Provider registry with a `SocialMediaProvider` abstraction for platform-specific behavior.
- Multi-tenant data model for users, teams, team memberships, social accounts, API tokens, and scheduled posts.
- Dynamic validation that computes the effective character limit from the selected destination accounts.
- Worker-based scheduler with retry backoff for failed publishes.
- OIDC-ready authentication flow plus opaque bearer token support for API clients.
- React frontend with month, week, and day calendar views, drag-and-drop scheduling, settings, team management, and an administration surface.
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

Run the frontend:

```bash
make frontend-install
make frontend-dev
```

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

### Agent-oriented examples

Create a Mastodon provider instance registration:

```bash
curl -X POST http://localhost:8080/v1/admin/provider-instances \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "mastodon",
    "name": "mastodon.de",
    "instance_url": "https://mastodon.de",
    "client_id": "client-id-from-app-registration",
    "client_secret": "client-secret-from-app-registration",
    "scopes": ["read", "write"]
  }'
```

Response:

```json
{
  "id": "63f4f5f9-1f73-4f33-8e85-8e6486c6f8f8",
  "provider": "mastodon",
  "name": "mastodon.de",
  "instance_url": "https://mastodon.de",
  "client_id": "client-id-from-app-registration",
  "has_client_secret": true,
  "scopes": ["read", "write"],
  "authorization_endpoint": "https://mastodon.de/oauth/authorize",
  "token_endpoint": "https://mastodon.de/oauth/token"
}
```

Connect a team account against a registered instance:

```bash
curl -X POST http://localhost:8080/v1/teams/$TEAM_ID/accounts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "provider": "mastodon",
    "provider_instance_id": "63f4f5f9-1f73-4f33-8e85-8e6486c6f8f8",
    "access_token": "user-access-token"
  }'
```

Schedule a post:

```bash
curl -X POST http://localhost:8080/v1/teams/$TEAM_ID/posts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Release note",
    "content": "Scheduler retry improvements are live.",
    "scheduled_at": "2026-05-01T10:00:00Z",
    "target_accounts": ["account-id-1", "account-id-2"]
  }'
```

List posts for a team:

```json
{
  "items": [
    {
      "id": "post-id",
      "team_id": "team-id",
      "author_user_id": "user-id",
      "title": "Release note",
      "content": "Scheduler retry improvements are live.",
      "scheduled_at": "2026-05-01T10:00:00Z",
      "status": "posted",
      "target_accounts": ["account-id-1"],
      "published_links": {
        "account-id-1": "https://mastodon.de/@goloom/1144"
      }
    }
  ]
}
```

## Frontend

The `frontend` app is now wired to the Go API instead of relying only on mock state.

- Settings includes the backend API base URL and bearer token used for requests.
- Teams, memberships, accounts, posts, archive links, provider instances, and user roles are loaded from the backend.
- The team account onboarding flow uses registered provider instances instead of freeform instance URLs.
- The admin view lets administrators register and update provider instances and client credentials.
- The settings screen shows a live runtime configuration snapshot for admin users.

## Notes

- The provider implementations are structured for extension; production deployments should replace the current generic HTTP posting logic with platform-specific API clients and token refresh flows.
- API tokens are stored as hashes, while third-party provider tokens are encrypted before being persisted.
- The schema now includes `provider_instances`, `users.is_admin`, `teams.description`, `social_accounts.provider_instance_id`, and `scheduled_posts.title`. Re-run `make schema` against existing databases before starting the app.
