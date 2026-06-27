---
title: Kubernetes (Helm)
description: Deploy goloom on Kubernetes with the official Helm chart — SQLite or external PostgreSQL.
sidebar:
  order: 4
---

goloom ships a **Helm chart** at
[`deploy/helm/goloom`](https://github.com/Goloom-App/goloom/tree/main/deploy/helm/goloom)
for deploying on Kubernetes. It runs the single goloom container with a Service,
optional Ingress, a Secret for the sensitive config, and either a
**PersistentVolume (SQLite)** or an **external PostgreSQL**.

## Requirements

- A Kubernetes cluster and [Helm](https://helm.sh/) 3.x.
- For the SQLite default: a `StorageClass` that can provision a
  `ReadWriteOnce` volume.
- For PostgreSQL: a reachable database (e.g. a
  [CloudNativePG](https://cloudnative-pg.io/) cluster).
- An ingress controller if you want to expose it via Ingress.

## Quick start (SQLite)

Fetch the chart from the repository and install it. SQLite needs no external
database — it stores everything on a PersistentVolume.

```bash
git clone https://github.com/Goloom-App/goloom.git

helm install goloom ./goloom/deploy/helm/goloom \
  --namespace goloom --create-namespace \
  --set secret.encryptionKey="$(openssl rand -hex 32)" \
  --set secret.bootstrapAdminToken="change-me-please" \
  --set config.publicBaseUrl="https://goloom.example.com"

# Reach it without an ingress:
kubectl -n goloom port-forward svc/goloom 8080:8080
# then open http://localhost:8080
```

:::caution
`secret.encryptionKey` must be a long random value and stay **stable** — it
encrypts stored provider tokens. Losing or changing it makes connected accounts
undecryptable. Back it up.
:::

Prefer a values file over `--set` so secrets aren't in your shell history:

```yaml
# my-values.yaml
config:
  publicBaseUrl: https://goloom.example.com
secret:
  encryptionKey: "<long-random-secret>"
  bootstrapAdminToken: "<strong-bootstrap-token>"
persistence:
  size: 5Gi
  storageClass: ""        # leave empty for the cluster default
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: goloom.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: goloom-tls
      hosts:
        - goloom.example.com
```

```bash
helm install goloom ./goloom/deploy/helm/goloom -n goloom --create-namespace -f my-values.yaml
```

## PostgreSQL

For scaling or HA, point goloom at an external PostgreSQL. With Postgres the
chart drops the single-replica/`Recreate` constraint and honours `replicaCount`.

```yaml
database:
  type: postgres
  postgres:
    url: postgres://goloom:secret@my-postgres:5432/goloom?sslmode=require
replicaCount: 2
```

The DSN is written into the chart-managed Secret. To keep it out of values
entirely, create your own Secret with the keys `ENCRYPTION_KEY`,
`BOOTSTRAP_ADMIN_TOKEN` and `DATABASE_URL`, and reference it:

```yaml
secret:
  existingSecret: goloom-secrets
database:
  type: postgres
```

Upgrading the PostgreSQL major version later is a dump & restore — see the
[PostgreSQL major upgrade guide](/migrations/postgres-major-upgrade/).

## Configuration

Common non-secret options live under `config:` (public URL, log level, CORS,
MCP), and OIDC under `oidc:`. Anything not first-classed by the chart can be set
through `extraEnv` using the variables from
[configuration](/getting-started/configuration/):

```yaml
oidc:
  enabled: true
  issuerUrl: https://id.example.com/realms/main
  clientId: goloom
  clientSecret: "<client-secret>"
extraEnv:
  - name: SCHEDULER_WORKERS
    value: "4"
  - name: RATE_LIMIT_PER_MINUTE
    value: "240"
```

The pod is hardened by default (non-root, dropped capabilities, read-only root
filesystem); tune `resources`, `nodeSelector`, `tolerations` and `affinity` as
needed. See [`values.yaml`](https://github.com/Goloom-App/goloom/blob/main/deploy/helm/goloom/values.yaml)
for every option.

## Upgrades

```bash
helm upgrade goloom ./goloom/deploy/helm/goloom -n goloom -f my-values.yaml
```

By default the image tag follows the chart's `appVersion`. Pin a specific app
version with `--set image.tag=v0.1.2`. Read the
[release notes](https://github.com/Goloom-App/goloom/releases) before upgrading —
goloom is pre-1.0, so breaking changes can land between minor versions.

## Next steps

- [Configuration](/getting-started/configuration/) — all environment variables.
- [First login](/getting-started/first-login/) — bootstrap your first admin.
- [Docker → Kubernetes](/migrations/docker-to-kubernetes/) — migrating an
  existing Docker deployment.
