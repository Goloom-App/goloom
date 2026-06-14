---
title: Administration
description: Admin tasks — provider management, audit logs, tokens and security hygiene.
sidebar:
  order: 1
---

Admin features cover instance-wide and cross-team concerns: managing providers,
reviewing audit logs and keeping secrets healthy.

## Provider management

Admins manage provider instance configuration under the admin endpoints
(`/v1/admin/*`). This includes the credentials and settings that let teams
connect [accounts](/guides/accounts/).

## Audit log

Each team has an **audit log** that records actions — including post lifecycle
changes, member changes and API-key revocations — and attributes them to the
**member or API key** responsible. This gives you an accountable history of who
did what.

## Tokens & secrets

- **API tokens** are stored as **hashes**; the plaintext is shown only once at
  creation.
- **Provider access tokens** are **encrypted at rest** with your
  `ENCRYPTION_KEY`.
- Rotate `BOOTSTRAP_ADMIN_TOKEN` after initial setup (see
  [first login](/getting-started/first-login/)).

## Health & operations

goloom exposes a health endpoint for monitoring:

```bash
curl https://goloom.example/healthz
```

For database choices and scaling, see
[configuration](/getting-started/configuration/) and the
[Docker → Kubernetes migration](/migrations/docker-to-kubernetes/).
