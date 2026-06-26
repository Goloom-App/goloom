---
title: Installation
description: Install and run goloom — via Docker or from source — as a single Go binary.
sidebar:
  order: 1
---

goloom ships as a **single Go binary** that serves both the web UI and the REST
API. The fastest way to try it is Docker; you can also build from source.

## Requirements

- A host that can run a container **or** the Go toolchain (Go 1.22+).
- Two secrets you generate yourself:
  - `ENCRYPTION_KEY` — used to encrypt provider tokens at rest.
  - `BOOTSTRAP_ADMIN_TOKEN` — used once to bootstrap the first admin.

By default goloom stores data in an embedded **SQLite** file, so no external
database is required.

## Run with Docker

```bash
docker run --rm \
  -p 8080:8080 \
  -e ENCRYPTION_KEY="$(openssl rand -hex 32)" \
  -e BOOTSTRAP_ADMIN_TOKEN="change-me-please" \
  -v "$(pwd)/data:/app/data" \
  goloom
```

Then open <http://localhost:8080>.

The `-v` mount keeps your SQLite database and uploaded media on the host so they
survive container restarts.

## Build from source

```bash
git clone https://github.com/Goloom-App/goloom.git
cd goloom

cp .env.example .env
# edit .env and set ENCRYPTION_KEY and BOOTSTRAP_ADMIN_TOKEN

make build
./bin/goloom
```

`make build` compiles the React frontend and embeds it into the Go binary, then
builds `bin/goloom`. Open <http://localhost:8080> when it starts.

## Next steps

- [Configuration](/getting-started/configuration/) — environment variables and options.
- [First login](/getting-started/first-login/) — bootstrap your first admin.
