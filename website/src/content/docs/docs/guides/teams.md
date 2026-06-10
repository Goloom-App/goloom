---
title: Teams and workspaces
description: Workspaces, member roles, and team settings in goloom.
---

goloom organizes work in **teams** (also called **workspaces**). Each team has its own accounts, posts, media, automation rules, and analytics. Switch teams from the header dropdown.

## Personal vs shared workspaces

Every user gets a **personal workspace** for solo planning. Shared teams add members with explicit roles. Personal workspaces do not have shared members — invite collaborators on a shared team instead.

## Member roles

| Role | View content | Edit posts & automation | Manage accounts | Manage members |
|------|--------------|-------------------------|-----------------|----------------|
| **Viewer** | Yes | No | No | No |
| **Editor** | Yes | Yes | Yes | No |
| **Owner** | Yes | Yes | Yes | Yes |

- **Owners** can rename the team, change settings, add or remove members, and transfer ownership.
- **Editors** can compose, schedule, connect accounts, and configure automation.
- **Viewers** can monitor the calendar, analytics, and review queue but cannot publish or change settings.

## Team settings

Open **Team settings** from the sidebar to:

- Update the team **name** and **description**
- **Add members** and assign roles
- Enable **AI features** and set the AI service URL (when your deployment includes the AI worker)
- Turn on **Monitor external posts** to import and track posts published outside goloom
- **Import old posts** from connected accounts for analytics and AI profile analysis

## AI agent access

When AI is enabled for a team, generation and voice profile tools appear in the sidebar. The admin panel lists all AI-enabled teams — see [AI features](/docs/guides/ai-features/) and [Administration](/docs/admin/administration/).

## API access

Team-scoped API paths use the team ID, for example `GET /v1/teams/{teamID}/posts`. Create API tokens under **Settings** after your first login.

## Related guides

- [Accounts](/docs/guides/accounts/) — connect social accounts per workspace
- [First login](/docs/getting-started/first-login/) — bootstrap access and tokens
