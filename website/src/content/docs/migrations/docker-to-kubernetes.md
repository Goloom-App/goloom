---
title: Docker → Kubernetes
description: Move a Docker + PostgreSQL goloom deployment to a Kubernetes (CNPG) setup.
sidebar:
  order: 1
---

If you run goloom with Docker and an external PostgreSQL database and want to
move to **Kubernetes** — for example a homelab cluster with CloudNativePG
(CNPG) — the migration is mostly about moving data and configuration, since
goloom itself is a single binary.

## Before you start

- A reachable **PostgreSQL** target in the cluster (e.g. a CNPG cluster).
- Your existing **`ENCRYPTION_KEY`** — you must reuse the exact same value, or
  encrypted provider tokens cannot be decrypted.
- A backup of your current database.

## Outline

1. **Provision PostgreSQL** in Kubernetes (e.g. a CNPG `Cluster`).
2. **Migrate data** from your current Postgres into the new database (dump and
   restore).
3. **Deploy goloom** as a `Deployment` + `Service`, configured with:
   - `DATABASE_URL` pointing at the in-cluster Postgres.
   - the **same `ENCRYPTION_KEY`** as before (via a `Secret`).
   - `PUBLIC_BASE_URL` set to the public URL behind your ingress.
4. **Expose it** through an Ingress / Gateway with TLS.
5. **Verify** with `GET /healthz` and a test login before cutting over DNS.

## Configuration parity

Keep the same environment configuration you used under Docker — see
[configuration](/getting-started/configuration/). The main differences are
*where* secrets and config come from (Kubernetes `Secret`/`ConfigMap`) rather
than a `.env` file.
