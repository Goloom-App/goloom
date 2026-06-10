---
title: Administration
description: Admin panel overview — metrics, providers, users, logs, and AI teams.
---

The **Admin** panel is available to users with the global **administrator** role. Open it from the sidebar to monitor instance health, register social provider instances, and inspect logs.

## Status

The **Status** tab is the operational overview.

### Directory metrics

Counts across the whole instance:

- **Users** — registered accounts
- **Teams** — workspaces
- **Provider instances** — registered Mastodon, Friendica, or Bluesky servers

### Post pipeline

Live counts by post state:

- **Drafts** — saved but not scheduled
- **Queued / pending** — waiting for publish time
- **Publishing** — currently being sent
- **Posted** — successfully published
- **Failed** — publish errors (check logs)
- **Cancelled** — user- or system-cancelled

### Scheduler and server

Runtime details include worker count, poll interval, metrics sync interval, account health check interval, and the HTTP listen address. A green **Scheduler running** indicator confirms the background worker is active.

### Metrics sync

Background jobs pull engagement and follower data from connected accounts. The panel shows sync intervals, how many posted targets are waiting for metrics, and how many accounts have follower data.

Use **Sync metrics now** to trigger an immediate sync when dashboards look stale.

## Configurations

The **Configurations** tab summarizes effective server settings (read-only):

- **Security** — whether `ENCRYPTION_KEY` is configured, anonymous rate limits
- **Authentication** — OIDC and bootstrap token status
- **Scheduler** — worker and interval settings
- **Providers** — Mastodon redirect URI and related flags

Use this tab to confirm environment variables took effect without shell access. Change values in your `.env` or deployment config — see [Configuration](/docs/getting-started/configuration/).

## Providers

**Providers** is where administrators register social server instances before teams can connect accounts.

For each instance you can set:

- **Provider type** — Mastodon, Friendica, or Bluesky
- **Instance URL** — server base URL (Bluesky defaults to `https://bsky.social`)
- **OAuth credentials** — client ID and secret for Mastodon (Friendica uses manual tokens per user)
- **Auto-register** — optional Mastodon app registration from the instance URL

The tab lists existing instances with linked account counts. Edit or delete instances that are no longer needed. Deleting an instance does not automatically disconnect user accounts — review [Accounts](/docs/guides/accounts/) first.

Administrators can also trigger an **RSS sync** from this area when testing feed automation.

## Users

The **Users** tab lists everyone who has signed in:

- Name and email
- Global role (**administrator** or **member**)
- Registration date

Global administrators can access the admin panel; **members** use team roles (`owner`, `editor`, `viewer`) inside workspaces. See [Teams](/docs/guides/teams/).

## Logs

The **Logs** tab streams recent server log entries with:

- **Level filter** — DEBUG, INFO, WARN, ERROR
- **Search** — text match in messages
- **Archived** — include archived entries
- **Pagination** — browse older entries

Use logs to diagnose failed publishes, OAuth issues, and scheduler errors. Archive noisy entries once resolved.

## AI Agents

The **AI Agents** tab lists every workspace with **AI features enabled** on this instance. It helps operators see which teams depend on the AI service and voice engine.

Enable or disable AI per team in **Team settings** — see [AI features](/docs/guides/ai-features/).

## Related

- [Configuration](/docs/getting-started/configuration/) — environment variables
- [Accounts](/docs/guides/accounts/) — connecting social accounts after provider setup
- [Automation](/docs/guides/automation/) — RSS sync triggers from admin
