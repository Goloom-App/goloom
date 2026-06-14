---
title: Teams & roles
description: Organize work into team workspaces with owner, editor and viewer roles.
sidebar:
  order: 11
---

Everything in goloom is organized into **teams** (workspaces). Accounts, posts,
media and analytics all belong to a team.

## Roles

Each member has a role within a team:

| Role | Can do |
| --- | --- |
| **Owner** | Full control: manage members, accounts, settings; approve and publish. |
| **Editor** | Create and edit posts and media; participate in review. |
| **Viewer** | Read-only access to content and analytics. |

## Managing members

Owners manage membership from the team's **Members** screen, inviting people and
assigning roles. Over the API, members live under
`/v1/teams/{teamID}/members`.

## Scoping

Switching teams changes your entire context — content, accounts and metrics are
always **scoped to the selected team**. API resources are likewise team-scoped,
which keeps agent automation predictable and isolated.

## Audit log

Team actions are recorded in a per-team **audit log**, attributing each action to
the member or API key that performed it. See
[Administration](/admin/administration/).
