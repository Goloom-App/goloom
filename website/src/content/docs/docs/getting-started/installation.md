---
title: Installation
description: Configure, build, and run goloom locally or with Docker.
---

## Configure environment

Copy the example environment file and set required secrets:

```bash
cp .env.example .env
```

Set required values:

```bash
ENCRYPTION_KEY=replace-with-a-long-random-secret
BOOTSTRAP_ADMIN_TOKEN=replace-with-a-strong-bootstrap-token
```

See [Configuration](/docs/getting-started/configuration/) for all available options.

## Build and run

From the repository root:

```bash
make build
./bin/goloom
```

Open [http://localhost:8080](http://localhost:8080).

For development with hot reload on the frontend:

```bash
nix develop
make run
```

Frontend-only dev server:

```bash
make frontend-dev
```

## Docker

Build the image:

```bash
docker build -t goloom .
```

Run a container:

```bash
docker run --rm \
  -p 8080:8080 \
  -e ENCRYPTION_KEY=replace-with-a-long-random-secret \
  -e BOOTSTRAP_ADMIN_TOKEN=replace-with-a-strong-bootstrap-token \
  -v "$(pwd)/data:/app/data" \
  goloom
```

## Database options

**SQLite (default)** — best for low-ops environments and small-to-medium teams:

```bash
DATABASE_URL=file:./data/goloom.db
```

**PostgreSQL** — for external DB operations and scale patterns:

```bash
DATABASE_URL=postgres://postgres:postgres@localhost:5432/goloom?sslmode=disable
```

## Next steps

After installation, complete [first login](/docs/getting-started/first-login/) to bootstrap admin access.
