---
title: PostgreSQL major upgrade
description: Upgrade goloom's PostgreSQL to a new major version (e.g. 16 → 18) with a dump & restore.
sidebar:
  order: 2
---

A PostgreSQL **major** version bump (for example **16 → 18**, the default in
goloom's `docker-compose.yml`) is **not** an in-place swap: a newer PostgreSQL
refuses to start on an older major's data directory. You migrate the data with a
**dump & restore**.

:::note
**SQLite users are not affected.** With the default
`DATABASE_URL=file:./data/goloom.db`, there is no database server to upgrade —
just keep backing up the `data/` folder.
:::

## Before you start

- **Take the dump *before* you change the image.** If you already pulled a compose
  file with the new major and started it, PostgreSQL won't come up on the old data
  dir — temporarily set the image back to your current major (e.g. `postgres:16`),
  start it, and dump from there.
- **A maintenance window.** Stop goloom during the migration so nothing writes to
  the database while you dump and restore.
- **Keep `ENCRYPTION_KEY` unchanged.** Provider tokens are encrypted at rest; the
  dump preserves the encrypted columns, so they only stay decryptable if the key
  is identical afterwards.
- **A verified backup** (the dump file below) before you delete anything.

The commands below assume the `docker-compose.yml` shipped with goloom: a `db`
service (database `goloom`, user `postgres`) and a `postgres_data` volume. Adapt
names for your own setup.

## Dump → upgrade → restore (Docker Compose)

```bash
# 1. Stop the app so the database is quiescent (leave the OLD db running)
docker compose stop app-postgres

# 2. Dump the database from the CURRENT (old) PostgreSQL, custom format
docker compose exec -T db \
  pg_dump -U postgres -Fc goloom > "goloom-$(date +%F).dump"

# 3. Verify the dump is non-empty, then stop the database
ls -lh goloom-*.dump
docker compose stop db

# 4. Drop the OLD data directory so the new major starts on a clean volume.
#    DESTRUCTIVE — only after you have a good dump. Find the exact name first:
docker volume ls | grep postgres_data
docker volume rm <project>_postgres_data

# 5. Start a fresh database on the NEW major (the goloom compose already pins
#    postgres:18). The bundled db/schema.sql runs on the empty volume.
docker compose up -d db

# 6. Restore the dump. --clean --if-exists replaces the schema the init script
#    created, so the restore is authoritative.
docker compose exec -T db \
  pg_restore -U postgres -d goloom --clean --if-exists --no-owner < goloom-*.dump

# 7. Start the app again
docker compose up -d app-postgres
```

The `db` profile is enabled with `--profile postgres` if your compose uses
profiles (`docker compose --profile postgres up -d`).

## Verify

```bash
curl http://localhost:8080/healthz      # should report {"status":"ok", ...}
```

Then sign in and confirm your teams, posts, schedules and connected accounts are
all present. Connected accounts only keep working if `ENCRYPTION_KEY` is the same
as before.

## Rollback

Keep the `goloom-*.dump` file (and, ideally, the old volume) until you have
verified the upgrade. To roll back, restore the dump into a database running your
previous major version.

## Managed PostgreSQL (RDS, Cloud SQL, CNPG, …)

The same principle applies:

- Use the provider's **in-place major-version upgrade** if it offers one, **or**
- `pg_dump` from the old instance and `pg_restore` into a new instance on the
  target major, then point `DATABASE_URL` at it.

For very large databases, PostgreSQL's `pg_upgrade` is faster than dump & restore
but more involved to run against containers — dump & restore is the simplest
reliable path for typical goloom deployments. See also
[Docker → Kubernetes](/migrations/docker-to-kubernetes/).
