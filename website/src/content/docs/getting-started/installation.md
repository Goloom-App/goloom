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

Official multi-arch images (linux/amd64 + arm64) are published to the GitHub
Container Registry on every release:

```bash
docker run --rm \
  -p 8080:8080 \
  -e ENCRYPTION_KEY="$(openssl rand -hex 32)" \
  -e BOOTSTRAP_ADMIN_TOKEN="change-me-please" \
  -v "$(pwd)/data:/app/data" \
  ghcr.io/goloom-app/goloom:latest
```

Then open <http://localhost:8080>.

The `-v` mount keeps your SQLite database and uploaded media on the host so they
survive container restarts.

:::tip
For production, **pin a version tag** (e.g. `ghcr.io/goloom-app/goloom:v0.1.0`)
instead of `:latest` so upgrades are deliberate. You can check which version is
running with `curl http://localhost:8080/healthz`. See
[versioning](#versioning--releases) below.
:::

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

## Versioning & releases

goloom follows [Semantic Versioning](https://semver.org/). It is intentionally
**pre-1.0 (0.x)**: it is usable and self-hostable today, but breaking changes can
still land between minor versions — pin a version tag and read the release notes
before upgrading.

- **Releases** are published on [GitHub](https://github.com/Goloom-App/goloom/releases)
  with a changelog, prebuilt Linux binaries (amd64/arm64) and the GHCR image.
- **The running version** is reported by `GET /healthz` and in the agent
  discovery document.
- **The REST API has its own contract version** under `/v1`, independent of the
  app version; it only changes on breaking API changes.

## Next steps

- [Configuration](/getting-started/configuration/) — environment variables and options.
- [First login](/getting-started/first-login/) — bootstrap your first admin.
