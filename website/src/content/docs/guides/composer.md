---
title: Composer
description: Write posts, tailor per-account versions, attach media and validate before scheduling.
sidebar:
  order: 3
---

The **composer** is where you write and schedule posts. Write once, then tailor
the result for each account before it goes out.

## Per-account versions

Select one or more connected accounts for a post. goloom keeps a **version per
account**, so you can adapt wording, hashtags or length to each network while
managing everything as one logical post.

## Media

Attach images and assets directly, or pick from the team's
[media library](/guides/media-library/) for reuse.

## Validation before scheduling

Before a post is scheduled, goloom **validates** it against each target
account's constraints. The same check is available over the API:

```bash
curl -X POST \
  -H "Authorization: Bearer $TOKEN" \
  https://goloom.example/v1/teams/$TEAM/posts/validate \
  -d '{"body":"Hello fediverse 👋"}'
```

This lets agents catch problems before committing to a schedule.

## Scheduling

Pick a publish time (timezone-aware) and schedule. Scheduled posts appear in the
[content calendar](/guides/content-calendar/) and can be rescheduled or
cancelled there.

If your team uses approvals, the post enters the
[review queue](/guides/review-queue/) instead of publishing immediately.
