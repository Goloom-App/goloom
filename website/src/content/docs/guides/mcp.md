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

The server speaks the modern **Streamable HTTP** transport
(MCP spec 2025-03-26) at a single endpoint:

```
https://your-goloom-host/mcp
```

- `POST /mcp` carries JSON-RPC messages; responses are `application/json` or a
  `text/event-stream` stream.
- `GET /mcp` opens the optional server→client SSE stream.

The trailing slash is optional (`/mcp` and `/mcp/` both work). The server runs
**stateless**: each request is self-contained and authenticated on its own, so
there is no session affinity to maintain — it works the same behind a single
instance or several replicas.

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

A token with **no scopes** has full access. To restrict an agent, grant scopes
(see [first login](/getting-started/first-login/)):

- `read` — required to open the MCP connection and use any read tool (teams,
  posts, calendar, analytics, brand profile).
- `write:draft` / `write:schedule` / `write` — needed for `draft_post` /
  `schedule_post` / the campaign- and feed-creating tools.
- `delete` — needed for `delete_post`.

You can also bind a token to a **single team**. Grant only what an agent needs —
for example `read` + `write:draft` for an agent that only prepares drafts.

:::note
The previous AI-specific scopes (`ai:read:context`, `ai:write:drafts`, …) were
removed; re-create existing agent tokens with the scopes above.
:::

## Connecting a client

Point an MCP client at the endpoint and pass your token as a bearer header. The
exact configuration depends on the client; a remote (Streamable HTTP) MCP server
entry looks like this:

```json
{
  "mcpServers": {
    "goloom": {
      "url": "https://your-goloom-host/mcp",
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
| `get_analytics_timeslots` | Best times to post: weekday/hour slots ranked by historical engagement, in a chosen timezone. |

## Validation

`schedule_post`, `draft_post`, `modify_post`, `create_recurring` and
`create_rss_feed` validate their input before anything is stored:

- **A title is required.** Every post needs an explicit `title` (including
  drafts) — it is never derived from the body. Automations (RSS/AI) generate one
  when there is no human title.
- **A team is required.** `team_id` must be set; a post is never stored with an
  inferred or empty team.
- **Targets must belong to the team.** Every account in `target_accounts` must
  exist and belong to `team_id`; cross-team or unknown accounts are rejected.
- **Character limits are enforced** when a post is scheduled (not for drafts).
  If the content — or a per-account `account_content_override` — exceeds a
  destination's limit, the call fails with a message naming the account, so the
  agent can shorten the text or add a fitting override and retry.
- **Overrides are never silently dropped.** An `account_content_override` keyed
  by an account that is not in `target_accounts` is an error, not a no-op.

## Working safely with agents

- Scope tokens tightly (`read` only, when an agent should not write) and bind
  them to a single team where possible.
- Route agent output through the [review queue](/guides/review-queue/) so a human
  approves before anything publishes.
- Every action is recorded in the team [audit log](/admin/administration/),
  attributed to the API key that performed it.

For the raw HTTP endpoints behind these tools, see the
[API reference](/api/).
