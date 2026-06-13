---
title: Analytics
description: Track reach, engagement and follower growth with per-post and per-account metrics.
sidebar:
  order: 8
---

goloom includes **built-in analytics** so you can measure what's working without
a separate tool.

## What's measured

- **Post metrics** — likes, reposts/boosts and replies per post.
- **Account metrics** — reach, engagement and follower growth over time.
- **Comparisons** across multiple accounts within the team.

Metrics are normalized across providers where possible (Mastodon and
Friendica expose Mastodon-compatible metrics; Bluesky provides its own).

## Over the API

Analytics are available under the team-scoped analytics endpoints:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  https://goloom.example/v1/teams/$TEAM/analytics
```

This makes it easy for dashboards or agents to pull performance data
programmatically. See the [API reference](/api/).
