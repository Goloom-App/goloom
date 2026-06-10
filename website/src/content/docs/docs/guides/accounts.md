---
title: Connected accounts
description: Connect Mastodon, Friendica, and Bluesky accounts to a workspace.
---

Social accounts belong to the **workspace** selected in the header. Use them as post destinations in the [Composer](/docs/guides/composer/) and in [Automation](/docs/guides/automation/).

**Editors** and **owners** can connect or disconnect accounts. **Viewers** see connected accounts but cannot change them.

## Before you connect

Most providers require a **registered instance** in the admin panel (Mastodon and Friendica). Ask your goloom administrator to add your server under **Admin → Providers** if it is not listed.

Bluesky uses your handle and credentials directly; a PDS URL is optional for custom deployments.

## Mastodon

### OAuth (recommended)

1. Open **Accounts** and choose **Mastodon**.
2. Select a **registered instance** from the dropdown.
3. Click **Authorize in browser** and complete login on your Mastodon server.
4. You return to goloom with the account connected.

OAuth requires the instance to be configured with the correct redirect URL (`PUBLIC_BASE_URL` / `MASTODON_REDIRECT_URI` on the server).

### Manual token

If OAuth is unavailable, paste an **access token** (and optional refresh token) and click **Connect with token**. Use this for testing or restricted instances.

Administrators can optionally **auto-register** Mastodon app credentials from an instance URL during provider setup.

## Friendica

Friendica uses **manual provider credentials** configured by an administrator:

1. Choose **Friendica** and the registered instance.
2. Enter your **local username** and **access token** from your Friendica app settings.
3. Click **Connect Friendica**.

Publishing and metrics follow the same Mastodon-compatible paths where supported.

## Bluesky

### App password (recommended)

1. Create an app password in the Bluesky settings UI.
2. In goloom, choose **Bluesky** → **App password**.
3. Enter your **handle** (for example `you.bsky.social`) and the app password.
4. Click **Connect Bluesky**.

### Access token (JWT)

Advanced setups can connect with a JWT **access token** instead of an app password.

## After connecting

- Accounts show **engagement** and **follower sync** status on the Accounts page.
- Edit display name, character limit override, or rotate tokens from the account edit dialog.
- If status shows **Needs re-auth**, update credentials or run OAuth again.

Provider tokens are **encrypted at rest** on the goloom server. Use a strong `ENCRYPTION_KEY` in production and keep it stable across migrations.

## Related guides

- [Configuration](/docs/getting-started/configuration/) — `MASTODON_*` and public URL settings
- [Administration](/docs/admin/administration/) — register provider instances
- [Analytics](/docs/guides/analytics/) — metrics from connected accounts
