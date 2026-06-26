---
title: AI features
description: AI-assisted drafting with a per-team brand voice — and an API built for agents.
sidebar:
  order: 9
---

goloom is built to work well with AI — both as an **assistant inside the app**
and as an **API that agents can drive**. Both surfaces share **one tool catalog**,
so the assistant in the UI and an external agent over the API can do the same
things and behave consistently.

## In the app: the AI assistant

goloom has a built-in **[AI assistant](/guides/ai-assistant/)** — a screen-aware
chat agent that drafts and edits posts, sets up campaigns and automations, and
schedules publishing for the team. It is conversational, understands what you are
currently looking at, and asks for confirmation before anything publishes,
schedules, deletes or auto-publishes.

## AI-assisted drafting

goloom can help draft post copy. A per-team **brand voice** keeps generated text
on-brand and avoids generic, obviously-AI phrasing, so output sounds like your
team rather than a default model.

Drafts produced with AI can flow through the
[review queue](/guides/review-queue/) for human approval before publishing.

## Built for agents

goloom ships a built-in **[MCP server](/guides/mcp/)** so external agents like
Claude or OpenClaw can plan and schedule posts through the same natural-language
tools as the in-app assistant — no glue code required.

The REST API is likewise designed for automation:

- **Stable JSON** responses across core endpoints.
- **Predictable, team-scoped** resource paths.
- A **validation endpoint** so agents can check a post before scheduling:
  `POST /v1/teams/{teamID}/posts/validate`.
- A full **API token lifecycle** for secure agent onboarding.

## Provider keys

AI features require credentials for your chosen model provider, configured via
environment variables. See [configuration](/getting-started/configuration/) and
the [API reference](/api/) for details.
