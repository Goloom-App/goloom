---
title: Teams & roles
description: Organize work into team workspaces with owner, editor and viewer roles.
sidebar:
  order: 12
---

Everything in goloom is organized into **teams** (workspaces). Accounts, posts,
media and analytics all belong to a team.

There is no special "personal" workspace: every team works the same way. New
users either create their first team in the
[onboarding wizard](/getting-started/first-login/#4-create-your-first-team-onboarding)
or join an existing one through an [invitation](#inviting-people-by-email) —
a team used solo today can become a shared one later just by adding members.

## Roles

Each member has a role within a team:

| Role | Can do |
| --- | --- |
| **Owner** | Full control: manage members, accounts, settings; approve and publish. |
| **Editor** | Create and edit posts and media; participate in review. |
| **Viewer** | Read-only access to content and analytics. |

## Managing members

Owners manage membership from the team settings, adding existing users and
assigning roles. Over the API, members live under
`/v1/teams/{teamID}/members`.

## Inviting people by email

People who don't have an account yet (or whose account you don't know) are
added via **email invitations**. As a team owner, open the team settings and
use **Invite by email**:

1. Enter the invitee's **email address** and pick their **role** (editor or
   viewer).
2. goloom creates the invitation and shows an **invite link** of the form
   `https://your-goloom-host/?invite=<token>` — copy and share it now, it is
   displayed **only once**.
3. The invitee opens the link and signs in (the link survives the OIDC
   round-trip). The invitation is bound to the invited email address: it is
   redeemed automatically when the signed-in user's email matches, and the
   person lands directly in your team — no onboarding team creation needed.

Invitations **expire after 7 days**. Pending invitations are listed in the same
card with their expiry date and can be **revoked** at any time; creating and
revoking invitations is recorded in the [audit log](#audit-log).

Over the API: `POST /v1/teams/{teamID}/invitations` creates an invitation
(returns the plaintext token once), `GET` lists pending ones,
`DELETE /v1/teams/{teamID}/invitations/{invitationID}` revokes, and
`POST /v1/invitations/accept` redeems a token for the signed-in user.

## Scoping

Switching teams changes your entire context — content, accounts and metrics are
always **scoped to the selected team**. API resources are likewise team-scoped,
which keeps agent automation predictable and isolated.

## Audit log

Team actions are recorded in a per-team **audit log**, attributing each action to
the member or API key that performed it. See
[Administration](/admin/administration/).
