---
title: Configuration
description: Configure goloom with environment variables — database, secrets, OIDC and providers.
sidebar:
  order: 2
---

goloom is configured entirely through **environment variables**. When building
from source, start from `.env.example` and copy it to `.env`.

```bash
cp .env.example .env
```

## Core

| Variable | Purpose |
| --- | --- |
| `APP_ENV` | Runtime environment (e.g. `development`, `production`). |
| `HTTP_ADDR` | Address/port the server listens on (default `:8080`). |
| `PUBLIC_BASE_URL` | Public URL of your instance — used for OAuth callbacks and links. |
| `DATABASE_URL` | Database connection string (see below). |
| `ENCRYPTION_KEY` | **Required.** Long random secret; encrypts provider tokens at rest. |

:::caution
Treat `ENCRYPTION_KEY` as permanent for a given database. If you lose or change
it, previously encrypted provider tokens can no longer be decrypted.
:::

## Database

SQLite is the zero-config default:

```bash
DATABASE_URL=file:./data/goloom.db
```

Switch to PostgreSQL for larger deployments:

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/goloom?sslmode=disable
```

## Bootstrap admin

| Variable | Purpose |
| --- | --- |
| `BOOTSTRAP_ADMIN_TOKEN` | One-time token to create the first admin. Rotate after setup. |

See [First login](/getting-started/first-login/) for the bootstrap flow.

## Scheduler

`SCHEDULER_*` variables tune how often goloom checks for and publishes due posts.
The defaults are sensible for most deployments.

## OIDC sign-in (optional)

Set the `OIDC_*` variables to let browser users sign in through your identity
provider instead of API tokens.

## Providers

Some providers need app credentials. For Mastodon, `MASTODON_*` variables can
hold default app credentials, though Mastodon onboarding can also auto-register
an app from the instance URL. See [Connect accounts](/guides/accounts/).
