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

## Filtering by account

On the **Accounts** tab the account cards act as a filter. By default every
account is included. The first click filters down to that one account; further
clicks add or remove accounts, and a **Reset** button returns to "all".
Deselected accounts are dimmed so it's clear which ones the growth chart and
engagement heatmap currently show.

## Best time to post

The engagement heatmap highlights the weekday/hour buckets where your published
posts performed best. AI agents can request the same signal through the
[`get_analytics_timeslots`](/guides/mcp/) MCP tool, which ranks slots by
historical engagement in a chosen timezone.

## Over the API

Analytics are available under the team-scoped analytics endpoints:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  https://goloom.example/v1/teams/$TEAM/analytics
```

This makes it easy for dashboards or agents to pull performance data
programmatically. See the [API reference](/api/).
