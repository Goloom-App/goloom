# Migration: Docker (PostgreSQL) → Kubernetes (CNPG)

Guide for moving a **production** Goloom instance from Docker Compose (PostgreSQL + local `data/`) to the homelab GitOps deployment on CloudNativePG (`homelab-postgres`).

Homelab manifests live in the separate **homelab-gitops** repository (`apps/base/goloom`). This document covers data, secrets, and cutover from the Goloom application perspective.

## Overview

| | Docker (source) | Kubernetes (target) |
|--|-----------------|---------------------|
| Database | `goloom-db` (Postgres 16) | CNPG `homelab-postgres`, database `goloom` |
| Media files | Volume `goloom_data` → `data/media/{team_id}/{sha256}` | PVC mounted at `/app/data` (required for production) |
| URL | Traefik + `HOST` / `PUBLIC_BASE_URL` | Ingress, default `goloom.cluster.f4mily.net` |
| Secrets | `.env` | SOPS `goloom-app` + CNPG secret `homelab-postgres-goloom` |

**Do not** rely on an empty CNPG database plus Goloom’s embedded `internal/store/postgres/schema.sql` alone if Docker was initialized from `db/schema.sql` — schemas can diverge. Migrate with `pg_dump` / `pg_restore`.

## Prerequisites

- CNPG cluster `homelab-postgres` healthy and backups to S3 (Barman) working.
- Flux reconciled; Goloom `Database` CR `homelab-postgres-goloom` applied.
- `kubectl` access to the homelab cluster (`KUBECONFIG` to Talos kubeconfig).
- Maintenance window (stop writes during dump or use a quiet period).
- **PVC for `/app/data`** added in GitOps before cutover (Deployment currently has no volume — media is ephemeral without it).

## Phase 1 — Prepare secrets and URLs

### 1.1 `ENCRYPTION_KEY` (critical)

Provider tokens are encrypted at rest with `ENCRYPTION_KEY`. The value in Kubernetes **must match** the Docker `.env` value, or connected accounts cannot be decrypted after migration.

Update homelab-gitops:

```bash
cd apps/base/goloom
just sops-create goloom-app goloom \
  ENCRYPTION_KEY="<same-as-docker>" \
  BOOTSTRAP_ADMIN_TOKEN="<optional-new-or-same>"
```

### 1.2 Public URL and OAuth

In GitOps (`apps/base/goloom/configmap.yaml` and `cluster-config.yaml`):

- Default hostname: `goloom.cluster.f4mily.net` (`host_goloom`).
- For a public `*.f4mily.net` host, set `host_goloom` in `apps/overlays/main/cluster-config.yaml` and align:
  - `PUBLIC_BASE_URL`
  - `ALLOWED_ORIGINS`
  - `MASTODON_REDIRECT_URI` (and Mastodon application settings)

Optional: configure `OIDC_*` in `goloom-app` (e.g. Authentik) after cutover.

### 1.3 Media PVC (GitOps)

Add a persistent volume and mount it at `/app/data` in the Goloom Deployment (`Recreate` strategy is already set). Media path inside the container:

```text
/app/data/media/{team_id}/{sha256}
```

## Phase 2 — Database export (Docker)

Stop the app container to avoid writes (database can stay up for consistent dump, or stop both for simplicity):

```bash
docker compose -f docker-compose-traefik.yml stop app
# or: docker stop goloom-app-postgres goloom-db
```

Custom-format dump (recommended):

```bash
docker exec -t goloom-db pg_dump -U postgres -Fc goloom > /backup/goloom.dump
```

Plain SQL alternative:

```bash
docker exec -t goloom-db pg_dump -U postgres goloom > /backup/goloom.sql
```

## Phase 3 — Database import (CNPG)

Port-forward the CNPG primary:

```bash
export KUBECONFIG=/path/to/talos/kubeconfig
kubectl port-forward -n cnpg-system svc/homelab-postgres-rw 5432:5432
```

Read credentials from the cluster:

```bash
export PGUSER=$(kubectl get secret -n goloom homelab-postgres-goloom -o jsonpath='{.data.username}' | base64 -d)
export PGPASSWORD=$(kubectl get secret -n goloom homelab-postgres-goloom -o jsonpath='{.data.password}' | base64 -d)
export PGDATABASE=goloom
```

Restore (overwrites existing empty/partial DB):

```bash
pg_restore -h localhost -U "$PGUSER" -d "$PGDATABASE" --clean --if-exists /backup/goloom.dump
```

For plain SQL:

```bash
psql -h localhost -U "$PGUSER" -d "$PGDATABASE" -f /backup/goloom.sql
```

Restart Goloom:

```bash
kubectl rollout restart deployment/goloom -n goloom
kubectl rollout status deployment/goloom -n goloom
```

Verify logs: no `relation "job_locks" does not exist` (indicates incomplete schema instead of a full dump restore).

## Phase 4 — Media files

Copy `data/media` from the Docker volume to the Kubernetes PVC.

Example — export from volume:

```bash
docker run --rm -v goloom_data:/from -v /backup/goloom-media:/to alpine \
  cp -a /from/media /to/media
```

Copy into the pod (after PVC is mounted), e.g. with a one-off Job or `kubectl cp` into `deploy/goloom` (distroless image has no shell — prefer a debug copy Job or mount PVC in a helper pod).

Confirm DB rows reference media hashes that exist on disk under `data/media/{team_id}/`.

## Phase 5 — Cutover

1. Stop the Docker stack: `docker compose -f docker-compose-traefik.yml down` (or stop Traefik route to old host).
2. Point DNS to cluster ingress if using a public hostname.
3. `flux reconcile kustomization apps -n flux-system` (or wait for the next sync).
4. Smoke tests:
   - `GET /healthz`
   - Login (OIDC or bootstrap token if still enabled)
   - Teams, scheduled posts, media library previews
   - Mastodon OAuth (redirect URL matches `PUBLIC_BASE_URL`)
   - Scheduler processing (no repeated DB errors in logs)

## Phase 6 — Post-cutover

- [ ] Rotate `BOOTSTRAP_ADMIN_TOKEN` after first admin login if it was reused.
- [ ] Confirm CNPG backups: `kubectl get scheduledbackup -n cnpg-system homelab-postgres-daily`.
- [ ] Update Mastodon (and other providers) OAuth redirect URLs to the new base URL.
- [ ] Remove or archive Docker volumes after a successful validation period.

## Rollback

1. Stop Kubernetes Goloom (scale to 0 or suspend Flux `apps` kustomization for goloom).
2. Start Docker Compose with the **unchanged** volume and database volume.
3. Restore DNS to the Docker/Traefik endpoint.

CNPG data remains for a retry; Docker volumes remain until you delete them.

## Homelab references

| Item | Location (homelab-gitops) |
|------|---------------------------|
| Deployment / Ingress | `apps/base/goloom/` |
| CNPG Database CR | `apps/overlays/main/databases/goloom.yaml` |
| Hostname | `apps/overlays/main/cluster-config.yaml` → `host_goloom` |
| Postgres migration (generic) | `docs/migrations/phase1-postgres.md` |
| CNPG DR from S3 | `docs/disaster-recovery/cnpg-s3-dr.md` |

## Schema parity (developers)

Docker init uses `db/schema.sql`. Runtime in Kubernetes applies `internal/store/postgres/schema.sql` on empty databases. Keep these in sync when adding tables (e.g. `job_locks`). **Migrating production data via `pg_dump` avoids relying on an empty CNPG bootstrap.**
