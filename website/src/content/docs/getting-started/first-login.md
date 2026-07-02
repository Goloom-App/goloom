---
title: First login
description: Bootstrap your first admin, sign in with OIDC or the recovery URL, and create scoped API tokens.
sidebar:
  order: 3
---

When goloom starts for the first time it has no users. You create the first
admin using the **bootstrap token** you set in
[configuration](/getting-started/configuration/). After that, day-to-day sign-in
is via **OpenID Connect (OIDC)**, with the token form kept as a recovery path.

## 1. Open the app

Navigate to your instance (for local runs, <http://localhost:8080>).

## 2. Bootstrap the first admin

On first start the login screen shows an **administrator token** field. Paste the
value of `BOOTSTRAP_ADMIN_TOKEN`. This grants initial admin access so you can
create your first team and connect accounts.

## 3. Sign in with OIDC

Once OIDC is configured (`OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, …), it becomes the
primary sign-in method:

- Opening the app **starts the OIDC flow automatically** and redirects you to
  your identity provider.
- After an explicit **Sign out** you land back on the login screen (it does *not*
  bounce straight back to the IdP), so you can switch accounts or reach recovery.

### Recovery URL

The token / bootstrap form lives behind a dedicated fallback URL — handy when
OIDC is misconfigured or you need the bootstrap admin:

```
https://your-goloom-host/?login=recovery
```

The login screen also links to it ("Sign in with a recovery token"). Deployments
**without** OIDC always show the token form directly.

## 4. Create your first team (onboarding)

New users don't get a workspace automatically — everything in goloom lives in a
[team](/guides/teams/), so the first sign-in runs a short **onboarding**:

- **Create your team.** A welcome step asks for a team name (prefilled from
  your profile) and an optional description. Use the team solo or invite
  others later — there is no separate "personal" workspace kind.
- **Invited users skip this step.** If you signed in through an
  [invite link](/guides/teams/#inviting-people-by-email) (`?invite=<token>`),
  you land directly in the team that invited you.
- **Platform tour.** Afterwards a short tour walks through the main areas
  (dashboard, calendar & composer, accounts, team, analytics, settings). Skip
  it anytime; replay it later via **Settings → Platform tour**.

## 5. Create scoped API tokens

Use **Settings → API tokens → + New Token** for automation and AI agents. The
modal lets you set:

- **Name** and an optional **description**.
- **Team** — restrict the token to a single team, or grant access to all teams
  you belong to.
- **Scopes** — leave empty for full access, or restrict to specific actions:

  | Scope | Allows |
  | --- | --- |
  | `read` | Read posts, calendar, analytics, media, accounts, AI context |
  | `write` | Any create/update (superset of the two below) |
  | `write:draft` | Create/update drafts only |
  | `write:schedule` | Create/update scheduled posts only |
  | `delete` | Any delete (superset of the two below) |
  | `delete:draft` | Delete drafts only |
  | `delete:schedule` | Delete scheduled posts only |

- **Expiry** date.

The token is shown **once** in a dialog — click it to copy. It cannot be
retrieved later. Use it as a bearer credential:

```http
Authorization: Bearer <api-token>
```

:::note
The old AI-specific scopes (`ai:read:context`, `ai:write:drafts`, …) were
removed. Re-create any existing AI/automation tokens with the scopes above —
`read` plus the relevant `write:*` are enough for an agent. See the
[MCP guide](/guides/mcp/).
:::

Your current browser sign-in appears in the token list marked **"this browser"**;
it is created automatically and rolls over after 12 h of inactivity.

## 6. Rotate the bootstrap secret

After your admin and tokens exist, **rotate `BOOTSTRAP_ADMIN_TOKEN`** (or remove
it) so it can no longer be used to gain admin access.

## Next steps

- [Invite members](/guides/teams/#inviting-people-by-email) to your team.
- [Connect accounts](/guides/accounts/) for Mastodon, Friendica or Bluesky.
- [Explore the API](/api/) for automation and agents.
