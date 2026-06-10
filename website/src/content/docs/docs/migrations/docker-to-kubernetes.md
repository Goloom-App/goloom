---
title: Docker to Kubernetes
description: Move a production Goloom instance from Docker PostgreSQL to a Kubernetes CNPG deployment.
---

This guide summarizes moving a **production** Goloom deployment from Docker Compose (PostgreSQL + local `data/`) to a Kubernetes cluster using CloudNativePG (CNPG). It keeps the essential data, secrets, and cutover steps; homelab-specific GitOps details live in a separate repository.

For the full operator runbook with exact commands and GitOps paths, see the repository source: [`docs/migrations/docker-to-kubernetes-homelab.md`](https://git.f4mily.net/goloom/src/branch/main/docs/migrations/docker-to-kubernetes-homelab.md).

## Overview

| | Docker (source) | Kubernetes (target) |
|--|-----------------|---------------------|
| Database | PostgreSQL container | CNPG cluster, database `goloom` |
| Media files | Docker volume → `data/media/{team_id}/{sha256}` | Persistent volume at `/app/data` |
| Public URL | Traefik + `PUBLIC_BASE_URL` | Ingress hostname |
| Secrets | `.env` file | Kubernetes secrets (e.g. SOPS) |

**Important:** Do not rely on an empty CNPG database plus Goloom’s embedded schema bootstrap alone if Docker was initialized from a different schema snapshot. Migrate with `pg_dump` / `pg_restore` so all tables and data match.

## Prerequisites

- Target CNPG cluster healthy with backups configured.
- `kubectl` access to the Kubernetes cluster.
- Maintenance window — stop writes during export or pick a quiet period.
- **Persistent volume for `/app/data`** mounted in the Goloom Deployment before cutover (media is ephemeral without it).

## Phase 1 — Secrets and URLs

### Encryption key

Provider tokens are encrypted with `ENCRYPTION_KEY`. The value in Kubernetes **must match** your Docker `.env` value, or connected accounts cannot be decrypted after migration.

Also align `BOOTSTRAP_ADMIN_TOKEN` if you still use bootstrap login.

### Public URL and OAuth

Update these for the new hostname:

- `PUBLIC_BASE_URL`
- `ALLOWED_ORIGINS`
- `MASTODON_REDIRECT_URI` (and Mastodon application settings on each instance)

Reconfigure `OIDC_*` after cutover if you use browser sign-in.

### Media volume

Mount persistent storage at `/app/data` in the Deployment. Media files live at:

```text
/app/data/media/{team_id}/{sha256}
```

## Phase 2 — Export database (Docker)

Stop the app container to avoid writes:

```bash
docker compose stop app
```

Custom-format dump (recommended):

```bash
docker exec -t goloom-db pg_dump -U postgres -Fc goloom > /backup/goloom.dump
```

Plain SQL alternative:

```bash
docker exec -t goloom-db pg_dump -U postgres goloom > /backup/goloom.sql
```

## Phase 3 — Import database (CNPG)

Port-forward the CNPG primary service, then restore with credentials from the cluster secret.

Custom-format restore:

```bash
pg_restore -h localhost -U "$PGUSER" -d goloom --clean --if-exists /backup/goloom.dump
```

Restart the Goloom Deployment and verify logs — errors like `relation "job_locks" does not exist` usually mean an incomplete restore rather than a full dump.

## Phase 4 — Media files

Copy `data/media` from the Docker volume to the Kubernetes PVC:

```bash
docker run --rm -v goloom_data:/from -v /backup/goloom-media:/to alpine \
  cp -a /from/media /to/media
```

Copy into the pod volume with `kubectl cp` or a one-off Job. Confirm database rows reference hashes that exist on disk.

## Phase 5 — Cutover

1. Stop the Docker stack or remove the old Traefik route.
2. Point DNS to the cluster ingress if using a public hostname.
3. Run smoke tests:
   - `GET /healthz`
   - Login (OIDC or bootstrap token)
   - Teams, scheduled posts, media library previews
   - Mastodon OAuth (redirect URL matches `PUBLIC_BASE_URL`)
   - Scheduler processing without repeated DB errors

## Phase 6 — Post-cutover

- Rotate `BOOTSTRAP_ADMIN_TOKEN` after first admin login if reused.
- Confirm database backups are scheduled in the cluster.
- Update OAuth redirect URLs on Mastodon and other providers.
- Archive Docker volumes only after a successful validation period.

## Rollback

1. Scale Kubernetes Goloom to zero or suspend the GitOps deployment.
2. Start Docker Compose with unchanged volumes.
3. Restore DNS to the Docker endpoint.

CNPG and Docker data remain until you delete them, so you can retry the migration.

## Related

- [Installation](/docs/getting-started/installation/) — single-binary and Docker defaults
- [Configuration](/docs/getting-started/configuration/) — `DATABASE_URL`, `ENCRYPTION_KEY`, URLs
