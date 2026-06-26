---
title: Installation
description: Install and run goloom — via Docker or from source — as a single Go binary.
sidebar:
  order: 1
---

goloom ships as a **single Go binary** that serves both the web UI and the REST
API. The fastest way to try it is Docker; you can also download a prebuilt binary
or build from source.

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

## Run a prebuilt binary

Every release ships **static Linux binaries** with the web UI embedded — no Go
toolchain, no Node, no external assets. Pick the one for your CPU from the
[latest release](https://github.com/Goloom-App/goloom/releases/latest):

- `goloom_<version>_linux_amd64` — most servers and PCs (Intel/AMD).
- `goloom_<version>_linux_arm64` — Raspberry Pi, ARM servers, Apple Silicon.

```bash
# 1. Download the binary for your release + arch (e.g. v0.1.1 / amd64)
ver=v0.1.1
base="https://github.com/Goloom-App/goloom/releases/download/$ver"
curl -fsSL -O "$base/goloom_${ver}_linux_amd64"

# 2. (optional) verify the download against the published checksums
curl -fsSL -O "$base/checksums.txt"
sha256sum --check --ignore-missing checksums.txt

# 3. Make it executable and give it a short name
chmod +x "goloom_${ver}_linux_amd64"
mv "goloom_${ver}_linux_amd64" goloom

# 4. Set the two required secrets and run
mkdir -p data
export ENCRYPTION_KEY="$(openssl rand -hex 32)"   # keep this stable — it decrypts stored tokens
export BOOTSTRAP_ADMIN_TOKEN="change-me-please"   # used once to create the first admin
./goloom
```

Then open <http://localhost:8080> and continue with
[first login](/getting-started/first-login/).

By default goloom listens on `:8080` and stores everything in a local **SQLite**
database at `./data/goloom.db` (relative to where you start it) — back up that
`data/` folder. Configuration is read from **environment variables** (there is no
auto-loaded `.env`), so `export` them — or run behind a service manager such as
**systemd** that injects them. See [configuration](/getting-started/configuration/)
for every option (port, PostgreSQL, OIDC, providers …).

:::caution
Keep `ENCRYPTION_KEY` **stable and backed up**. It encrypts your stored provider
tokens at rest; if it changes, goloom can no longer decrypt them and connected
accounts must be re-linked.
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
