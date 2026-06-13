---
title: Review queue
description: Route drafts through approval before they publish — editors propose, owners approve.
sidebar:
  order: 7
---

The **review queue** adds an approval step between drafting and publishing. It's
useful when editors or AI agents produce content that a human should sign off on.

## How it works

1. An editor (or an agent via the API) creates a post.
2. Instead of publishing, the post enters the **review queue**.
3. A reviewer approves it — after which it schedules/publishes — or sends it back
   with changes.

## Roles

Approval rights follow [team roles](/guides/teams/): typically **owners** and
**editors** can review, while **viewers** can only observe.

## Accountability

Every approval and change is captured in the team's audit log, attributed to the
member or API key that performed it. See [Administration](/admin/administration/).
