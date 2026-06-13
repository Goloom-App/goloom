---
title: First login
description: Bootstrap your first admin, then create API tokens and rotate the bootstrap secret.
sidebar:
  order: 3
---

When goloom starts for the first time it has no users. You create the first
admin using the **bootstrap token** you set in
[configuration](/getting-started/configuration/).

## 1. Open the app

Navigate to your instance (for local runs, <http://localhost:8080>).

## 2. Bootstrap with the admin token

In the **Settings** screen, provide the value of `BOOTSTRAP_ADMIN_TOKEN`. This
grants you initial admin access so you can create your first team and connect
accounts.

## 3. Create normal API tokens

Once you're in, create regular **API tokens** for day-to-day use and for any AI
agents that will talk to the API. Tokens are stored as hashes — copy a token
when it is shown, because it cannot be retrieved again.

Use a token as a bearer credential:

```http
Authorization: Bearer <api-token>
```

## 4. Rotate the bootstrap secret

After your admin and tokens exist, **rotate `BOOTSTRAP_ADMIN_TOKEN`** (or remove
it) so it can no longer be used to gain admin access.

## Next steps

- [Create a team](/guides/teams/) and invite members.
- [Connect accounts](/guides/accounts/) for Mastodon, Friendica or Bluesky.
- [Explore the API](/api/) for automation and agents.
