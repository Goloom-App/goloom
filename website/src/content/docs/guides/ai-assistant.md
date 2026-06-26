---
title: AI assistant
description: The in-app AI assistant — a screen-aware chat agent that drafts, edits, schedules and automates posts, with a confirmation step before anything goes live.
sidebar:
  order: 10
  badge:
    text: AI
    variant: tip
---

goloom has a built-in **AI assistant**: a chat agent that lives inside the app and
can actually *do things* for your team — look up data, draft and edit posts, set
up campaigns and automations, and schedule publishing. It is driven by the same
tool catalog as the [MCP server](/guides/mcp/), so an agent behaves consistently
whether it runs inside goloom or connects from the outside. The difference is
that the in-app assistant is **conversational and screen-aware**, and it asks for
confirmation before any action that publishes, schedules, deletes, or
auto-publishes.

Open it from the **AI Assistant** panel in the app. The conversation is always
**scoped to the team you are working in**.

:::note
The assistant needs an AI provider configured for the team (an API key for your
chosen model). Without it the chat returns *"AI service not configured"*. See
[configuration](/getting-started/configuration/).
:::

## What it can do

**Answer questions and surface insights** — without leaving the chat:

- your calendar, posts and a content search (`get_calendar`, `find_free_slot`,
  `get_posts`, `search_posts`)
- connected accounts and their character limits (`get_platforms`)
- engagement analytics and top posts (`get_analytics`)
- **follower growth** over time and a single-metric history chart
  (`get_account_growth`, `get_metric_history`)
- best-performing hashtags and the best times to post
  (`get_hashtag_performance`, `get_analytics_timeslots`)
- your team's **brand voice**, which it recalls before writing copy so drafts
  sound like you (`get_brand_profile`)

**Draft and edit content** — these run immediately (nothing is published):

- save a new **draft** with per-platform variants (`draft_post`)
- **edit an existing** draft or scheduled post in place — never a duplicate
  (`modify_post`)
- define a reusable **campaign** format (`create_campaign`)

**Schedule and automate** — these are proposed and run only after you confirm
(see [Confirmation step](#confirmation-step)):

- **schedule** a post for a specific time (`schedule_post`)
- **delete** a draft or scheduled post (`delete_post`)
- set up a **recurring** auto-publishing automation (`create_recurring`)
- set up an **RSS** automation that turns feed items into posts (`create_rss_feed`)

## Screen awareness

The assistant can see **what you are currently looking at**, so you can talk in
context. Say *"shorten this post"* or *"schedule this for Friday"* and it reads
your active screen and the focused entity (`get_current_view`) instead of guessing.

When you have the **composer** open, the assistant edits the *unsaved* draft you
are writing rather than creating a new one: it returns a **suggested revision**
you apply with **Apply in composer** — default text and/or per-account overrides,
nothing is saved until you do. You can detach the composer context, and use the
per-account scope chips to **include or exclude** individual accounts from an edit.

## Confirmation step

Reads, drafts and in-place edits happen right away. Anything that **publishes,
schedules, deletes, or sets up auto-publishing** is different: the assistant does
**not** execute it. Instead it shows a **confirmation card** ("Confirm: …") with
**Confirm** and **Dismiss**. The action only runs when you click **Confirm**.

Your click is not a trust shortcut — team access and the required token scope are
re-checked when the action actually runs, exactly as for any other write.

## Referencing things with @-mentions

Type **`@`** to reference a specific **account**, **campaign**, **recurring
automation** or **RSS automation**. The assistant loads that entity's details
into the conversation, so it works with the real thing instead of a guess.
Mentioning an **account** also **scopes the request to it** — e.g. it targets
that account in a new post, or sets only that account's override when revising —
and leaves the others alone unless you ask otherwise.

## Slash commands and links

Type **`/`** for quick commands:

| Command | Does |
| --- | --- |
| `/draft` | Start a prompt to draft a post. |
| `/campaign` | Start a prompt to create a campaign. |
| `/recurring` | Start a prompt for a recurring automation. |
| `/rss` | Start a prompt for an RSS automation. |
| `/clear` | Start a **new conversation** (clears the current thread). |

**Paste a link** in your message and the assistant fetches the page's readable
text (`fetch_url`) so it can draft about real content — it won't invent what a
page says.

## Safety

- AI-produced drafts can flow through the [review queue](/guides/review-queue/)
  so a human approves before anything publishes.
- Every write the assistant performs is recorded in the team
  [audit log](/admin/administration/), attributed to the user who ran it.

## See also

- [AI features](/guides/ai-features/) — the overview of AI in goloom.
- [MCP server](/guides/mcp/) — drive the same tools from an external agent over
  the API.
