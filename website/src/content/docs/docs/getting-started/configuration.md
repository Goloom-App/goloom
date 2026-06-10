---
title: Configuration
description: Environment variables and configuration options for goloom.
---

Start from `.env.example` in the repository root. Common configuration keys:

## Application

| Variable | Description |
| --- | --- |
| `APP_ENV` | Runtime environment (e.g. `development`, `production`) |
| `HTTP_ADDR` | HTTP listen address (default `:8080`) |
| `PUBLIC_BASE_URL` | Public URL used for OAuth callbacks and links |

## Database

| Variable | Description |
| --- | --- |
| `DATABASE_URL` | SQLite file path or PostgreSQL connection string |

Examples:

```bash
# SQLite (default)
DATABASE_URL=file:./data/goloom.db

# PostgreSQL
DATABASE_URL=postgres://postgres:postgres@localhost:5432/goloom?sslmode=disable
```

## Security

| Variable | Description |
| --- | --- |
| `ENCRYPTION_KEY` | Secret used to encrypt provider access tokens at rest |
| `BOOTSTRAP_ADMIN_TOKEN` | One-time token for first admin access |
| `BOOTSTRAP_ADMIN_*` | Additional bootstrap admin settings |

Set a strong `ENCRYPTION_KEY` and rotate bootstrap secrets after initial setup.

## Scheduler

| Variable | Description |
| --- | --- |
| `SCHEDULER_*` | Background job scheduling options |

## Authentication

| Variable | Description |
| --- | --- |
| `OIDC_*` | OpenID Connect settings for browser sign-in |

## Providers

| Variable | Description |
| --- | --- |
| `MASTODON_*` | Mastodon instance and app registration settings |

Provider access tokens are encrypted at rest. API tokens are stored as hashes.
