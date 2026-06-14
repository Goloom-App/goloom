---
title: Connect accounts
description: Connect Mastodon, Friendica and Bluesky accounts to a goloom team.
sidebar:
  order: 2
---

goloom publishes to **fediverse** networks. Each team connects its own accounts,
which are then available in the composer and calendar.

## Supported providers

### Mastodon

- Connect via **OAuth**.
- goloom can **auto-register** app credentials from the instance URL, so you
  usually only need to enter your instance and authorize.
- Publishing plus metrics: `likes`, `reposts`, `replies`.

### Friendica

- Provide **app credentials** manually for your Friendica node.
- Publishing with Mastodon-compatible metrics.

### Bluesky

- Connect with an **app password**.
- Publishing and metrics support.

## How connection works

1. Open the team's **Accounts** screen.
2. Choose a provider and follow the connection flow (OAuth redirect or app
   password / credentials, depending on the provider).
3. The account appears in the composer and calendar once connected.

Provider access tokens are **encrypted at rest** using your `ENCRYPTION_KEY`.

## Re-authentication

If a provider token expires or is revoked upstream, the account is flagged on the
dashboard. Reconnect it from the **Accounts** screen to resume publishing.
