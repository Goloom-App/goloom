---
title: MCP server
description: goloom ships a built-in Model Context Protocol server so AI agents can plan and schedule posts with natural language.
sidebar:
  order: 10
  badge:
    text: AI
    variant: tip
---

goloom ships a **built-in [Model Context Protocol](https://modelcontextprotocol.io)
(MCP) server**. It lets AI agents — Claude, OpenClaw and any MCP-capable client —
drive goloom with natural language: find a free slot, draft a post, schedule it,
check analytics, and more, using a set of well-defined tools.

The MCP server is **enabled by default** and runs inside the same goloom process —
no extra service to deploy.

## Endpoint

The server is served over **SSE + JSON-RPC** at:

```
https://your-goloom-host/mcp/
```

- `GET /mcp/` opens the SSE stream.
- `POST /mcp/` carries JSON-RPC messages.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `MCP_ENABLED` | `true` | Set to `false` to disable the MCP server entirely. |
| `MCP_RATE_LIMIT_PER_MINUTE` | `60` | Per-client request rate limit. |

See [configuration](/getting-started/configuration/) for how environment
variables are set.

## Authentication

The MCP endpoint uses the same **bearer tokens** as the REST API:

```http
Authorization: Bearer <api-token>
```

API tokens must carry one of the AI scopes:

- `ai:read:context` — read teams, posts, calendar, analytics and brand profile.
- `ai:write:drafts` — additionally draft, schedule and modify posts.

Create a scoped API token from **Settings** (see [first login](/getting-started/first-login/)).
Grant only the scopes an agent actually needs.

## Connecting a client

Point an MCP client at the endpoint and pass your token as a bearer header. The
exact configuration depends on the client; a remote/SSE MCP server entry looks
like this:

```json
{
  "mcpServers": {
    "goloom": {
      "url": "https://your-goloom-host/mcp/",
      "headers": {
        "Authorization": "Bearer <api-token>"
      }
    }
  }
}
```

## Available tools

| Tool | What it does |
| --- | --- |
| `get_teams` | List teams the token can access. |
| `get_platforms` | List connected accounts with provider, username and character limits. |
| `get_brand_profile` | Read the team's tonality, style rules, identity and knowledge sources. |
| `get_calendar` | Get scheduled, pending and draft posts for a date range. |
| `find_free_slot` | Find the next free time slot (supports weekday names). |
| `schedule_post` | Schedule a post, with per-platform content overrides. |
| `draft_post` | Save a post as a draft. |
| `get_posts` | List posts, optionally filtered by status. |
| `modify_post` | Update content, schedule, targets or overrides of a post. |
| `delete_post` | Delete a scheduled or draft post. |
| `search_posts` | Search posts by text, date range or status. |
| `create_campaign` | Create a campaign with structure, hashtags and instructions. |
| `get_campaign` | Read full campaign details. |
| `create_recurring` | Create a recurring post template (RRULE schedule). |
| `create_rss_feed` | Create an RSS feed automation with a content template. |
| `get_analytics` | Engagement analytics (likes, reposts, followers) for a team or posts. |
| `get_hashtag_performance` | Best-performing hashtags from published-post analytics. |

## Working safely with agents

- Scope tokens tightly (`ai:read:context` only, when an agent should not write).
- Route agent output through the [review queue](/guides/review-queue/) so a human
  approves before anything publishes.
- Every action is recorded in the team [audit log](/admin/administration/),
  attributed to the API key that performed it.

For the raw HTTP endpoints behind these tools, see the
[API reference](/api/).
